package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// handleDownloadOutput streams execution output directly from S3.
// GET /api/v0/trusted-actions/runs/{id}/output
// GET /api/v0/trusted-actions/runs/{id}/logs
func (h *Handler) handleDownloadOutput(w http.ResponseWriter, r *http.Request, id, artifact string) {
	ctx := r.Context()

	exec, err := h.executionStore.Get(ctx, id)
	if err != nil {
		h.logger.Error("failed to get execution", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to get execution")
		return
	}
	if exec == nil {
		writeError(w, http.StatusNotFound, "not_found", "execution not found")
		return
	}

	if !exec.Status.IsTerminal() {
		writeError(w, http.StatusConflict, "not_ready", "execution has not completed yet")
		return
	}

	var s3Key string
	var contentType string

	switch artifact {
	case "output":
		if exec.OutputFormat == "tar.gz" {
			s3Key = fmt.Sprintf("executions/%s/output.tar.gz", id)
			contentType = "application/gzip"
		} else {
			s3Key = fmt.Sprintf("executions/%s/output.json", id)
			contentType = "application/json"
		}
	case "logs":
		s3Key = fmt.Sprintf("executions/%s/execution.log", id)
		contentType = "text/plain"
	default:
		writeError(w, http.StatusBadRequest, "invalid_artifact", "artifact must be 'output' or 'logs'")
		return
	}

	out, err := h.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(h.cfg.ArtifactBucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchKey") {
			writeError(w, http.StatusNotFound, "artifact_not_found", fmt.Sprintf("%s not available for this execution", artifact))
			return
		}
		h.logger.Error("failed to get S3 object", "error", err, "key", s3Key)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve artifact")
		return
	}
	defer out.Body.Close()

	w.Header().Set("Content-Type", contentType)
	if out.ContentLength != nil && *out.ContentLength > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", *out.ContentLength))
	}
	w.Header().Set("X-Execution-ID", id)
	w.Header().Set("X-Execution-Status", string(exec.Status))
	w.WriteHeader(http.StatusOK)

	_, _ = io.Copy(w, out.Body)
}
