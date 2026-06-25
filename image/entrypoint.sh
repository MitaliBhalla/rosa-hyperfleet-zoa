#!/bin/bash
set -uo pipefail

ts() { date -u '+%H:%M:%S'; }
EXEC_LOG="/artifacts/execution.log"

{
  echo "[$(ts)] runner starting"
  echo "[zoa] execution_id=${RUN_ID} action=${ACTION_NAME} target=${CLUSTER_ID}"
  echo "[zoa] operator=${OPERATOR} scope=${SCOPE} type=${TYPE}"
  echo "[zoa] revision=${REVISION}"
  PARAMS=$(env | grep "^PARAM_" | sed 's/^PARAM_//' | tr '\n' ' ')
  [ -n "$PARAMS" ] && echo "[zoa] params=${PARAMS}"
  echo "[zoa] started_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo "---"
  /zoa/run.sh
} 2>&1 | tee "$EXEC_LOG"
EXIT_CODE=${PIPESTATUS[0]}

{
  echo "---"
  echo "[$(ts)] exit_code=${EXIT_CODE}"
  echo "[$(ts)] patching configmap"
} | tee -a "$EXEC_LOG"

OUTPUT_CM="${OUTPUT_CONFIGMAP:-}"
if [ -z "${OUTPUT_CM}" ]; then
  echo "[$(ts)] ERROR: OUTPUT_CONFIGMAP not set" | tee -a "$EXEC_LOG"
  exit ${EXIT_CODE}
fi

# --- Output size validation ---
# ConfigMap limit is 1MiB. Base64 inflates by 4/3. Safe raw budget: 700KB combined.
BUDGET=700000
LOG_SIZE=$(stat -c%s "$EXEC_LOG")
OUT_SIZE=0
[ -f /artifacts/output.json ] && OUT_SIZE=$(stat -c%s /artifacts/output.json)
COMBINED=$((LOG_SIZE + OUT_SIZE))

if [ "$COMBINED" -gt "$BUDGET" ]; then
  echo "[$(ts)] WARNING: combined output too large (${COMBINED} > ${BUDGET} bytes), truncating" | tee -a "$EXEC_LOG"
  if [ "$OUT_SIZE" -le "$BUDGET" ]; then
    LOG_BUDGET=$((BUDGET - OUT_SIZE))
    { printf '[TRUNCATED: showing last %d of %d bytes]\n' "$LOG_BUDGET" "$LOG_SIZE"
      tail -c "$LOG_BUDGET" "$EXEC_LOG"
    } > /artifacts/truncated.log
    mv /artifacts/truncated.log "$EXEC_LOG"
  else
    OUT_BUDGET=$((BUDGET - 100000))
    { head -c "$OUT_BUDGET" /artifacts/output.json
      printf '\n\n--- TRUNCATED (original: %d bytes, showing first %d) ---\n' "$OUT_SIZE" "$OUT_BUDGET"
    } > /artifacts/truncated_output.json
    mv /artifacts/truncated_output.json /artifacts/output.json
    LOG_BUDGET=100000
    if [ "$LOG_SIZE" -gt "$LOG_BUDGET" ]; then
      { printf '[TRUNCATED: showing last %d of %d bytes]\n' "$LOG_BUDGET" "$LOG_SIZE"
        tail -c "$LOG_BUDGET" "$EXEC_LOG"
      } > /artifacts/truncated.log
      mv /artifacts/truncated.log "$EXEC_LOG"
    fi
  fi
fi

LOG_B64=$(base64 -w0 "$EXEC_LOG")
PATCH="{\"binaryData\":{\"execution.log\":\"${LOG_B64}\""

if [ -f /artifacts/output.json ]; then
  OUTPUT_B64=$(base64 -w0 /artifacts/output.json)
  PATCH="${PATCH},\"output.json\":\"${OUTPUT_B64}\""
fi

PATCH="${PATCH}},\"data\":{\"exit-code\":\"${EXIT_CODE}\"}}"

if kubectl patch configmap "${OUTPUT_CM}" -n "${JOB_NAMESPACE}" --type=merge -p "${PATCH}" 2>&1; then
  echo "[$(ts)] configmap patched"
else
  echo "[$(ts)] ERROR: failed to patch ConfigMap"
fi

exit ${EXIT_CODE}
