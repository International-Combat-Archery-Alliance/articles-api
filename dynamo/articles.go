package dynamo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/International-Combat-Archery-Alliance/articles-api/articles"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var _ articles.Repository = &DB{}

const (
	articleEntityName = "ARTICLE"
)

type articleDynamo struct {
	PK          string
	SK          string
	GSI1PK      string
	GSI1SK      string
	Slug        string
	Version     int
	Title       string
	Excerpt     string
	Content     string
	Status      string
	Author      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PublishedAt *time.Time
}

func articlePK(slug string) string {
	return fmt.Sprintf("%s#%s", articleEntityName, slug)
}

func articleSK(slug string) string {
	return fmt.Sprintf("%s#%s", articleEntityName, slug)
}

func newArticleDynamo(article articles.Article) articleDynamo {
	contentBytes, err := json.Marshal(article.Content)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal content: %s", err))
	}

	var gsi1SK string
	if article.Status == articles.StatusPublished && article.PublishedAt != nil {
		gsi1SK = fmt.Sprintf("PUBLISHED_AT#%s#%s", article.PublishedAt.Format(time.RFC3339Nano), article.Slug)
	} else {
		gsi1SK = fmt.Sprintf("CREATED_AT#%s#%s", article.CreatedAt.Format(time.RFC3339Nano), article.Slug)
	}

	return articleDynamo{
		PK:     articlePK(article.Slug),
		SK:     articleSK(article.Slug),
		GSI1PK: fmt.Sprintf("STATUS#%s", article.Status),
		GSI1SK: gsi1SK,
		Slug:   article.Slug,

		Version:     article.Version,
		Title:       article.Title,
		Excerpt:     article.Excerpt,
		Content:     string(contentBytes),
		Status:      string(article.Status),
		Author:      article.Author,
		CreatedAt:   article.CreatedAt.UTC(),
		UpdatedAt:   article.UpdatedAt.UTC(),
		PublishedAt: article.PublishedAt,
	}
}

func articleFromArticleDynamo(item articleDynamo) articles.Article {
	var content any
	err := json.Unmarshal([]byte(item.Content), &content)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal content: %s", err))
	}

	return articles.Article{
		Slug:        item.Slug,
		Version:     item.Version,
		Title:       item.Title,
		Excerpt:     item.Excerpt,
		Content:     content,
		Status:      articles.Status(item.Status),
		Author:      item.Author,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
		PublishedAt: item.PublishedAt,
	}
}

func (d *DB) getArticleByKey(ctx context.Context, pk string, sk string) (articles.Article, error) {
	resp, err := d.dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return articles.Article{}, articles.NewTimeoutError("GetArticle timed out")
		}
		return articles.Article{}, articles.NewFailedToFetchError("Failed to fetch article", err)
	}

	if len(resp.Item) == 0 {
		return articles.Article{}, articles.NewArticleDoesNotExistError("Article not found", nil)
	}

	var item articleDynamo
	err = attributevalue.UnmarshalMap(resp.Item, &item)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal article from DB: %s", err))
	}
	return articleFromArticleDynamo(item), nil
}

func (d *DB) GetArticle(ctx context.Context, slug string) (articles.Article, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	return d.getArticleByKey(ctx, articlePK(slug), articleSK(slug))
}

func (d *DB) GetPublishedArticle(ctx context.Context, slug string) (articles.Article, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	article, err := d.getArticleByKey(ctx, articlePK(slug), articleSK(slug))
	if err != nil {
		return articles.Article{}, err
	}

	if article.Status != articles.StatusPublished {
		return articles.Article{}, articles.NewArticleDoesNotExistError("Article not found", nil)
	}

	return article, nil
}

