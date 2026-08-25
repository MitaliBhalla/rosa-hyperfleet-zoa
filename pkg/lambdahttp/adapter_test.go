package lambdahttp

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
)

func TestStreamingHandler_BasicGET(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	handler := NewStreamingHandler(mux)
	resp, err := handler.Handle(context.Background(), events.LambdaFunctionURLRequest{
		RawPath: "/health",
		RequestContext: events.LambdaFunctionURLRequestContext{
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{
				Method: "GET",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", body)
	}
	if resp.Headers["Content-Type"] != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", resp.Headers["Content-Type"])
	}
}

func TestStreamingHandler_POSTWithBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})

	handler := NewStreamingHandler(mux)
	resp, err := handler.Handle(context.Background(), events.LambdaFunctionURLRequest{
		RawPath: "/echo",
		Body:    `{"action":"test"}`,
		RequestContext: events.LambdaFunctionURLRequestContext{
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{
				Method: "POST",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"action":"test"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestStreamingHandler_Base64Body(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})

	payload := "binary content here"
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))

	handler := NewStreamingHandler(mux)
	resp, err := handler.Handle(context.Background(), events.LambdaFunctionURLRequest{
		RawPath:         "/upload",
		Body:            encoded,
		IsBase64Encoded: true,
		RequestContext: events.LambdaFunctionURLRequestContext{
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{
				Method: "POST",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != payload {
		t.Errorf("expected %q, got %q", payload, body)
	}
}

func TestStreamingHandler_QueryString(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("status")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("status=" + q))
	})

	handler := NewStreamingHandler(mux)
	resp, err := handler.Handle(context.Background(), events.LambdaFunctionURLRequest{
		RawPath:        "/search",
		RawQueryString: "status=succeeded",
		RequestContext: events.LambdaFunctionURLRequestContext{
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{
				Method: "GET",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "status=succeeded" {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestStreamingHandler_Headers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /whoami", func(w http.ResponseWriter, r *http.Request) {
		op := r.Header.Get("X-Operator")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("operator=" + op))
	})

	handler := NewStreamingHandler(mux)
	resp, err := handler.Handle(context.Background(), events.LambdaFunctionURLRequest{
		RawPath: "/whoami",
		Headers: map[string]string{
			"x-operator":   "slopezma",
			"x-account-id": "123456789012",
		},
		RequestContext: events.LambdaFunctionURLRequestContext{
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{
				Method: "GET",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "operator=slopezma" {
		t.Errorf("unexpected body: %s", body)
	}
	_ = resp
}

func TestStreamingHandler_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := NewStreamingHandler(mux)
	resp, err := handler.Handle(context.Background(), events.LambdaFunctionURLRequest{
		RawPath: "/nonexistent",
		RequestContext: events.LambdaFunctionURLRequestContext{
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{
				Method: "GET",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
}

func TestStreamingHandler_LargeResponse(t *testing.T) {
	largeBody := strings.Repeat("x", 10*1024*1024) // 10MB
	mux := http.NewServeMux()
	mux.HandleFunc("GET /large", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(largeBody))
	})

	handler := NewStreamingHandler(mux)
	resp, err := handler.Handle(context.Background(), events.LambdaFunctionURLRequest{
		RawPath: "/large",
		RequestContext: events.LambdaFunctionURLRequestContext{
			HTTP: events.LambdaFunctionURLRequestContextHTTPDescription{
				Method: "GET",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 10*1024*1024 {
		t.Errorf("expected 10MB body, got %d bytes", len(body))
	}
}
