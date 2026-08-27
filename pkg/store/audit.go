package store

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type AuditStore interface {
	Record(ctx context.Context, entry *AuditEntry) error
	List(ctx context.Context, accountID string, filter *AuditFilter) ([]*AuditEntry, error)

	// ListAll returns audit entries across all accounts via date-bucket-index.
	// All optional filters (target, action, method, etc.) are applied as
	// DynamoDB FilterExpressions server-side.
	ListAll(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error)
}

type DynamoDBAuditStore struct {
	client    DynamoDBAPI
	tableName string
	ttlDays   int
}

func NewAuditStore(client DynamoDBAPI, tableName string, ttlDays int) *DynamoDBAuditStore {
	return &DynamoDBAuditStore{
		client:    client,
		tableName: tableName,
		ttlDays:   ttlDays,
	}
}

func (s *DynamoDBAuditStore) Record(ctx context.Context, entry *AuditEntry) error {
	entry.TTL = time.Now().AddDate(0, 0, s.ttlDays).Unix()
	entry.DateBucket = entry.Timestamp[:10]

	item, err := attributevalue.MarshalMap(entry)
	if err != nil {
		return fmt.Errorf("marshaling audit entry: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &s.tableName,
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("recording audit entry: %w", err)
	}
	return nil
}

func (s *DynamoDBAuditStore) List(ctx context.Context, accountID string, filter *AuditFilter) ([]*AuditEntry, error) {
	keyCond := expression.Key("accountId").Equal(expression.Value(accountID))

	if filter != nil {
		switch {
		case filter.Since != nil && filter.Before != nil:
			keyCond = expression.KeyAnd(
				keyCond,
				expression.Key("timestamp").Between(
					expression.Value(filter.Since.Format(time.RFC3339Nano)),
					expression.Value(filter.Before.Format(time.RFC3339Nano)),
				),
			)
		case filter.Since != nil:
			keyCond = expression.KeyAnd(
				keyCond,
				expression.Key("timestamp").GreaterThanEqual(expression.Value(filter.Since.Format(time.RFC3339Nano))),
			)
		case filter.Before != nil:
			keyCond = expression.KeyAnd(
				keyCond,
				expression.Key("timestamp").LessThanEqual(expression.Value(filter.Before.Format(time.RFC3339Nano))),
			)
		}
	}

	builder := expression.NewBuilder().WithKeyCondition(keyCond)

	if filter != nil {
		var conditions []expression.ConditionBuilder
		if filter.Action != nil {
			conditions = append(conditions, expression.Name("action").Equal(expression.Value(*filter.Action)))
		}
		if filter.Method != nil {
			conditions = append(conditions, expression.Name("method").Equal(expression.Value(*filter.Method)))
		}
		if filter.Operator != nil {
			conditions = append(conditions, expression.Name("operator").Contains(*filter.Operator))
		}
		if filter.Force != nil {
			conditions = append(conditions, expression.Name("force").Equal(expression.Value(*filter.Force)))
		}
		if filter.DryRun != nil {
			conditions = append(conditions, expression.Name("dryRun").Equal(expression.Value(*filter.DryRun)))
		}
		if len(conditions) > 0 {
			combined := conditions[0]
			for _, c := range conditions[1:] {
				combined = combined.And(c)
			}
			builder = builder.WithFilter(combined)
		}
	}

	expr, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("building query expression: %w", err)
	}

	resultLimit := 100
	if filter != nil && filter.Limit > 0 {
		resultLimit = filter.Limit
	}

	const maxPages = 10

	var entries []*AuditEntry
	var lastKey map[string]types.AttributeValue
	for page := 0; page < maxPages; page++ {
		input := &dynamodb.QueryInput{
			TableName:                 &s.tableName,
			KeyConditionExpression:    expr.KeyCondition(),
			ExpressionAttributeNames:  expr.Names(),
			ExpressionAttributeValues: expr.Values(),
			FilterExpression:          expr.Filter(),
			ScanIndexForward:          aws.Bool(false),
			ExclusiveStartKey:         lastKey,
		}

		out, err := s.client.Query(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("querying audit entries: %w", err)
		}

		for _, item := range out.Items {
			var entry AuditEntry
			if err := attributevalue.UnmarshalMap(item, &entry); err != nil {
				return nil, fmt.Errorf("unmarshaling audit entry: %w", err)
			}
			entries = append(entries, &entry)
		}

		if len(entries) >= resultLimit {
			entries = entries[:resultLimit]
			break
		}

		if out.LastEvaluatedKey == nil {
			break
		}
		lastKey = out.LastEvaluatedKey
	}
	return entries, nil
}

