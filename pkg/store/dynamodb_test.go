package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type mockDynamoDBAPI struct {
	putItemFn    func(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	getItemFn    func(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	queryFn      func(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	updateItemFn func(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
}

func (m *mockDynamoDBAPI) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if m.putItemFn != nil {
		return m.putItemFn(ctx, params, optFns...)
	}
	return &dynamodb.PutItemOutput{}, nil
}

func (m *mockDynamoDBAPI) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if m.getItemFn != nil {
		return m.getItemFn(ctx, params, optFns...)
	}
	return &dynamodb.GetItemOutput{}, nil
}

func (m *mockDynamoDBAPI) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if m.queryFn != nil {
		return m.queryFn(ctx, params, optFns...)
	}
	return &dynamodb.QueryOutput{}, nil
}

func (m *mockDynamoDBAPI) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	if m.updateItemFn != nil {
		return m.updateItemFn(ctx, params, optFns...)
	}
	return &dynamodb.UpdateItemOutput{}, nil
}

// --- ExecutionStore tests using mocked DynamoDB ---

func TestDynamoDBExecutionStore_Create_WhenSuccess_ItShouldSetTTLAndTargetStatusKey(t *testing.T) {
	var capturedInput *dynamodb.PutItemInput
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			capturedInput = params
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	exec := &Execution{
		ID:        "exec-1",
		Action:    "get_pods",
		AccountID: "123456",
		Status:    StatusDispatched,
		CreatedAt: time.Now().Format(time.RFC3339Nano),
	}

	err := s.Create(context.Background(), exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedInput == nil {
		t.Fatal("PutItem was not called")
	}
	if *capturedInput.TableName != "test-table" {
		t.Errorf("expected table 'test-table', got %q", *capturedInput.TableName)
	}
	if exec.TTL == 0 {
		t.Error("expected TTL to be set")
	}
	if exec.TargetStatusKey == "" {
		t.Error("expected TargetStatusKey to be set")
	}
	expectedPrefix := string(StatusDispatched) + "#"
	if len(exec.TargetStatusKey) < len(expectedPrefix) {
		t.Errorf("TargetStatusKey %q doesn't start with %q", exec.TargetStatusKey, expectedPrefix)
	}
}

func TestDynamoDBExecutionStore_Create_WhenConflict_ItShouldReturnError(t *testing.T) {
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, _ *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			return nil, &types.ConditionalCheckFailedException{Message: ptrStr("The conditional request failed")}
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	exec := &Execution{ID: "dup-1", Status: StatusDispatched, CreatedAt: time.Now().Format(time.RFC3339Nano)}

	err := s.Create(context.Background(), exec)
	if err == nil {
		t.Fatal("expected error for conditional check failure")
	}
}

func TestDynamoDBExecutionStore_Get_WhenItemExists_ItShouldReturnExecution(t *testing.T) {
	item, _ := attributevalue.MarshalMap(&Execution{
		ID:        "exec-get-1",
		Action:    "get_pods",
		Status:    StatusSucceeded,
		CreatedAt: "2024-06-15T10:00:00Z",
	})

	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, params *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			key := params.Key["executionId"].(*types.AttributeValueMemberS)
			if key.Value != "exec-get-1" {
				t.Errorf("expected key 'exec-get-1', got %q", key.Value)
			}
			return &dynamodb.GetItemOutput{Item: item}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	exec, err := s.Get(context.Background(), "exec-get-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec == nil {
		t.Fatal("expected execution, got nil")
	}
	if exec.ID != "exec-get-1" {
		t.Errorf("expected ID 'exec-get-1', got %q", exec.ID)
	}
	if exec.Status != StatusSucceeded {
		t.Errorf("expected status 'succeeded', got %q", exec.Status)
	}
}

func TestDynamoDBExecutionStore_Get_WhenItemNotFound_ItShouldReturnNil(t *testing.T) {
	mock := &mockDynamoDBAPI{
		getItemFn: func(_ context.Context, _ *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	exec, err := s.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exec != nil {
		t.Fatalf("expected nil, got %+v", exec)
	}
}

func TestDynamoDBExecutionStore_TransitionStatus_WhenSuccess_ItShouldCallUpdateItem(t *testing.T) {
	var capturedInput *dynamodb.UpdateItemInput
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			capturedInput = params
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	err := s.TransitionStatus(context.Background(), "exec-t1", StatusDispatched, StatusSucceeded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("UpdateItem was not called")
	}
	if *capturedInput.TableName != "test-table" {
		t.Errorf("expected table 'test-table', got %q", *capturedInput.TableName)
	}
	key := capturedInput.Key["executionId"].(*types.AttributeValueMemberS)
	if key.Value != "exec-t1" {
		t.Errorf("expected key 'exec-t1', got %q", key.Value)
	}
}

func TestDynamoDBExecutionStore_TransitionStatus_WhenConditionFails_ItShouldReturnError(t *testing.T) {
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, _ *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			return nil, &types.ConditionalCheckFailedException{Message: ptrStr("condition not met")}
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	err := s.TransitionStatus(context.Background(), "exec-t2", StatusDispatched, StatusSucceeded)
	if err == nil {
		t.Fatal("expected error for conditional check failure")
	}
}

func TestDynamoDBExecutionStore_QueryByStatus_WhenItemsExist_ItShouldReturnAll(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	for _, exec := range []*Execution{
		{ID: "e1", Status: StatusDispatched, CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: "e2", Status: StatusDispatched, CreatedAt: "2024-01-01T00:01:00Z"},
	} {
		item, _ := attributevalue.MarshalMap(exec)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: items, LastEvaluatedKey: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.QueryByStatus(context.Background(), StatusDispatched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_MarkCleaned_WhenSuccess_ItShouldCallUpdateItem(t *testing.T) {
	called := false
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			called = true
			key := params.Key["executionId"].(*types.AttributeValueMemberS)
			if key.Value != "exec-clean" {
				t.Errorf("expected key 'exec-clean', got %q", key.Value)
			}
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	err := s.MarkCleaned(context.Background(), "exec-clean")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("UpdateItem was not called")
	}
}

// --- AuditStore tests using mocked DynamoDB ---

func TestDynamoDBAuditStore_Record_WhenSuccess_ItShouldSetTTL(t *testing.T) {
	var capturedInput *dynamodb.PutItemInput
	mock := &mockDynamoDBAPI{
		putItemFn: func(_ context.Context, params *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			capturedInput = params
			return &dynamodb.PutItemOutput{}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	entry := &AuditEntry{
		AccountID:  "123456",
		Timestamp:  time.Now().Format(time.RFC3339Nano),
		Method:     "POST",
		Path:       "/api/v0/trusted-actions/get_pods/run",
		StatusCode: 200,
		Operator:   "arn:aws:iam::123456:user/test",
	}

	err := s.Record(context.Background(), entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("PutItem was not called")
	}
	if *capturedInput.TableName != "audit-table" {
		t.Errorf("expected table 'audit-table', got %q", *capturedInput.TableName)
	}
	if entry.TTL == 0 {
		t.Error("expected TTL to be set")
	}
}

func TestDynamoDBAuditStore_List_WhenItemsExist_ItShouldReturnEntries(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	for _, entry := range []*AuditEntry{
		{AccountID: "123", Timestamp: "2024-06-01T10:00:00Z", Method: "GET", StatusCode: 200},
		{AccountID: "123", Timestamp: "2024-06-01T10:01:00Z", Method: "POST", StatusCode: 200},
	} {
		item, _ := attributevalue.MarshalMap(entry)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	results, err := s.List(context.Background(), "123", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 entries, got %d", len(results))
	}
}

func TestDynamoDBAuditStore_List_WhenFilterApplied_ItShouldRespectLimit(t *testing.T) {
	items := make([]map[string]types.AttributeValue, 0, 10)
	for i := 0; i < 10; i++ {
		entry := &AuditEntry{
			AccountID: "123",
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano),
			Method:    "POST",
			Path:      fmt.Sprintf("/api/v0/trusted-actions/action%d/run", i),
		}
		item, _ := attributevalue.MarshalMap(entry)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	since := time.Now().Add(-24 * time.Hour)
	filter := &AuditFilter{
		Since: &since,
		Limit: 5,
	}
	results, err := s.List(context.Background(), "123", filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 results (limit=5), got %d", len(results))
	}
}

// --- Additional ExecutionStore query tests ---

func TestDynamoDBExecutionStore_List_WhenItemsExist_ItShouldReturnAll(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	for _, exec := range []*Execution{
		{ID: "e1", AccountID: "acc1", Status: StatusSucceeded, CreatedAt: "2024-01-01T00:00:00Z"},
		{ID: "e2", AccountID: "acc1", Status: StatusFailed, CreatedAt: "2024-01-01T01:00:00Z"},
	} {
		item, _ := attributevalue.MarshalMap(exec)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "account-index" {
				t.Errorf("expected account-index, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.List(context.Background(), "acc1", 50, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_List_WhenQueryFails_ItShouldReturnError(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return nil, fmt.Errorf("dynamodb unavailable")
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	_, err := s.List(context.Background(), "acc1", 50, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDynamoDBExecutionStore_TransitionWithMetadata_WhenSuccess_ItShouldIncludeExtraFields(t *testing.T) {
	var capturedInput *dynamodb.UpdateItemInput
	mock := &mockDynamoDBAPI{
		updateItemFn: func(_ context.Context, params *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			capturedInput = params
			return &dynamodb.UpdateItemOutput{}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	err := s.TransitionWithMetadata(context.Background(), "exec-1", StatusDispatched, StatusSucceeded, map[string]interface{}{
		"summary": "completed successfully",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedInput == nil {
		t.Fatal("UpdateItem was not called")
	}
	key := capturedInput.Key["executionId"].(*types.AttributeValueMemberS)
	if key.Value != "exec-1" {
		t.Errorf("expected key 'exec-1', got %q", key.Value)
	}
}

func TestDynamoDBExecutionStore_QueryByStatusAndClass_WhenSuccess_ItShouldFilterByClass(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	exec := &Execution{ID: "e1", Status: StatusDispatched, ExecutionMode: "sync", CreatedAt: "2024-01-01T00:00:00Z"}
	item, _ := attributevalue.MarshalMap(exec)
	items = append(items, item)

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.FilterExpression == nil {
				t.Error("expected filter expression for class filter")
			}
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.QueryByStatusAndClass(context.Background(), StatusDispatched, "sync")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_QueryTerminal_WhenSuccess_ItShouldQueryAllTerminalStatuses(t *testing.T) {
	queryCalls := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCalls++
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.QueryTerminal(context.Background(), 1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
	// Should query for succeeded, failed, and timedOut
	if queryCalls != 3 {
		t.Errorf("expected 3 query calls (one per terminal status), got %d", queryCalls)
	}
}

func TestDynamoDBExecutionStore_ListByTargetAndAction_WhenSuccess_ItShouldUseTargetIndex(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "target-index" {
				t.Errorf("expected target-index, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	_, err := s.ListByTargetAndAction(context.Background(), "cluster-1", "delete_pod", time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBExecutionStore_CountActiveByTarget_WhenItemsExist_ItShouldReturnCount(t *testing.T) {
	var queryCount int
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			queryCount++
			if params.Select != types.SelectCount {
				t.Error("expected SelectCount")
			}
			if *params.IndexName != "target-status-index" {
				t.Errorf("expected target-status-index, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Count: 3}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	count, err := s.CountActiveByTarget(context.Background(), "cluster-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if queryCount != 2 {
		t.Errorf("expected 2 queries (dispatched + approved), got %d", queryCount)
	}
	if count != 6 {
		t.Errorf("expected count=6 (3+3), got %d", count)
	}
}

func TestDynamoDBExecutionStore_QueryByTargetAndStatus_WhenSuccess_ItShouldUseTargetStatusIndex(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "target-status-index" {
				t.Errorf("expected target-status-index, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	_, err := s.QueryByTargetAndStatus(context.Background(), "cluster-1", StatusDispatched)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBExecutionStore_QueryTerminalByTarget_WhenSuccess_ItShouldFilterOldUncleaned(t *testing.T) {
	oldTime := time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano)

	// Only return items for "succeeded" status queries (simulating that
	// only one terminal status has a matching execution)
	succeededItems := []map[string]types.AttributeValue{}
	exec := &Execution{ID: "e1", Status: StatusSucceeded, CreatedAt: oldTime, Cleaned: false, TargetCluster: "cluster-1"}
	item, _ := attributevalue.MarshalMap(exec)
	succeededItems = append(succeededItems, item)

	callCount := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			callCount++
			// First call is for "succeeded" status
			if callCount == 1 {
				return &dynamodb.QueryOutput{Items: succeededItems}, nil
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.QueryTerminalByTarget(context.Background(), "cluster-1", 1*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

// --- Performance and GSI routing tests ---

func TestDynamoDBExecutionStore_List_WhenStatusFilter_ItShouldUseStatusIndex(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	exec := &Execution{ID: "e1", AccountID: "acc1", Status: StatusFailed, CreatedAt: "2024-01-01T00:00:00Z"}
	item, _ := attributevalue.MarshalMap(exec)
	items = append(items, item)

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "status-index" {
				t.Errorf("expected status-index GSI for status filter, got %q", *params.IndexName)
			}
			if params.FilterExpression == nil {
				t.Error("expected filter expression containing accountId")
			}
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	status := StatusFailed
	filter := &ListFilter{Status: &status}
	results, err := s.List(context.Background(), "acc1", 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_List_WhenNoStatusFilter_ItShouldUseAccountIndex(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "account-index" {
				t.Errorf("expected account-index GSI for non-status filter, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	mode := "async"
	filter := &ListFilter{ExecutionMode: &mode}
	_, err := s.List(context.Background(), "acc1", 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBExecutionStore_List_WhenMaxPagesReached_ItShouldStopPaginating(t *testing.T) {
	pageCount := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			pageCount++
			item, _ := attributevalue.MarshalMap(&Execution{
				ID: fmt.Sprintf("e%d", pageCount), AccountID: "acc1", Status: StatusSucceeded,
				CreatedAt: time.Now().Add(-time.Duration(pageCount) * time.Minute).Format(time.RFC3339Nano),
			})
			return &dynamodb.QueryOutput{
				Items:            []map[string]types.AttributeValue{item},
				LastEvaluatedKey: map[string]types.AttributeValue{"executionId": &types.AttributeValueMemberS{Value: fmt.Sprintf("e%d", pageCount)}},
			}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	results, err := s.List(context.Background(), "acc1", 1000, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageCount != 10 {
		t.Errorf("expected max 10 pages, got %d", pageCount)
	}
	if len(results) != 10 {
		t.Errorf("expected 10 results (1 per page, 10 pages max), got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_List_WhenLimitReached_ItShouldStopEarly(t *testing.T) {
	items := make([]map[string]types.AttributeValue, 0, 20)
	for i := 0; i < 20; i++ {
		exec := &Execution{
			ID: fmt.Sprintf("e%d", i), AccountID: "acc1", Status: StatusSucceeded,
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Minute).Format(time.RFC3339Nano),
		}
		item, _ := attributevalue.MarshalMap(exec)
		items = append(items, item)
	}

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	filter := &ListFilter{Limit: 5}
	results, err := s.List(context.Background(), "acc1", 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 5 {
		t.Errorf("expected 5 results (limit=5), got %d", len(results))
	}
}

func TestDynamoDBExecutionStore_List_WhenStatusAndSinceFilter_ItShouldUseStatusIndexWithTimeRange(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if *params.IndexName != "status-index" {
				t.Errorf("expected status-index, got %q", *params.IndexName)
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewExecutionStore(mock, "test-table", 30)
	status := StatusFailed
	since := time.Now().Add(-1 * time.Hour)
	filter := &ListFilter{Status: &status, Since: &since}
	_, err := s.List(context.Background(), "acc1", 50, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBAuditStore_List_WhenMaxPagesReached_ItShouldStopPaginating(t *testing.T) {
	pageCount := 0
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			pageCount++
			entry := &AuditEntry{
				AccountID: "123",
				Timestamp: time.Now().Add(-time.Duration(pageCount) * time.Minute).Format(time.RFC3339Nano),
				Method:    "GET",
			}
			item, _ := attributevalue.MarshalMap(entry)
			return &dynamodb.QueryOutput{
				Items:            []map[string]types.AttributeValue{item},
				LastEvaluatedKey: map[string]types.AttributeValue{"accountId": &types.AttributeValueMemberS{Value: "123"}},
			}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	filter := &AuditFilter{Limit: 1000}
	results, err := s.List(context.Background(), "123", filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pageCount != 10 {
		t.Errorf("expected max 10 pages, got %d", pageCount)
	}
	if len(results) != 10 {
		t.Errorf("expected 10 results (1 per page, 10 pages max), got %d", len(results))
	}
}

func TestDynamoDBAuditStore_List_WhenBeforeFilter_ItShouldUseKeyCondition(t *testing.T) {
	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.KeyConditionExpression == nil {
				t.Error("expected key condition expression")
			}
			return &dynamodb.QueryOutput{Items: nil}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	before := time.Now()
	since := time.Now().Add(-1 * time.Hour)
	filter := &AuditFilter{Since: &since, Before: &before}
	_, err := s.List(context.Background(), "123", filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamoDBAuditStore_List_WhenMethodFilter_ItShouldApplyFilterExpression(t *testing.T) {
	items := []map[string]types.AttributeValue{}
	entry := &AuditEntry{AccountID: "123", Timestamp: "2024-01-01T00:00:00Z", Method: "POST"}
	item, _ := attributevalue.MarshalMap(entry)
	items = append(items, item)

	mock := &mockDynamoDBAPI{
		queryFn: func(_ context.Context, params *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
			if params.FilterExpression == nil {
				t.Error("expected filter expression for method filter")
			}
			return &dynamodb.QueryOutput{Items: items}, nil
		},
	}

	s := NewAuditStore(mock, "audit-table", 90)
	method := "POST"
	filter := &AuditFilter{Method: &method}
	results, err := s.List(context.Background(), "123", filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func ptrStr(s string) *string { return &s }
