package actions

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
)

type mockEKSClient struct {
	clusters []string
	cluster  *ekstypes.Cluster
	err      error
}

func (m *mockEKSClient) ListClusters(_ context.Context, _ *eks.ListClustersInput, _ ...func(*eks.Options)) (*eks.ListClustersOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &eks.ListClustersOutput{Clusters: m.clusters}, nil
}

func (m *mockEKSClient) DescribeCluster(_ context.Context, params *eks.DescribeClusterInput, _ ...func(*eks.Options)) (*eks.DescribeClusterOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &eks.DescribeClusterOutput{Cluster: m.cluster}, nil
}

func eksTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestListEKSClusters_WhenClustersExist_ItShouldReturnList(t *testing.T) {
	mock := &mockEKSClient{clusters: []string{"cluster-1", "cluster-2", "cluster-3"}}
	params := &ExecutionParams{
		Params: map[string]string{},
		Logger: eksTestLogger(),
	}

	result, err := listClusters(context.Background(), mock, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	output := result.Output.(map[string]interface{})
	if output["count"] != 3 {
		t.Errorf("expected count=3, got %v", output["count"])
	}
}

func TestListEKSClusters_WhenNoClusters_ItShouldReturnEmpty(t *testing.T) {
	mock := &mockEKSClient{clusters: []string{}}
	params := &ExecutionParams{
		Params: map[string]string{},
		Logger: eksTestLogger(),
	}

	result, err := listClusters(context.Background(), mock, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := result.Output.(map[string]interface{})
	if output["count"] != 0 {
		t.Errorf("expected count=0, got %v", output["count"])
	}
}

func TestListEKSClusters_WhenAPIFails_ItShouldReturnError(t *testing.T) {
	mock := &mockEKSClient{err: fmt.Errorf("access denied")}
	params := &ExecutionParams{
		Params: map[string]string{},
		Logger: eksTestLogger(),
	}

	_, err := listClusters(context.Background(), mock, params)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDescribeEKSCluster_WhenClusterExists_ItShouldReturnDetails(t *testing.T) {
	mock := &mockEKSClient{
		cluster: &ekstypes.Cluster{
			Name:            aws.String("my-cluster"),
			Arn:             aws.String("arn:aws:eks:us-east-1:123456:cluster/my-cluster"),
			Status:          ekstypes.ClusterStatusActive,
			Version:         aws.String("1.29"),
			PlatformVersion: aws.String("eks.5"),
			Endpoint:        aws.String("https://ABCDEF.gr7.us-east-1.eks.amazonaws.com"),
			RoleArn:         aws.String("arn:aws:iam::123456:role/eks-role"),
		},
	}
	params := &ExecutionParams{
		Params: map[string]string{"name": "my-cluster"},
		Logger: eksTestLogger(),
	}

	result, err := describeCluster(context.Background(), mock, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	output := result.Output.(map[string]interface{})
	if output["name"] != "my-cluster" {
		t.Errorf("expected name 'my-cluster', got %v", output["name"])
	}
	if output["status"] != "ACTIVE" {
		t.Errorf("expected status ACTIVE, got %v", output["status"])
	}
}

func TestDescribeEKSCluster_WhenAPIFails_ItShouldReturnError(t *testing.T) {
	mock := &mockEKSClient{err: fmt.Errorf("cluster not found")}
	params := &ExecutionParams{
		Params: map[string]string{"name": "nonexistent"},
		Logger: eksTestLogger(),
	}

	_, err := describeCluster(context.Background(), mock, params)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDescribeEKSCluster_WhenValidation_NameRequired(t *testing.T) {
	action := &describeEKSCluster{}
	awsCfg := &aws.Config{Region: "us-east-1"}
	params := &ExecutionParams{
		Params:    map[string]string{},
		AWSConfig: awsCfg,
		Logger:    eksTestLogger(),
	}
	if err := action.Validate(context.Background(), params); err == nil {
		t.Fatal("expected validation error for missing name")
	}
}

func TestListEKSClusters_WhenValidation_AWSConfigRequired(t *testing.T) {
	action := &listEKSClusters{}
	params := &ExecutionParams{
		Params: map[string]string{},
		Logger: eksTestLogger(),
	}
	if err := action.Validate(context.Background(), params); err == nil {
		t.Fatal("expected validation error for missing AWS config")
	}
}
