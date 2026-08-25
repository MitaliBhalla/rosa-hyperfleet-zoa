// Package eksauth provides EKS authentication for non-Pod environments (Lambda).
// It generates bearer tokens using the same STS presigned URL mechanism as
// aws-iam-authenticator, allowing Lambda functions to authenticate to EKS
// clusters using their IAM execution role.
package eksauth

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"k8s.io/client-go/rest"
)

const (
	clusterIDHeader = "x-k8s-aws-id"
	tokenPrefix     = "k8s-aws-v1."
)

// NewRESTConfig builds a rest.Config for connecting to an EKS cluster from Lambda.
// The token is refreshed on every request using STS presigned URL.
//
// Required env vars (loaded into config by caller):
//   - EKS_CLUSTER_ENDPOINT: full https:// URL of the EKS API server
//   - EKS_CLUSTER_CA: base64-encoded CA certificate
//   - EKS_CLUSTER_NAME: cluster name (used in the token's x-k8s-aws-id header)
func NewRESTConfig(endpoint, caBase64, clusterName string, awsCfg aws.Config) (*rest.Config, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("EKS_CLUSTER_ENDPOINT is required")
	}
	if caBase64 == "" {
		return nil, fmt.Errorf("EKS_CLUSTER_CA is required")
	}
	if clusterName == "" {
		return nil, fmt.Errorf("EKS_CLUSTER_NAME is required")
	}

	caData, err := base64.StdEncoding.DecodeString(caBase64)
	if err != nil {
		return nil, fmt.Errorf("decoding EKS cluster CA: %w", err)
	}

	tokenSource := &stsTokenSource{
		stsClient:   sts.NewFromConfig(awsCfg),
		clusterName: clusterName,
	}

	return &rest.Config{
		Host: endpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: caData,
		},
		WrapTransport: func(rt http.RoundTripper) http.RoundTripper {
			return &tokenTransport{
				base:        rt,
				tokenSource: tokenSource,
			}
		},
	}, nil
}

// tokenCacheTTL is set to 9 minutes. EKS tokens are valid for up to 15 minutes
// (X-Amz-Expires=600), so a 9-minute cache ensures tokens are never used after
// ~60% of their lifetime — leaving margin for clock skew.
const tokenCacheTTL = 9 * time.Minute

// stsTokenSource generates EKS-compatible bearer tokens using STS GetCallerIdentity presigned URL.
// Tokens are cached for tokenCacheTTL to avoid re-signing on every K8s API call.
type stsTokenSource struct {
	stsClient   *sts.Client
	clusterName string

	mu       sync.Mutex
	cached   string
	cachedAt time.Time
}

func (s *stsTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	if s.cached != "" && time.Since(s.cachedAt) < tokenCacheTTL {
		token := s.cached
		s.mu.Unlock()
		return token, nil
	}
	s.mu.Unlock()

	presigner := sts.NewPresignClient(s.stsClient, func(opts *sts.PresignOptions) {
		opts.ClientOptions = append(opts.ClientOptions, func(o *sts.Options) {
			o.APIOptions = append(o.APIOptions, eksPresignMiddleware(s.clusterName))
		})
	})

	presigned, err := presigner.PresignGetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("presigning GetCallerIdentity: %w", err)
	}

	token := tokenPrefix + base64.RawURLEncoding.EncodeToString([]byte(presigned.URL))

	s.mu.Lock()
	s.cached = token
	s.cachedAt = time.Now()
	s.mu.Unlock()

	return token, nil
}

// eksPresignMiddleware injects the x-k8s-aws-id header and X-Amz-Expires=600
// query parameter into the STS presigned request. Both must be set before the
// SigV4 signer computes the canonical request:
//   - x-k8s-aws-id: cluster scope that EKS pins in the signature
//   - X-Amz-Expires=600: STS token validity (10 min); tokens are cached for 9 min
//     to ensure freshness while reducing STS API calls
func eksPresignMiddleware(clusterName string) func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		return stack.Build.Add(middleware.BuildMiddlewareFunc("EKSPresign",
			func(ctx context.Context, in middleware.BuildInput, next middleware.BuildHandler) (
				middleware.BuildOutput, middleware.Metadata, error,
			) {
				if req, ok := in.Request.(*smithyhttp.Request); ok {
					req.Header.Set(clusterIDHeader, clusterName)
					q := req.URL.Query()
					q.Set("X-Amz-Expires", "600")
					req.URL.RawQuery = q.Encode()
				}
				return next.HandleBuild(ctx, in)
			}), middleware.After)
	}
}

// tokenTransport injects a fresh EKS bearer token into each K8s API request.
type tokenTransport struct {
	base        http.RoundTripper
	tokenSource *stsTokenSource
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.tokenSource.Token(req.Context())
	if err != nil {
		return nil, fmt.Errorf("generating EKS token: %w", err)
	}
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+token)
	return t.base.RoundTrip(req)
}

var _ http.RoundTripper = (*tokenTransport)(nil)
