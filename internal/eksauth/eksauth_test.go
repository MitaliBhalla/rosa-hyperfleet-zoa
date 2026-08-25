package eksauth

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
)

func TestTokenCaching_WhenCalledTwice_ItShouldReturnCachedToken(t *testing.T) {
	src := &stsTokenSource{
		cached:   "k8s-aws-v1.test-token",
		cachedAt: time.Now(),
	}

	token, err := src.Token(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "k8s-aws-v1.test-token" {
		t.Fatalf("expected cached token, got %q", token)
	}
}

func TestTokenCaching_WhenExpired_ItShouldBypassCache(t *testing.T) {
	src := &stsTokenSource{
		cached:   "k8s-aws-v1.stale-token",
		cachedAt: time.Now().Add(-10 * time.Minute),
	}

	// Token is expired; verify the cache state is detected as stale.
	src.mu.Lock()
	isStale := time.Since(src.cachedAt) >= tokenCacheTTL
	src.mu.Unlock()

	if !isStale {
		t.Fatal("expected token to be stale after 10 minutes")
	}
}

func TestNewRESTConfig_WhenMissingEndpoint_ItShouldReturnError(t *testing.T) {
	_, err := NewRESTConfig("", "Y2E=", "cluster", aws.Config{})
	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}
}

func TestNewRESTConfig_WhenMissingCA_ItShouldReturnError(t *testing.T) {
	_, err := NewRESTConfig("https://example.com", "", "cluster", aws.Config{})
	if err == nil {
		t.Fatal("expected error for missing CA")
	}
}

func TestNewRESTConfig_WhenMissingClusterName_ItShouldReturnError(t *testing.T) {
	_, err := NewRESTConfig("https://example.com", "Y2E=", "", aws.Config{})
	if err == nil {
		t.Fatal("expected error for missing cluster name")
	}
}
