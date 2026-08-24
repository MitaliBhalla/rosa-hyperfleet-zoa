// Package lambdahttp adapts Lambda Function URL events to net/http handlers,
// supporting native response streaming via LambdaFunctionURLStreamingResponse.
package lambdahttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/aws/aws-lambda-go/events"
)

// StreamingHandler wraps an http.Handler to serve Lambda Function URL requests
// with response streaming support (up to 200MB).
type StreamingHandler struct {
	handler http.Handler
}

// NewStreamingHandler creates a Lambda Function URL streaming adapter
// for the given http.Handler.
func NewStreamingHandler(h http.Handler) *StreamingHandler {
	return &StreamingHandler{handler: h}
}

// Handle processes a Lambda Function URL request event by converting it to an
// http.Request, passing it to the wrapped handler, and returning a streaming
// response via io.Reader.
func (s *StreamingHandler) Handle(ctx context.Context, event events.LambdaFunctionURLRequest) (*events.LambdaFunctionURLStreamingResponse, error) {
	req, err := newHTTPRequest(ctx, event)
	if err != nil {
		return &events.LambdaFunctionURLStreamingResponse{
			StatusCode: http.StatusInternalServerError,
			Headers:    map[string]string{"Content-Type": "application/json"},
			Body:       strings.NewReader(`{"kind":"Error","code":"adapter_error","reason":"failed to build request"}`),
		}, nil
	}

	rec := &streamRecorder{
		headers: http.Header{},
		body:    &bytes.Buffer{},
	}
	s.handler.ServeHTTP(rec, req)

	responseHeaders := make(map[string]string, len(rec.headers))
	for k, v := range rec.headers {
		responseHeaders[k] = strings.Join(v, ", ")
	}

	return &events.LambdaFunctionURLStreamingResponse{
		StatusCode: rec.statusCode,
		Headers:    responseHeaders,
		Body:       rec.body,
	}, nil
}

func newHTTPRequest(ctx context.Context, event events.LambdaFunctionURLRequest) (*http.Request, error) {
	rawPath := event.RawPath
	if rawPath == "" {
		rawPath = "/"
	}
	rawQuery := event.RawQueryString

	u := &url.URL{
		Path:     rawPath,
		RawQuery: rawQuery,
	}

	var body io.Reader
	if event.Body != "" {
		if event.IsBase64Encoded {
			decoded, err := base64.StdEncoding.DecodeString(event.Body)
			if err != nil {
				return nil, err
			}
			body = bytes.NewReader(decoded)
		} else {
			body = strings.NewReader(event.Body)
		}
	}

	method := event.RequestContext.HTTP.Method
	if method == "" {
		method = http.MethodGet
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}

	for k, v := range event.Headers {
		req.Header.Set(k, v)
	}

	if ip := event.RequestContext.HTTP.SourceIP; ip != "" {
		req.Header.Set("X-Source-IP", ip)
	}
	if ua := event.RequestContext.HTTP.UserAgent; ua != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", ua)
	}
	if rid := event.RequestContext.RequestID; rid != "" {
		req.Header.Set("X-Request-ID", rid)
	}

	return req, nil
}

// streamRecorder captures the http.Handler response into a buffer for streaming.
type streamRecorder struct {
	headers    http.Header
	body       *bytes.Buffer
	statusCode int
	written    bool
}

func (r *streamRecorder) Header() http.Header {
	return r.headers
}

func (r *streamRecorder) WriteHeader(code int) {
	if !r.written {
		r.statusCode = code
		r.written = true
	}
}

func (r *streamRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.statusCode = http.StatusOK
		r.written = true
	}
	return r.body.Write(b)
}