// ListAll returns audit entries across all accounts via date-bucket-index GSI
// (PK=dateBucket, SK=timestamp). All optional filters (target, action, method,
// etc.) are applied as DynamoDB FilterExpressions on the server side.
//
// The date-bucket-index path requires a time bound (filter.Since) to function.
// The CLI enforces a default of 24h, ensuring no unbounded queries ever reach DynamoDB.
func (s *DynamoDBAuditStore) ListAll(ctx context.Context, filter *AuditFilter) ([]*AuditEntry, error) {
	resultLimit := 100
	if filter != nil && filter.Limit > 0 {
		resultLimit = filter.Limit
	}

	return s.listAllByDateBucket(ctx, filter, resultLimit)
}

// listAllByDateBucket queries the date-bucket-index GSI day-by-day from today
// (or filter.Before) backwards until the Since boundary. Each bucket query returns
// items sorted by timestamp descending (newest first within that day). Because we
// iterate buckets from newest to oldest, the combined result is naturally newest-first.
func (s *DynamoDBAuditStore) listAllByDateBucket(ctx context.Context, filter *AuditFilter, resultLimit int) ([]*AuditEntry, error) {
	since := time.Now().Add(-24 * time.Hour)
	if filter != nil && filter.Since != nil {
		since = *filter.Since
	}

	endTime := time.Now().UTC()
	if filter != nil && filter.Before != nil {
		endTime = filter.Before.UTC()
	}

	sinceStr := since.Format(time.RFC3339Nano)
	endDay := endTime.Truncate(24 * time.Hour)
	startDay := since.UTC().Truncate(24 * time.Hour)

	var entries []*AuditEntry
	for day := endDay; !day.Before(startDay); day = day.AddDate(0, 0, -1) {
		bucket := day.Format("2006-01-02")

		var keyCond expression.KeyConditionBuilder
		if filter != nil && filter.Before != nil {
			beforeStr := filter.Before.Format(time.RFC3339Nano)
			keyCond = expression.KeyAnd(
				expression.Key("dateBucket").Equal(expression.Value(bucket)),
				expression.Key("timestamp").Between(
					expression.Value(sinceStr),
					expression.Value(beforeStr),
				),
			)
		} else {
			keyCond = expression.KeyAnd(
				expression.Key("dateBucket").Equal(expression.Value(bucket)),
				expression.Key("timestamp").GreaterThanEqual(expression.Value(sinceStr)),
			)
		}

		builder := expression.NewBuilder().WithKeyCondition(keyCond)
		builder = s.applyAuditFilterConditions(builder, filter)

		expr, err := builder.Build()
		if err != nil {
			return nil, fmt.Errorf("building date-bucket query expression: %w", err)
		}

		const maxPages = 10
		var lastKey map[string]types.AttributeValue
		for page := 0; page < maxPages; page++ {
			input := &dynamodb.QueryInput{
				TableName:                 &s.tableName,
				IndexName:                 aws.String("date-bucket-index"),
				KeyConditionExpression:    expr.KeyCondition(),
				ExpressionAttributeNames:  expr.Names(),
				ExpressionAttributeValues: expr.Values(),
				FilterExpression:          expr.Filter(),
				ScanIndexForward:          aws.Bool(false),
				ExclusiveStartKey:         lastKey,
			}

			out, err := s.client.Query(ctx, input)
			if err != nil {
				return nil, fmt.Errorf("querying audit date-bucket: %w", err)
			}

			for _, item := range out.Items {
				var entry AuditEntry
				if err := attributevalue.UnmarshalMap(item, &entry); err != nil {
					return nil, fmt.Errorf("unmarshaling audit entry: %w", err)
				}
				entries = append(entries, &entry)
			}

			if resultLimit > 0 && len(entries) >= resultLimit {
				entries = entries[:resultLimit]
				return entries, nil
			}

			if out.LastEvaluatedKey == nil {
				break
			}
			lastKey = out.LastEvaluatedKey
		}
	}

	return entries, nil
}

// applyAuditFilterConditions adds non-key filter conditions for audit queries.
func (s *DynamoDBAuditStore) applyAuditFilterConditions(builder expression.Builder, filter *AuditFilter) expression.Builder {
	if filter == nil {
		return builder
	}

	var conditions []expression.ConditionBuilder
	if filter.Target != nil {
		conditions = append(conditions, expression.Name("targetCluster").Equal(expression.Value(*filter.Target)))
	}
	if filter.Action != nil {
		conditions = append(conditions, expression.Name("action").Equal(expression.Value(*filter.Action)))
	}
	if filter.Method != nil {
		conditions = append(conditions, expression.Name("method").Equal(expression.Value(*filter.Method)))
	}
	if filter.Operator != nil {
		conditions = append(conditions, expression.Name("operator").Contains(*filter.Operator))
	}
	if filter.Force != nil {
		conditions = append(conditions, expression.Name("force").Equal(expression.Value(*filter.Force)))
	}
	if filter.DryRun != nil {
		conditions = append(conditions, expression.Name("dryRun").Equal(expression.Value(*filter.DryRun)))
	}
	if len(conditions) > 0 {
		combined := conditions[0]
		for _, c := range conditions[1:] {
			combined = combined.And(c)
		}
		builder = builder.WithFilter(combined)
	}
	return builder
}
