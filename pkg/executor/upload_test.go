package executor

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// --- S3 Mock ---

type mockS3Client struct {
	putCalls    []s3.PutObjectInput
	err         error
	headResults map[string]*s3.HeadObjectOutput
	headErr     error
}

func (m *mockS3Client) PutObject(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	m.putCalls = append(m.putCalls, *params)
	return &s3.PutObjectOutput{}, m.err
}

func (m *mockS3Client) HeadObject(_ context.Context, params *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.headResults != nil && params.Key != nil {
		if result, ok := m.headResults[*params.Key]; ok {
			return result, nil
		}
		return nil, fmt.Errorf("NoSuchKey: key %s not found", *params.Key)
	}
	if m.headErr != nil {
		return nil, m.headErr
	}
	return &s3.HeadObjectOutput{}, m.err
}

// --- Tests ---

func TestUploadSyncArtifacts_WhenOutputAndLogs_ItShouldUploadBoth(t *testing.T) {
	mock := &mockS3Client{}
	e := &Executor{
		s3Client:       mock,
		artifactBucket: "zoa-artifacts-test",
		logger:         noopLogger(),
	}

	output := []byte(`{"nodes": [{"name": "node-1"}]}`)
	logs := []byte("time=2024-01-01T00:00:00Z level=INFO msg=\"execution started\"\n")

	outputLen, logLen := e.uploadSyncArtifacts(context.Background(), "exec-upload-1", output, logs, noopLogger())

	if outputLen != int64(len(output)) {
		t.Errorf("expected outputLen=%d, got %d", len(output), outputLen)
	}
	if logLen != int64(len(logs)) {
		t.Errorf("expected logLen=%d, got %d", len(logs), logLen)
	}
	if len(mock.putCalls) != 2 {
		t.Fatalf("expected 2 PutObject calls, got %d", len(mock.putCalls))
	}
	if *mock.putCalls[0].Key != "executions/exec-upload-1/output.json" {
		t.Errorf("expected output key, got %q", *mock.putCalls[0].Key)
	}
	if *mock.putCalls[1].Key != "executions/exec-upload-1/execution.log" {
		t.Errorf("expected log key, got %q", *mock.putCalls[1].Key)
	}
	if *mock.putCalls[0].Bucket != "zoa-artifacts-test" {
		t.Errorf("expected bucket 'zoa-artifacts-test', got %q", *mock.putCalls[0].Bucket)
	}
}

func TestUploadSyncArtifacts_WhenOnlyOutput_ItShouldUploadOutputOnly(t *testing.T) {
	mock := &mockS3Client{}
	e := &Executor{
		s3Client:       mock,
		artifactBucket: "zoa-artifacts-test",
		logger:         noopLogger(),
	}

	output := []byte(`{"result": "ok"}`)
	outputLen, logLen := e.uploadSyncArtifacts(context.Background(), "exec-output-only", output, nil, noopLogger())

	if outputLen != int64(len(output)) {
		t.Errorf("expected outputLen=%d, got %d", len(output), outputLen)
	}
	if logLen != 0 {
		t.Errorf("expected logLen=0, got %d", logLen)
	}
	if len(mock.putCalls) != 1 {
		t.Fatalf("expected 1 PutObject call, got %d", len(mock.putCalls))
	}
}

func TestUploadSyncArtifacts_WhenNoData_ItShouldNotUpload(t *testing.T) {
	mock := &mockS3Client{}
	e := &Executor{
		s3Client:       mock,
		artifactBucket: "zoa-artifacts-test",
		logger:         noopLogger(),
	}

	outputLen, logLen := e.uploadSyncArtifacts(context.Background(), "exec-empty", nil, nil, noopLogger())

	if outputLen != 0 || logLen != 0 {
		t.Errorf("expected 0/0, got %d/%d", outputLen, logLen)
	}
	if len(mock.putCalls) != 0 {
		t.Errorf("expected 0 PutObject calls, got %d", len(mock.putCalls))
	}
}

func TestUploadSyncArtifacts_WhenS3Fails_ItShouldReturnZeroLengths(t *testing.T) {
	mock := &mockS3Client{err: fmt.Errorf("access denied")}
	e := &Executor{
		s3Client:       mock,
		artifactBucket: "zoa-artifacts-test",
		logger:         noopLogger(),
	}

	output := []byte(`{"result": "ok"}`)
	outputLen, logLen := e.uploadSyncArtifacts(context.Background(), "exec-fail", output, []byte("logs"), noopLogger())

	if outputLen != 0 {
		t.Errorf("expected outputLen=0 on S3 failure, got %d", outputLen)
	}
	if logLen != 0 {
		t.Errorf("expected logLen=0 on S3 failure, got %d", logLen)
	}
}