func (d *DB) CreateArticle(ctx context.Context, article articles.Article) error {
	ctx, cancel := context.WithTimeoutCause(ctx, time.Second, articles.NewTimeoutError("CreateArticle to DB took too long"))
	defer cancel()

	dynamoItem := newArticleDynamo(article)

	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return articles.NewFailedToTranslateToDBModelError("Failed to convert Article to dynamo model", err)
	}

	expr := exprMustBuild(expression.NewBuilder().
		WithCondition(newEntityVersionConditional(dynamoItem.Version)))

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:                 aws.String(d.tableName),
		Item:                      item,
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		var condCheckFailedErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckFailedErr) {
			return articles.NewArticleAlreadyExistsError(fmt.Sprintf("Article with slug %q already exists", article.Slug), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return articles.NewTimeoutError("CreateArticle timed out")
		} else {
			return articles.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}

func (d *DB) UpdateArticle(ctx context.Context, article articles.Article) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	dynamoItem := newArticleDynamo(article)

	item, err := attributevalue.MarshalMap(dynamoItem)
	if err != nil {
		return articles.NewFailedToTranslateToDBModelError("Failed to convert Article to dynamo model", err)
	}

	expr := exprMustBuild(expression.NewBuilder().
		WithCondition(existingEntityVersionConditional(dynamoItem.Version)))

	_, err = d.dynamoClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:                 aws.String(d.tableName),
		Item:                      item,
		ConditionExpression:       expr.Condition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
	})
	if err != nil {
		var condCheckFailedErr *types.ConditionalCheckFailedException
		if errors.As(err, &condCheckFailedErr) {
			return articles.NewArticleDoesNotExistError(fmt.Sprintf("Article with slug %q does not exist", article.Slug), err)
		} else if errors.Is(err, context.DeadlineExceeded) {
			return articles.NewTimeoutError("UpdateArticle timed out")
		} else {
			return articles.NewFailedToWriteError("Failed PutItem call", err)
		}
	}

	return nil
}

func (d *DB) DeleteArticle(ctx context.Context, slug string) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	_, err := d.dynamoClient.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(d.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: articlePK(slug)},
			"SK": &types.AttributeValueMemberS{Value: articleSK(slug)},
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return articles.NewTimeoutError("DeleteArticle timed out")
		}
		return articles.NewFailedToWriteError("Failed to delete article", err)
	}

	return nil
}

func (d *DB) GetArticles(ctx context.Context, limit int32, cursor *string, articleStatus string) (articles.GetArticlesResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()

	statusPK := fmt.Sprintf("STATUS#%s", articleStatus)
	beginsWithPrefix := "PUBLISHED_AT#"
	if articleStatus == string(articles.StatusDraft) {
		beginsWithPrefix = "CREATED_AT#"
	}

	keyCond := expression.Key("GSI1PK").Equal(expression.Value(statusPK)).
		And(expression.Key("GSI1SK").BeginsWith(beginsWithPrefix))

	expr, err := expression.NewBuilder().WithKeyCondition(keyCond).Build()
	if err != nil {
		panic(fmt.Sprintf("failed to build dynamo key expression: %s", err))
	}

	var startKey map[string]types.AttributeValue
	if cursor != nil {
		startKey, err = cursorToLastEval(*cursor)
		if err != nil {
			return articles.GetArticlesResponse{}, articles.NewInvalidCursorError("Invalid cursor", err)
		}
	}

	result, err := d.dynamoClient.Query(ctx, &dynamodb.QueryInput{
		IndexName:                 aws.String(gsi1),
		TableName:                 aws.String(d.tableName),
		KeyConditionExpression:    expr.KeyCondition(),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		ScanIndexForward:          aws.Bool(false),
		Limit:                     aws.Int32(limit + 1),
		ExclusiveStartKey:         startKey,
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return articles.GetArticlesResponse{}, articles.NewTimeoutError("GetArticles timed out")
		}
		return articles.GetArticlesResponse{}, articles.NewFailedToFetchError("Failed to fetch articles from dynamo", err)
	}

	var dynamoItems []articleDynamo
	err = attributevalue.UnmarshalListOfMaps(result.Items, &dynamoItems)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal dynamo articles: %s", err))
	}

	hasNextPage := len(dynamoItems) > int(limit)

	var newCursor *string
	if hasNextPage && len(result.LastEvaluatedKey) > 0 {
		lastItemGivenToUser := result.Items[len(result.Items)-2]
		lastItemKey := getKeyFromItem(result.LastEvaluatedKey, lastItemGivenToUser)
		c, err := lastEvalKeyToCursor(lastItemKey)
		if err != nil {
			panic(fmt.Sprintf("failed to make cursor from lastEvalKey: %s", err))
		}
		newCursor = &c
	}

	convertedArticles := make([]articles.Article, len(dynamoItems))
	for i, v := range dynamoItems {
		convertedArticles[i] = articleFromArticleDynamo(v)
	}

	return articles.GetArticlesResponse{
		Data:        convertedArticles[:min(int(limit), len(convertedArticles))],
		Cursor:      newCursor,
		HasNextPage: hasNextPage,
	}, nil
}
