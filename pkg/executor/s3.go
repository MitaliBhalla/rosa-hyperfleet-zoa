package executor

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3API defines the subset of S3 operations used by the executor.
// Extracted as an interface to enable unit testing without a live S3 bucket.
type S3API interface {
	PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
}

// Verify *s3.Client satisfies S3API at compile time.
var _ S3API = (*s3.Client)(nil)
