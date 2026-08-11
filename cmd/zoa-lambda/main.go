// Binary zoa-lambda is the Lambda entry point. It performs dependency injection
// and starts either an HTTP server (API mode with LWA) or a native Lambda handler (Worker mode).
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	awslambda "github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/openshift-online/rosa-hyperfleet-zoa/internal/eksauth"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/api"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/config"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/executor"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/handler"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/scheduler"
	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/store"
)

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	logLevel := parseLogLevel(os.Getenv("LOG_LEVEL"))
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	logger.Info("starting zoa-lambda",
		"handler_mode", cfg.HandlerMode,
		"reconciler_deadline_s", cfg.ReconcilerDeadlineSeconds,
		"execution_deadline_s", cfg.ExecutionDeadlineSeconds,
		"max_batch_per_tick", cfg.MaxBatchPerTick,
	)

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(cfg.Region))
	if err != nil {
		logger.Error("failed to load AWS config", "error", err)
		os.Exit(1)
	}

	// For cross-account data access (MC→RC), assume the data-access role to get
	// credentials that target the RC account's DynamoDB tables and S3 bucket.
	dataStoreCfg := awsCfg
	if cfg.DataStoreRoleARN != "" {
		dataStoreCfg, err = awsconfig.LoadDefaultConfig(context.Background(),
			awsconfig.WithRegion(cfg.Region),
			awsconfig.WithCredentialsProvider(
				stscreds.NewAssumeRoleProvider(
					sts.NewFromConfig(awsCfg),
					cfg.DataStoreRoleARN,
				),
			),
		)
		if err != nil {
			logger.Error("failed to assume data-access role", "role", cfg.DataStoreRoleARN, "error", err)
			os.Exit(1)
		}
		logger.Info("cross-account data access configured", "role", cfg.DataStoreRoleARN)
	}

	dynamoClient := dynamodb.NewFromConfig(dataStoreCfg)
	s3Client := s3.NewFromConfig(dataStoreCfg)
	execStore := store.NewExecutionStore(dynamoClient, cfg.ExecutionsTable, cfg.DynamoDBTTLDays)

	var restCfg *rest.Config
	var kubeClient kubernetes.Interface

	restCfg, err = eksauth.NewRESTConfig(cfg.EKSClusterEndpoint, cfg.EKSClusterCA, cfg.EKSClusterName, awsCfg)
	if err != nil {
		logger.Error("failed to build EKS REST config", "error", err,
			"endpoint", cfg.EKSClusterEndpoint)
		os.Exit(1)
	}

	kubeClient, err = kubernetes.NewForConfig(restCfg)
	if err != nil {
		logger.Error("failed to create kubernetes client", "error", err)
		os.Exit(1)
	}

	if err := handler.CheckStartupHealth(context.Background(), handler.HealthDeps{
		DynamoClient: dynamoClient,
		S3Client:     s3Client,
		KubeClient:   kubeClient,
		TableName:    cfg.ExecutionsTable,
		BucketName:   cfg.ArtifactBucket,
		Mode:         cfg.HandlerMode,
	}, logger); err != nil {
		logger.Error("startup health check failed — exiting", "error", err)
		os.Exit(1)
	}

	exec := executor.New(kubeClient, restCfg, s3Client, &awsCfg, executor.ExecutorConfig{
		ArtifactBucket:  cfg.ArtifactBucket,
		UploaderRoleARN: cfg.UploaderRoleARN,
		AWSReadRoleARN:  cfg.AWSReadRoleARN,
		AWSWriteRoleARN: cfg.AWSWriteRoleARN,
		KMSKeyARN:       cfg.KMSKeyARN,
		Region:          cfg.Region,
		JobImage:        cfg.JobImage,
	}, logger)

	switch cfg.HandlerMode {
	case "api":
		// API mode: start HTTP server on :8080 for Lambda Web Adapter (LWA).
		// LWA proxies Function URL requests to this server, enabling response streaming.
		auditStore := store.NewAuditStore(dynamoClient, cfg.AuditTable, cfg.DynamoDBTTLDays)
		apiHandler := api.New(cfg, execStore, auditStore, exec, s3Client, logger)

		logger.Info("API mode: starting HTTP server on :8080 for Lambda Web Adapter")
		if err := http.ListenAndServe(":8080", apiHandler); err != nil {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}

	case "worker":
		// Worker mode: native Lambda handler. Receives EventBridge schedules
		// and self-invocation events (no HTTP involved).
		lambdaClient := awslambda.NewFromConfig(awsCfg)
		reconciler := scheduler.NewReconciler(execStore, kubeClient, lambdaClient, exec, cfg, logger)

		h := handler.New(handler.Deps{
			Cfg:        cfg,
			Reconciler: reconciler,
			Executor:   exec,
			ExecStore:  execStore,
			Logger:     logger,
		})
		lambda.Start(h.HandleEvent)
	}
}
