package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
)

type staticCredentials struct{}

func (s staticCredentials) Retrieve(ctx context.Context) (aws.Credentials, error) {
	return aws.Credentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBYaDHqa0AP",
		Source:          "test",
	}, nil
}

func TestExtractRegion(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "When URL has standard API Gateway format it should extract region",
			url:  "https://abc123.execute-api.us-east-1.amazonaws.com/prod",
			want: "us-east-1",
		},
		{
			name: "When URL has eu-west-1 region it should extract correctly",
			url:  "https://xyz789.execute-api.eu-west-1.amazonaws.com/prod",
			want: "eu-west-1",
		},
		{
			name:    "When URL has no execute-api segment it should return error",
			url:     "https://example.com/api",
			wantErr: true,
		},
		{
			name:    "When URL is invalid it should return error",
			url:     "://not-a-url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractRegion(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractRegion(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractRegion(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestClientDispatch(t *testing.T) {
	t.Run("When API returns success it should parse response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/api/v0/trusted-actions/get_nodes/run" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(DispatchResponse{
				ID:     "exec-123",
				Status: "dispatched",
			})
		}))
		defer server.Close()

		c := &Client{
			baseURL:     server.URL,
			region:      "us-east-1",
			credentials: staticCredentials{},
			httpClient:  http.DefaultClient,
			signer:      nil,
		}

		resp, err := c.Dispatch(context.Background(), "get_nodes", &DispatchRequest{
			TargetCluster: "mc-useast1-1",
			Jira:          "TEST-123",
		})
		if err != nil {
			t.Fatalf("Dispatch() error = %v", err)
		}
		if resp.ID != "exec-123" {
			t.Errorf("Dispatch().ID = %q, want %q", resp.ID, "exec-123")
		}
	})

	t.Run("When API returns error it should return APIError", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(APIError{
				Code:   "CooldownActive",
				Reason: "write cooldown active (28s remaining)",
			})
		}))
		defer server.Close()

		c := &Client{
			baseURL:     server.URL,
			region:      "us-east-1",
			credentials: staticCredentials{},
			httpClient:  http.DefaultClient,
			signer:      nil,
		}

		_, err := c.Dispatch(context.Background(), "rollout_restart", &DispatchRequest{
			TargetCluster: "mc-useast1-1",
			Jira:          "TEST-123",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		apiErr, ok := err.(*APIError)
		if !ok {
			t.Fatalf("expected *APIError, got %T: %v", err, err)
		}
		if apiErr.Code != "CooldownActive" {
			t.Errorf("APIError.Code = %q, want %q", apiErr.Code, "CooldownActive")
		}
	})
}

func TestClientGetExecution(t *testing.T) {
	t.Run("When include=output it should pass query parameter", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v0/trusted-actions/runs/exec-456" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.URL.Query().Get("include") != "output" {
				t.Errorf("expected include=output, got %q", r.URL.Query().Get("include"))
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(Execution{
				ID:     "exec-456",
				Status: "succeeded",
				Output: "node1 Ready\n",
			})
		}))
		defer server.Close()

		c := &Client{
			baseURL:     server.URL,
			region:      "us-east-1",
			credentials: staticCredentials{},
			httpClient:  http.DefaultClient,
			signer:      nil,
		}

		exec, err := c.GetExecution(context.Background(), "exec-456", "output")
		if err != nil {
			t.Fatalf("GetExecution() error = %v", err)
		}
		if exec.Output.String() != "node1 Ready\n" {
			t.Errorf("GetExecution().Output = %q, want %q", exec.Output, "node1 Ready\n")
		}
	})
}
