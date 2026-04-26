package dynamo

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cursorTestItem struct {
	PK    string
	SK    string
	Name  string
	Time  time.Time
	Count int
}

func TestCursorEncodeAndDecode(t *testing.T) {
	item := cursorTestItem{
		PK:    "ARTICLE#test-slug",
		SK:    "ARTICLE#test-slug",
		Name:  "Test Article",
		Time:  time.Now().UTC().Truncate(time.Second),
		Count: 42,
	}

	key, err := attributevalue.MarshalMap(item)
	require.NoError(t, err)

	cursor, err := lastEvalKeyToCursor(key)
	require.NoError(t, err)
	assert.NotEmpty(t, cursor)

	keyBack, err := cursorToLastEval(cursor)
	require.NoError(t, err)

	assert.Equal(t, key, keyBack)
}

func TestCursorToLastEvalInvalidBase64(t *testing.T) {
	_, err := cursorToLastEval("not-valid-base64!!!")
	require.Error(t, err)
}

func TestGetKeyFromItem(t *testing.T) {
	fullKey := map[string]types.AttributeValue{
		"PK":     &types.AttributeValueMemberS{Value: "ARTICLE#slug-1"},
		"SK":     &types.AttributeValueMemberS{Value: "ARTICLE#slug-1"},
		"GSI1PK": &types.AttributeValueMemberS{Value: "ARTICLE"},
		"GSI1SK": &types.AttributeValueMemberS{Value: "STATUS#0#published#UPDATED_AT#2024-01-01T00:00:00Z#slug-1"},
	}

	fullItem := map[string]types.AttributeValue{
		"PK":     &types.AttributeValueMemberS{Value: "ARTICLE#slug-1"},
		"SK":     &types.AttributeValueMemberS{Value: "ARTICLE#slug-1"},
		"GSI1PK": &types.AttributeValueMemberS{Value: "ARTICLE"},
		"GSI1SK": &types.AttributeValueMemberS{Value: "STATUS#0#published#UPDATED_AT#2024-01-01T00:00:00Z#slug-1"},
		"Title":  &types.AttributeValueMemberS{Value: "Test Title"},
		"Status": &types.AttributeValueMemberS{Value: "published"},
	}

	key := getKeyFromItem(fullKey, fullItem)

	assert.Len(t, key, len(fullKey))
	for k := range fullKey {
		assert.Equal(t, fullItem[k], key[k], "key %s should match", k)
	}
}
