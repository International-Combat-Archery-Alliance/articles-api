package dynamo

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/articles-api/articles"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func articleFixture(slug string, status articles.Status) articles.Article {
	var publishedAt *time.Time
	if status == articles.StatusPublished {
		t := now.Add(time.Hour)
		publishedAt = &t
	}
	return articles.Article{
		Slug:        slug,
		Version:     1,
		Title:       "Test Article: " + slug,
		Excerpt:     "An excerpt for " + slug,
		Content:     map[string]any{"blocks": []any{map[string]any{"type": "paragraph", "data": map[string]any{"text": "Hello"}}}},
		Status:      status,
		Author:      "test@example.com",
		CreatedAt:   now,
		UpdatedAt:   now,
		PublishedAt: publishedAt,
	}
}

func TestGetArticle(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully get an article", func(t *testing.T) {
		resetTable(ctx)
		article := articleFixture("test-get", articles.StatusDraft)
		require.NoError(t, testDB.CreateArticle(ctx, article))

		result, err := testDB.GetArticle(ctx, "test-get")
		require.NoError(t, err)
		assert.Equal(t, "test-get", result.Slug)
		assert.Equal(t, articles.StatusDraft, result.Status)
		assert.Equal(t, 1, result.Version)
		assert.Equal(t, "test@example.com", result.Author)
	})

	t.Run("article does not exist", func(t *testing.T) {
		resetTable(ctx)

		_, err := testDB.GetArticle(ctx, "nonexistent")
		require.Error(t, err)
		var articleErr *articles.Error
		require.ErrorAs(t, err, &articleErr)
		assert.Equal(t, articles.REASON_ARTICLE_DOES_NOT_EXIST, articleErr.Reason)
	})
}

func TestGetPublishedArticle(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully get a published article", func(t *testing.T) {
		resetTable(ctx)
		article := articleFixture("published-slug", articles.StatusPublished)
		require.NoError(t, testDB.CreateArticle(ctx, article))

		result, err := testDB.GetPublishedArticle(ctx, "published-slug")
		require.NoError(t, err)
		assert.Equal(t, "published-slug", result.Slug)
		assert.Equal(t, articles.StatusPublished, result.Status)
	})

	t.Run("draft article returns not found", func(t *testing.T) {
		resetTable(ctx)
		article := articleFixture("draft-slug", articles.StatusDraft)
		require.NoError(t, testDB.CreateArticle(ctx, article))

		_, err := testDB.GetPublishedArticle(ctx, "draft-slug")
		require.Error(t, err)
		var articleErr *articles.Error
		require.ErrorAs(t, err, &articleErr)
		assert.Equal(t, articles.REASON_ARTICLE_DOES_NOT_EXIST, articleErr.Reason)
	})

	t.Run("non-existent article returns not found", func(t *testing.T) {
		resetTable(ctx)

		_, err := testDB.GetPublishedArticle(ctx, "nonexistent")
		require.Error(t, err)
		var articleErr *articles.Error
		require.ErrorAs(t, err, &articleErr)
		assert.Equal(t, articles.REASON_ARTICLE_DOES_NOT_EXIST, articleErr.Reason)
	})
}

func TestCreateArticle(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully create an article", func(t *testing.T) {
		resetTable(ctx)
		article := articleFixture("new-article", articles.StatusDraft)

		require.NoError(t, testDB.CreateArticle(ctx, article))

		key, err := attributevalue.MarshalMap(map[string]any{
			"PK": "ARTICLE#new-article",
			"SK": "ARTICLE#new-article",
		})
		require.NoError(t, err)
		out, err := dynamoClient.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(testTableName),
			Key:       key,
		})
		require.NoError(t, err)
		require.NotEmpty(t, out.Item)

		var stored articleDynamo
		require.NoError(t, attributevalue.UnmarshalMap(out.Item, &stored))
		assert.Equal(t, "ARTICLE#new-article", stored.PK)
		assert.Equal(t, "new-article", stored.Slug)
		assert.Equal(t, 1, stored.Version)
		assert.Equal(t, "ARTICLE", stored.GSI1PK)
		assert.Contains(t, stored.GSI1SK, "STATUS#1#draft#UPDATED_AT#")
	})

	t.Run("article already exists", func(t *testing.T) {
		resetTable(ctx)
		article := articleFixture("duplicate", articles.StatusDraft)
		require.NoError(t, testDB.CreateArticle(ctx, article))

		err := testDB.CreateArticle(ctx, article)
		require.Error(t, err)
		var articleErr *articles.Error
		require.ErrorAs(t, err, &articleErr)
		assert.Equal(t, articles.REASON_ARTICLE_ALREADY_EXISTS, articleErr.Reason)
	})

	t.Run("create published article with correct GSI prefix", func(t *testing.T) {
		resetTable(ctx)
		article := articleFixture("published-gsi", articles.StatusPublished)
		require.NoError(t, testDB.CreateArticle(ctx, article))

		result, err := testDB.GetArticle(ctx, "published-gsi")
		require.NoError(t, err)
		assert.Equal(t, articles.StatusPublished, result.Status)
		assert.NotNil(t, result.PublishedAt)
	})
}

