package handler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"k8s.io/client-go/kubernetes"
)

// HealthDeps holds clients needed for the startup health check.
type HealthDeps struct {
	DynamoClient *dynamodb.Client
	S3Client     *s3.Client
	KubeClient   kubernetes.Interface
	TableName    string
	BucketName   string
	Mode         string // "api" or "worker"
}

// CheckStartupHealth verifies hard dependencies are reachable.
//
// Mode-aware behavior:
//   - API mode: only checks DynamoDB (needed for most requests). S3 and K8s are
//     soft deps — checked lazily when a TA execution or download is triggered.
//     This ensures read-only operations (list TAs, get status) still work even
//     if EKS is temporarily unreachable.
//   - Worker mode: checks DynamoDB AND K8s (reconciler always needs both).
func CheckStartupHealth(ctx context.Context, deps HealthDeps, logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var failures int

	// DynamoDB: hard dependency for both modes
	if deps.DynamoClient != nil && deps.TableName != "" {
		_, err := deps.DynamoClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: &deps.TableName,
		})
		if err != nil {
			logger.Error("STARTUP HEALTH: DynamoDB table unreachable", "table", deps.TableName, "error", err)
			failures++
		} else {
			logger.Info("startup health: DynamoDB OK", "table", deps.TableName)
		}
	}

	// K8s: hard dependency for worker mode only (reconciler, GC, execution all need it).
	// For API mode, K8s is a soft dependency — only needed when executing a TA.
	// The executor returns a clear error at execution time if K8s is down.
	if deps.Mode == "worker" && deps.KubeClient != nil {
		_, err := deps.KubeClient.Discovery().ServerVersion()
		if err != nil {
			logger.Error("STARTUP HEALTH: Kubernetes API unreachable", "error", err)
			failures++
		} else {
			logger.Info("startup health: Kubernetes API OK")
		}
	}

	// S3: soft dependency for both modes — only needed for artifact upload/download.
	// Log a warning but don't fail startup.
	if deps.S3Client != nil && deps.BucketName != "" {
		_, err := deps.S3Client.HeadBucket(ctx, &s3.HeadBucketInput{
			Bucket: &deps.BucketName,
		})
		if err != nil {
			logger.Warn("startup health: S3 bucket unreachable (downloads/uploads will fail)", "bucket", deps.BucketName, "error", err)
		} else {
			logger.Info("startup health: S3 OK", "bucket", deps.BucketName)
		}
	}

	if failures > 0 {
		return fmt.Errorf("startup health check failed: %d hard dependencies unreachable", failures)
	}
	return nil
}
