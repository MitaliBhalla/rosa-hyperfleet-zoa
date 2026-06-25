#!/bin/bash
set -uo pipefail

ts() { date -u '+%H:%M:%S'; }
ts_at() { date -u -d "@$1" '+%H:%M:%S'; }

EXEC_TIMEOUT="${EXECUTION_TIMEOUT:-1800}"
UPLOAD_TIMEOUT="${UPLOAD_TIMEOUT:-120}"
RUNNER_JOB="${RUNNER_JOB_NAME}"
OUTPUT_CM="${OUTPUT_CONFIGMAP}"
BUCKET="${ARTIFACT_BUCKET}"
NS="${JOB_NAMESPACE}"

T0=$(date +%s)
echo "[$(ts_at $T0)] upload starting, waiting for runner (timeout: ${EXEC_TIMEOUT}s)"

while true; do
  STATUS=$(kubectl get job "${RUNNER_JOB}" -n "${NS}" \
    -o jsonpath='{.status.conditions[?(@.status=="True")].type}' 2>/dev/null)
  case "$STATUS" in
    *Complete*|*Failed*) break ;;
  esac
  ELAPSED=$(($(date +%s) - T0))
  if [ "$ELAPSED" -ge "$EXEC_TIMEOUT" ]; then
    echo "[$(ts)] WARNING: runner did not finish within ${EXEC_TIMEOUT}s"
    break
  fi
  sleep 1
done

T1=$(date +%s)
WAIT_S=$((T1 - T0))
echo "[$(ts_at $T1)] runner done (${WAIT_S}s), reading configmap"

CM_JSON=$(kubectl get configmap "${OUTPUT_CM}" -n "${NS}" -o json 2>/dev/null)
LOG_B64=$(printf '%s' "$CM_JSON" | jq -r '.binaryData["execution.log"] // empty')
OUTPUT_B64=$(printf '%s' "$CM_JSON" | jq -r '.binaryData["output.json"] // empty')
EXIT_CODE=$(printf '%s' "$CM_JSON" | jq -r '.data["exit-code"] // "1"')

T2=$(date +%s)
CM_S=$((T2 - T1))
echo "[$(ts_at $T2)] configmap read (${CM_S}s), decoding"

if [ -z "${LOG_B64}" ] && [ -z "${OUTPUT_B64}" ]; then
  echo "[$(ts)] ERROR: ConfigMap has no data — runner crashed before writing"
  exit 1
fi

mkdir -p /tmp/upload
[ -n "${LOG_B64}" ] && printf '%s' "${LOG_B64}" | base64 -d > /tmp/upload/execution.log
[ -n "${OUTPUT_B64}" ] && printf '%s' "${OUTPUT_B64}" | base64 -d > /tmp/upload/output.json

T3=$(date +%s)
DECODE_S=$((T3 - T2))

UL=/tmp/upload/execution.log
{
  echo "--- upload ---"
  echo "[$(ts_at $T0)] upload starting"
  echo "[$(ts_at $T1)] runner waited (${WAIT_S}s)"
  echo "[$(ts_at $T2)] configmap read (${CM_S}s)"
  echo "[$(ts_at $T3)] decoded (${DECODE_S}s), uploading to s3"
} >> "$UL"

echo "[$(ts)] uploading to s3://${BUCKET}/${RUN_ID}/"
if aws s3 cp /tmp/upload/ "s3://${BUCKET}/${RUN_ID}/" --recursive 2>&1; then
  echo "[$(ts)] upload done (exit_code=${EXIT_CODE})"
  exit 0
else
  echo "[$(ts)] WARNING: first upload attempt failed, retrying in 5s..."
  sleep 5
  if aws s3 cp /tmp/upload/ "s3://${BUCKET}/${RUN_ID}/" --recursive 2>&1; then
    echo "[$(ts)] upload done on retry (exit_code=${EXIT_CODE})"
    exit 0
  else
    echo "[$(ts)] ERROR: s3 upload failed after retry"
    exit 1
  fi
fi