func TestUpdateArticle(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully update an article", func(t *testing.T) {
		resetTable(ctx)
		article := articleFixture("to-update", articles.StatusDraft)
		require.NoError(t, testDB.CreateArticle(ctx, article))

		article.Title = "Updated Title"
		article.Excerpt = "Updated Excerpt"
		article.Version = 2
		article.UpdatedAt = now.Add(time.Hour)

		require.NoError(t, testDB.UpdateArticle(ctx, article))

		result, err := testDB.GetArticle(ctx, "to-update")
		require.NoError(t, err)
		assert.Equal(t, "Updated Title", result.Title)
		assert.Equal(t, "Updated Excerpt", result.Excerpt)
		assert.Equal(t, 2, result.Version)
	})

	t.Run("update article that does not exist", func(t *testing.T) {
		resetTable(ctx)
		article := articleFixture("new-versioned", articles.StatusDraft)
		article.Version = 2

		err := testDB.UpdateArticle(ctx, article)
		require.Error(t, err)
		var articleErr *articles.Error
		require.ErrorAs(t, err, &articleErr)
		assert.Equal(t, articles.REASON_ARTICLE_DOES_NOT_EXIST, articleErr.Reason)
	})

	t.Run("optimistic locking - wrong version", func(t *testing.T) {
		resetTable(ctx)
		article := articleFixture("versioned", articles.StatusDraft)
		require.NoError(t, testDB.CreateArticle(ctx, article))

		article.Title = "Concurrent Update"
		article.Version = 1

		err := testDB.UpdateArticle(ctx, article)
		require.Error(t, err)
		var articleErr *articles.Error
		require.ErrorAs(t, err, &articleErr)
		assert.Equal(t, articles.REASON_ARTICLE_DOES_NOT_EXIST, articleErr.Reason)
	})
}

func TestDeleteArticle(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully delete an article", func(t *testing.T) {
		resetTable(ctx)
		article := articleFixture("to-delete", articles.StatusDraft)
		require.NoError(t, testDB.CreateArticle(ctx, article))

		require.NoError(t, testDB.DeleteArticle(ctx, "to-delete"))

		_, err := testDB.GetArticle(ctx, "to-delete")
		require.Error(t, err)
		var articleErr *articles.Error
		require.ErrorAs(t, err, &articleErr)
		assert.Equal(t, articles.REASON_ARTICLE_DOES_NOT_EXIST, articleErr.Reason)
	})
}

