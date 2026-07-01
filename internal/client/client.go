package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

type Client struct {
	baseURL     string
	region      string
	credentials aws.CredentialsProvider
	httpClient  *http.Client
	signer      *v4.Signer
}

func New(apiURL string, creds aws.CredentialsProvider) (*Client, error) {
	region, err := extractRegion(apiURL)
	if err != nil {
		return nil, err
	}

	return &Client{
		baseURL:     strings.TrimRight(apiURL, "/"),
		region:      region,
		credentials: creds,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		signer:      v4.NewSigner(),
	}, nil
}

func extractRegion(apiURL string) (string, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return "", fmt.Errorf("invalid API URL: %w", err)
	}
	// Format: <id>.execute-api.<region>.amazonaws.com
	parts := strings.Split(u.Host, ".")
	for i, p := range parts {
		if p == "execute-api" && i+1 < len(parts) {
			return parts[i+1], nil
		}
	}
	return "", fmt.Errorf("cannot extract region from URL %q (expected *.execute-api.<region>.amazonaws.com)", apiURL)
}

func (c *Client) Dispatch(ctx context.Context, action string, req *DispatchRequest) (*DispatchResponse, error) {
	var resp DispatchResponse
	if err := c.do(ctx, http.MethodPost, "/trusted-actions/"+url.PathEscape(action)+"/run", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetExecution(ctx context.Context, id string, include string) (*Execution, error) {
	path := "/trusted-actions/runs/" + url.PathEscape(id)
	if include != "" {
		q := url.Values{}
		q.Set("include", include)
		path += "?" + q.Encode()
	}
	var resp Execution
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListExecutions(ctx context.Context, query url.Values) (*ExecutionList, error) {
	path := "/trusted-actions/runs"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var resp ExecutionList
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetAction(ctx context.Context, name string) (*Action, error) {
	var resp Action
	if err := c.do(ctx, http.MethodGet, "/trusted-actions/"+url.PathEscape(name), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListActions(ctx context.Context) (*ActionList, error) {
	var resp ActionList
	if err := c.do(ctx, http.MethodGet, "/trusted-actions", nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) ListAudit(ctx context.Context, query url.Values) (*AuditList, error) {
	path := "/trusted-actions/audit"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	var resp AuditList
	if err := c.do(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, result any) error {
	fullURL := c.baseURL + "/api/v0" + path

	var bodyReader io.ReadSeeker
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	} else {
		bodyReader = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if c.signer != nil {
		creds, err := c.credentials.Retrieve(ctx)
		if err != nil {
			return fmt.Errorf("retrieving AWS credentials: %w", err)
		}

		payloadHash := sha256Hash(bodyReader)
		if err := c.signer.SignHTTP(ctx, creds, req, payloadHash, "execute-api", c.region, time.Now()); err != nil {
			return fmt.Errorf("signing request: %w", err)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	const maxResponseSize = 10 << 20 // 10 MB
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if json.Unmarshal(respBody, &apiErr) == nil && apiErr.Code != "" {
			return &apiErr
		}
		body := string(respBody)
		if len(body) > 512 {
			body = body[:512] + "...(truncated)"
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

func sha256Hash(reader io.ReadSeeker) string {
	h := sha256.New()
	_, _ = io.Copy(h, reader)
	_, _ = reader.Seek(0, io.SeekStart)
	return hex.EncodeToString(h.Sum(nil))
}
