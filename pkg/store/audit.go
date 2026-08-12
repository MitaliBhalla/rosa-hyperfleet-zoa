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
