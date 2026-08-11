package metrics

import (
	"net/http"
	"time"
)

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

// HTTPMetrics wraps an http.Handler and emits EMF metrics for every request.
// Dimensions: Cluster, HandlerMode, Method, Path (first 2 segments).
func HTTPMetrics(cluster string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rec, r)

		elapsed := time.Since(start)
		dims := map[string]string{
			"Cluster":     cluster,
			"HandlerMode": "api",
			"Method":      r.Method,
		}

		m := map[string]MetricValue{
			"RequestDuration": Milliseconds(elapsed.Milliseconds()),
			"RequestCount":    Count(1),
		}
		if rec.statusCode >= 500 {
			m["ServerErrors"] = Count(1)
		}

		Emit(dims, m)
	})
}
