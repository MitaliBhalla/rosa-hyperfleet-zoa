package actions

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type mockEC2Client struct {
	endpoints []ec2types.VpcEndpoint
	err       error
}

func (m *mockEC2Client) DescribeVpcEndpoints(_ context.Context, params *ec2.DescribeVpcEndpointsInput, _ ...func(*ec2.Options)) (*ec2.DescribeVpcEndpointsOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	if len(params.VpcEndpointIds) > 0 {
		var filtered []ec2types.VpcEndpoint
		for _, ep := range m.endpoints {
			for _, id := range params.VpcEndpointIds {
				if aws.ToString(ep.VpcEndpointId) == id {
					filtered = append(filtered, ep)
				}
			}
		}
		return &ec2.DescribeVpcEndpointsOutput{VpcEndpoints: filtered}, nil
	}
	return &ec2.DescribeVpcEndpointsOutput{VpcEndpoints: m.endpoints}, nil
}

func vpcTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}

func TestListVPCEndpoints_WhenEndpointsExist_ItShouldReturnList(t *testing.T) {
	mock := &mockEC2Client{
		endpoints: []ec2types.VpcEndpoint{
			{VpcEndpointId: aws.String("vpce-001"), ServiceName: aws.String("com.amazonaws.s3"), State: ec2types.StateAvailable},
			{VpcEndpointId: aws.String("vpce-002"), ServiceName: aws.String("com.amazonaws.sts"), State: ec2types.StateAvailable},
		},
	}
	params := &ExecutionParams{
		Params: map[string]string{},
		Logger: vpcTestLogger(),
	}

	result, err := listEndpoints(context.Background(), mock, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	output := result.Output.(map[string]interface{})
	if output["count"] != 2 {
		t.Errorf("expected count=2, got %v", output["count"])
	}
}

func TestListVPCEndpoints_WhenNoEndpoints_ItShouldReturnEmpty(t *testing.T) {
	mock := &mockEC2Client{endpoints: []ec2types.VpcEndpoint{}}
	params := &ExecutionParams{
		Params: map[string]string{},
		Logger: vpcTestLogger(),
	}

	result, err := listEndpoints(context.Background(), mock, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := result.Output.(map[string]interface{})
	if output["count"] != 0 {
		t.Errorf("expected count=0, got %v", output["count"])
	}
}

func TestListVPCEndpoints_WhenAPIFails_ItShouldReturnError(t *testing.T) {
	mock := &mockEC2Client{err: fmt.Errorf("access denied")}
	params := &ExecutionParams{
		Params: map[string]string{},
		Logger: vpcTestLogger(),
	}

	_, err := listEndpoints(context.Background(), mock, params)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDescribeVPCEndpoint_WhenEndpointExists_ItShouldReturnDetails(t *testing.T) {
	mock := &mockEC2Client{
		endpoints: []ec2types.VpcEndpoint{
			{
				VpcEndpointId:   aws.String("vpce-abc123"),
				VpcId:           aws.String("vpc-001"),
				ServiceName:     aws.String("com.amazonaws.us-east-1.s3"),
				State:           ec2types.StateAvailable,
				VpcEndpointType: ec2types.VpcEndpointTypeGateway,
			},
		},
	}
	params := &ExecutionParams{
		Params: map[string]string{"name": "vpce-abc123"},
		Logger: vpcTestLogger(),
	}

	result, err := describeEndpoint(context.Background(), mock, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	output := result.Output.(map[string]interface{})
	if output["id"] != "vpce-abc123" {
		t.Errorf("expected id 'vpce-abc123', got %v", output["id"])
	}
}

func TestDescribeVPCEndpoint_WhenNotFound_ItShouldReturnError(t *testing.T) {
	mock := &mockEC2Client{endpoints: []ec2types.VpcEndpoint{}}
	params := &ExecutionParams{
		Params: map[string]string{"name": "vpce-nonexist"},
		Logger: vpcTestLogger(),
	}

	_, err := describeEndpoint(context.Background(), mock, params)
	if err == nil {
		t.Fatal("expected error for not found")
	}
}

func TestDescribeVPCEndpoint_WhenValidation_NameRequired(t *testing.T) {
	action := &describeVPCEndpoint{}
	awsCfg := &aws.Config{Region: "us-east-1"}
	params := &ExecutionParams{
		Params:    map[string]string{},
		AWSConfig: awsCfg,
		Logger:    vpcTestLogger(),
	}
	if err := action.Validate(context.Background(), params); err == nil {
		t.Fatal("expected validation error for missing name")
	}
}

func TestDescribeVPCEndpoint_WhenValidation_InvalidPrefix(t *testing.T) {
	action := &describeVPCEndpoint{}
	awsCfg := &aws.Config{Region: "us-east-1"}
	params := &ExecutionParams{
		Params:    map[string]string{"name": "not-a-vpce-id"},
		AWSConfig: awsCfg,
		Logger:    vpcTestLogger(),
	}
	if err := action.Validate(context.Background(), params); err == nil {
		t.Fatal("expected validation error for invalid vpce prefix")
	}
}

func TestListVPCEndpoints_WhenValidation_AWSConfigRequired(t *testing.T) {
	action := &listVPCEndpoints{}
	params := &ExecutionParams{
		Params: map[string]string{},
		Logger: vpcTestLogger(),
	}
	if err := action.Validate(context.Background(), params); err == nil {
		t.Fatal("expected validation error for missing AWS config")
	}
}