func TestMarshalErrorOutput_WhenError_ItShouldReturnJSON(t *testing.T) {
	data := marshalErrorOutput(fmt.Errorf("test error"))
	if string(data) != `{"error":"test error"}` {
		t.Errorf("unexpected error output: %s", data)
	}
}

func TestMarshalParamsEnv_WhenParamsExist_ItShouldReturnJSON(t *testing.T) {
	result := marshalParamsEnv(map[string]string{"name": "pod-1", "namespace": "grafana"})
	if result == "{}" {
		t.Error("expected non-empty params JSON")
	}
}

func TestMarshalParamsEnv_WhenEmpty_ItShouldReturnEmptyObject(t *testing.T) {
	result := marshalParamsEnv(nil)
	if result != "{}" {
		t.Errorf("expected '{}', got %q", result)
	}
}

func TestArtifactSizes_WhenJSONOutput_ItShouldReturnJSONFormat(t *testing.T) {
	outputSize := int64(617)
	logSize := int64(780)
	mock := &mockS3Client{
		headResults: map[string]*s3.HeadObjectOutput{
			"executions/exec-1/output.json":   {ContentLength: &outputSize},
			"executions/exec-1/execution.log": {ContentLength: &logSize},
		},
	}
	e := &Executor{s3Client: mock, artifactBucket: "test-bucket", logger: noopLogger()}

	outBytes, lgBytes, format := e.ArtifactSizes(context.Background(), "exec-1")

	if outBytes != 617 {
		t.Errorf("expected outputBytes=617, got %d", outBytes)
	}
	if lgBytes != 780 {
		t.Errorf("expected logBytes=780, got %d", lgBytes)
	}
	if format != "json" {
		t.Errorf("expected format='json', got %q", format)
	}
}

func TestArtifactSizes_WhenTarGzOutput_ItShouldReturnTarGzFormat(t *testing.T) {
	outputSize := int64(102400)
	logSize := int64(1024)
	mock := &mockS3Client{
		headResults: map[string]*s3.HeadObjectOutput{
			"executions/exec-2/output.tar.gz": {ContentLength: &outputSize},
			"executions/exec-2/execution.log": {ContentLength: &logSize},
		},
	}
	e := &Executor{s3Client: mock, artifactBucket: "test-bucket", logger: noopLogger()}

	outBytes, lgBytes, format := e.ArtifactSizes(context.Background(), "exec-2")

	if outBytes != 102400 {
		t.Errorf("expected outputBytes=102400, got %d", outBytes)
	}
	if lgBytes != 1024 {
		t.Errorf("expected logBytes=1024, got %d", lgBytes)
	}
	if format != "tar.gz" {
		t.Errorf("expected format='tar.gz', got %q", format)
	}
}

func TestArtifactSizes_WhenNoArtifacts_ItShouldReturnZeros(t *testing.T) {
	mock := &mockS3Client{
		headResults: map[string]*s3.HeadObjectOutput{},
	}
	e := &Executor{s3Client: mock, artifactBucket: "test-bucket", logger: noopLogger()}

	outBytes, lgBytes, format := e.ArtifactSizes(context.Background(), "exec-missing")

	if outBytes != 0 {
		t.Errorf("expected outputBytes=0, got %d", outBytes)
	}
	if lgBytes != 0 {
		t.Errorf("expected logBytes=0, got %d", lgBytes)
	}
	if format != "" {
		t.Errorf("expected empty format, got %q", format)
	}
}

func TestArtifactSizes_WhenOnlyLogs_ItShouldReturnLogsWithEmptyFormat(t *testing.T) {
	logSize := int64(500)
	mock := &mockS3Client{
		headResults: map[string]*s3.HeadObjectOutput{
			"executions/exec-3/execution.log": {ContentLength: &logSize},
		},
	}
	e := &Executor{s3Client: mock, artifactBucket: "test-bucket", logger: noopLogger()}

	outBytes, lgBytes, format := e.ArtifactSizes(context.Background(), "exec-3")

	if outBytes != 0 {
		t.Errorf("expected outputBytes=0, got %d", outBytes)
	}
	if lgBytes != 500 {
		t.Errorf("expected logBytes=500, got %d", lgBytes)
	}
	if format != "" {
		t.Errorf("expected empty format (no output), got %q", format)
	}
}