func TestGetArticles(t *testing.T) {
	ctx := context.Background()

	t.Run("list published articles sorted by updatedAt desc", func(t *testing.T) {
		resetTable(ctx)

		a1 := articleFixture("article-1", articles.StatusPublished)
		a1.UpdatedAt = now.Add(1 * time.Hour)
		require.NoError(t, testDB.CreateArticle(ctx, a1))

		a2 := articleFixture("article-2", articles.StatusPublished)
		a2.UpdatedAt = now.Add(2 * time.Hour)
		require.NoError(t, testDB.CreateArticle(ctx, a2))

		result, err := testDB.GetArticles(ctx, 10, nil, string(articles.StatusPublished))
		require.NoError(t, err)
		require.Len(t, result.Data, 2)
		assert.Equal(t, "article-2", result.Data[0].Slug)
		assert.Equal(t, "article-1", result.Data[1].Slug)
		assert.False(t, result.HasNextPage)
		assert.Nil(t, result.Cursor)
	})

	t.Run("list draft articles", func(t *testing.T) {
		resetTable(ctx)

		a1 := articleFixture("draft-1", articles.StatusDraft)
		a1.UpdatedAt = now.Add(1 * time.Hour)
		require.NoError(t, testDB.CreateArticle(ctx, a1))

		a2 := articleFixture("draft-2", articles.StatusDraft)
		a2.UpdatedAt = now.Add(2 * time.Hour)
		require.NoError(t, testDB.CreateArticle(ctx, a2))

		result, err := testDB.GetArticles(ctx, 10, nil, string(articles.StatusDraft))
		require.NoError(t, err)
		require.Len(t, result.Data, 2)
		assert.Equal(t, articles.StatusDraft, result.Data[0].Status)
		assert.Equal(t, articles.StatusDraft, result.Data[1].Status)
	})

	t.Run("pagination with limit", func(t *testing.T) {
		resetTable(ctx)

		for i := range 5 {
			a := articleFixture("page-test-"+string(rune('a'+i)), articles.StatusPublished)
			a.UpdatedAt = now.Add(time.Duration(i+1) * time.Hour)
			require.NoError(t, testDB.CreateArticle(ctx, a))
		}

		result, err := testDB.GetArticles(ctx, 3, nil, string(articles.StatusPublished))
		require.NoError(t, err)
		require.Len(t, result.Data, 3)
		assert.True(t, result.HasNextPage)
		assert.NotNil(t, result.Cursor)

		result2, err := testDB.GetArticles(ctx, 3, result.Cursor, string(articles.StatusPublished))
		require.NoError(t, err)
		require.Len(t, result2.Data, 2)
		assert.False(t, result2.HasNextPage)
		assert.Nil(t, result2.Cursor)
	})

	t.Run("admin listing returns all articles with drafts first then published, sorted by updatedAt desc within each", func(t *testing.T) {
		resetTable(ctx)

		d1 := articleFixture("draft-1", articles.StatusDraft)
		d1.UpdatedAt = now.Add(1 * time.Hour)
		require.NoError(t, testDB.CreateArticle(ctx, d1))

		p1 := articleFixture("pub-1", articles.StatusPublished)
		p1.UpdatedAt = now.Add(3 * time.Hour)
		require.NoError(t, testDB.CreateArticle(ctx, p1))

		p2 := articleFixture("pub-2", articles.StatusPublished)
		p2.UpdatedAt = now.Add(2 * time.Hour)
		require.NoError(t, testDB.CreateArticle(ctx, p2))

		result, err := testDB.GetArticles(ctx, 10, nil, "")
		require.NoError(t, err)
		require.Len(t, result.Data, 3)
		assert.Equal(t, "draft-1", result.Data[0].Slug)
		assert.Equal(t, "pub-1", result.Data[1].Slug)
		assert.Equal(t, "pub-2", result.Data[2].Slug)
		assert.False(t, result.HasNextPage)
		assert.Nil(t, result.Cursor)
	})

	t.Run("admin listing with cursor", func(t *testing.T) {
		resetTable(ctx)

		for i := range 5 {
			a := articleFixture("admin-test-"+string(rune('a'+i)), articles.StatusDraft)
			a.UpdatedAt = now.Add(time.Duration(i+1) * time.Hour)
			require.NoError(t, testDB.CreateArticle(ctx, a))
		}

		result, err := testDB.GetArticles(ctx, 3, nil, "")
		require.NoError(t, err)
		require.Len(t, result.Data, 3)
		assert.True(t, result.HasNextPage)
		assert.NotNil(t, result.Cursor)

		result2, err := testDB.GetArticles(ctx, 3, result.Cursor, "")
		require.NoError(t, err)
		require.Len(t, result2.Data, 2)
		assert.False(t, result2.HasNextPage)
		assert.Nil(t, result2.Cursor)
	})

	t.Run("invalid cursor returns error", func(t *testing.T) {
		resetTable(ctx)
		badCursor := "not-valid-base64!!!"
		_, err := testDB.GetArticles(ctx, 10, &badCursor, string(articles.StatusPublished))
		require.Error(t, err)
		var articleErr *articles.Error
		require.ErrorAs(t, err, &articleErr)
		assert.Equal(t, articles.REASON_INVALID_CURSOR, articleErr.Reason)
	})

	t.Run("no articles found returns empty list", func(t *testing.T) {
		resetTable(ctx)

		result, err := testDB.GetArticles(ctx, 10, nil, string(articles.StatusPublished))
		require.NoError(t, err)
		assert.Empty(t, result.Data)
		assert.False(t, result.HasNextPage)
		assert.Nil(t, result.Cursor)
	})
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
