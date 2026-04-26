package api

import (
	"context"
	"testing"
	"time"

	"github.com/International-Combat-Archery-Alliance/articles-api/articles"
	"github.com/International-Combat-Archery-Alliance/articles-api/ptr"
	"github.com/International-Combat-Archery-Alliance/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var now = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func domainArticleFixture(slug string, status articles.Status) articles.Article {
	var publishedAt *time.Time
	if status == articles.StatusPublished {
		t := now.Add(time.Hour)
		publishedAt = &t
	}
	return articles.Article{
		Slug:        slug,
		Version:     1,
		Title:       "Test Article: " + slug,
		Excerpt:     "Excerpt for " + slug,
		Content:     map[string]any{"blocks": []any{map[string]any{"type": "paragraph", "data": map[string]any{"text": "Hello"}}}},
		Status:      status,
		Author:      "admin@example.com",
		CreatedAt:   now,
		UpdatedAt:   now,
		PublishedAt: publishedAt,
	}
}

func TestGetArticlesV1(t *testing.T) {
	t.Run("successfully get published articles", func(t *testing.T) {
		a1 := domainArticleFixture("article-1", articles.StatusPublished)
		a2 := domainArticleFixture("article-2", articles.StatusPublished)
		mock := &mockDB{
			GetArticlesFunc: func(ctx context.Context, limit int32, cursor *string, status string) (articles.GetArticlesResponse, error) {
				assert.Equal(t, string(articles.StatusPublished), status)
				return articles.GetArticlesResponse{
					Data:        []articles.Article{a1, a2},
					HasNextPage: false,
				}, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := GetArticlesV1RequestObject{Params: GetArticlesV1Params{}}
		resp, err := api.GetArticlesV1(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case GetArticlesV1200JSONResponse:
			assert.Len(t, r.Data, 2)
			assert.False(t, r.HasNextPage)
			assert.Nil(t, r.Cursor)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 400 for invalid cursor", func(t *testing.T) {
		mock := &mockDB{
			GetArticlesFunc: func(ctx context.Context, limit int32, cursor *string, status string) (articles.GetArticlesResponse, error) {
				return articles.GetArticlesResponse{}, articles.NewInvalidCursorError("bad cursor", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := GetArticlesV1RequestObject{Params: GetArticlesV1Params{}}
		resp, err := api.GetArticlesV1(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case GetArticlesV1400JSONResponse:
			assert.Equal(t, InvalidCursor, r.Code)
		default:
			t.Fatalf("expected 400 response, got: %T", resp)
		}
	})
}

func TestGetArticlesV1Slug(t *testing.T) {
	t.Run("successfully get article by slug", func(t *testing.T) {
		article := domainArticleFixture("my-slug", articles.StatusPublished)
		mock := &mockDB{
			GetPublishedArticleFunc: func(ctx context.Context, slug string) (articles.Article, error) {
				assert.Equal(t, "my-slug", slug)
				return article, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := GetArticlesV1SlugRequestObject{Slug: "my-slug"}
		resp, err := api.GetArticlesV1Slug(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case GetArticlesV1Slug200JSONResponse:
			assert.Equal(t, "my-slug", r.Article.Slug)
			assert.Equal(t, Published, r.Article.Status)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 404 when article not found", func(t *testing.T) {
		mock := &mockDB{
			GetPublishedArticleFunc: func(ctx context.Context, slug string) (articles.Article, error) {
				return articles.Article{}, articles.NewArticleDoesNotExistError("not found", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := GetArticlesV1SlugRequestObject{Slug: "nonexistent"}
		resp, err := api.GetArticlesV1Slug(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case GetArticlesV1Slug404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("expected 404 response, got: %T", resp)
		}
	})
}

func TestGetArticlesV1Admin(t *testing.T) {
	t.Run("successfully get published and draft articles", func(t *testing.T) {
		published := domainArticleFixture("pub", articles.StatusPublished)
		published.UpdatedAt = now.Add(time.Hour)
		draft := domainArticleFixture("drf", articles.StatusDraft)

		mock := &mockDB{
			GetArticlesFunc: func(ctx context.Context, limit int32, cursor *string, status string) (articles.GetArticlesResponse, error) {
				if status == string(articles.StatusPublished) {
					return articles.GetArticlesResponse{
						Data:        []articles.Article{published},
						HasNextPage: false,
					}, nil
				}
				return articles.GetArticlesResponse{
					Data:        []articles.Article{draft},
					HasNextPage: false,
				}, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := GetArticlesV1AdminRequestObject{Params: GetArticlesV1AdminParams{}}
		resp, err := api.GetArticlesV1Admin(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case GetArticlesV1Admin200JSONResponse:
			assert.Len(t, r.Data, 2)
			assert.False(t, r.HasNextPage)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 500 when both queries fail", func(t *testing.T) {
		mock := &mockDB{
			GetArticlesFunc: func(ctx context.Context, limit int32, cursor *string, status string) (articles.GetArticlesResponse, error) {
				return articles.GetArticlesResponse{}, articles.NewTimeoutError("timeout")
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := GetArticlesV1AdminRequestObject{Params: GetArticlesV1AdminParams{}}
		resp, err := api.GetArticlesV1Admin(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case GetArticlesV1Admin500JSONResponse:
			assert.Equal(t, InternalError, r.Code)
		default:
			t.Fatalf("expected 500 response, got: %T", resp)
		}
	})

	t.Run("respects limit parameter", func(t *testing.T) {
		articlesList := make([]articles.Article, 5)
		for i := range 5 {
			a := domainArticleFixture("article-"+string(rune('a'+i)), articles.StatusPublished)
			a.UpdatedAt = now.Add(time.Duration(5-i) * time.Hour)
			articlesList[i] = a
		}
		mock := &mockDB{
			GetArticlesFunc: func(ctx context.Context, limit int32, cursor *string, status string) (articles.GetArticlesResponse, error) {
				return articles.GetArticlesResponse{
					Data:        []articles.Article{articlesList[0], articlesList[1], articlesList[2]},
					HasNextPage: true,
				}, nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		limit := 3
		req := GetArticlesV1AdminRequestObject{Params: GetArticlesV1AdminParams{Limit: &limit}}
		resp, err := api.GetArticlesV1Admin(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case GetArticlesV1Admin200JSONResponse:
			assert.Len(t, r.Data, 3)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestPostArticlesV1(t *testing.T) {
	t.Run("successfully create an article", func(t *testing.T) {
		mock := &mockDB{
			CreateArticleFunc: func(ctx context.Context, article articles.Article) error {
				assert.Equal(t, "new-slug", article.Slug)
				assert.Equal(t, articles.StatusDraft, article.Status)
				assert.Equal(t, "admin@example.com", article.Author)
				assert.Equal(t, 1, article.Version)
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		ctx := middleware.CtxWithJWT(context.Background(), &mockAuthToken{
			email:   "admin@example.com",
			isAdmin: true,
		})
		ctx = ctxWithLogger(ctx, noopLogger)

		body := CreateArticleRequest{
			Slug:    "new-slug",
			Title:   "New Article",
			Excerpt: "New excerpt",
			Content: map[string]any{"blocks": []any{}},
		}
		req := PostArticlesV1RequestObject{Body: &body}
		resp, err := api.PostArticlesV1(ctx, req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case PostArticlesV1201JSONResponse:
			assert.Equal(t, "new-slug", r.Article.Slug)
			assert.Equal(t, Draft, r.Article.Status)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 409 when slug already exists", func(t *testing.T) {
		mock := &mockDB{
			CreateArticleFunc: func(ctx context.Context, article articles.Article) error {
				return articles.NewArticleAlreadyExistsError("slug exists", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		ctx := middleware.CtxWithJWT(context.Background(), &mockAuthToken{
			email:   "admin@example.com",
			isAdmin: true,
		})
		ctx = ctxWithLogger(ctx, noopLogger)

		body := CreateArticleRequest{
			Slug:    "existing-slug",
			Title:   "Article",
			Excerpt: "Excerpt",
			Content: map[string]any{},
		}
		req := PostArticlesV1RequestObject{Body: &body}
		resp, err := api.PostArticlesV1(ctx, req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case PostArticlesV1409JSONResponse:
			assert.Equal(t, AlreadyExists, r.Code)
		default:
			t.Fatalf("expected 409 response, got: %T", resp)
		}
	})

	t.Run("returns 401 when no JWT in context", func(t *testing.T) {
		mock := &mockDB{}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		body := CreateArticleRequest{
			Slug:    "any-slug",
			Title:   "Any",
			Excerpt: "Any",
			Content: map[string]any{},
		}
		req := PostArticlesV1RequestObject{Body: &body}
		resp, err := api.PostArticlesV1(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case PostArticlesV1401JSONResponse:
			assert.Equal(t, AuthError, r.Code)
		default:
			t.Fatalf("expected 401 response, got: %T", resp)
		}
	})
}

func TestPatchArticlesV1Slug(t *testing.T) {
	t.Run("successfully update an article", func(t *testing.T) {
		existing := domainArticleFixture("update-me", articles.StatusDraft)
		mock := &mockDB{
			GetArticleFunc: func(ctx context.Context, slug string) (articles.Article, error) {
				return existing, nil
			},
			UpdateArticleFunc: func(ctx context.Context, article articles.Article) error {
				assert.Equal(t, 2, article.Version)
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		newTitle := "Updated Title"
		newExcerpt := "Updated Excerpt"
		req := PatchArticlesV1SlugRequestObject{
			Slug: "update-me",
			Body: &UpdateArticleRequest{
				Title:   &newTitle,
				Excerpt: &newExcerpt,
			},
		}
		resp, err := api.PatchArticlesV1Slug(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case PatchArticlesV1Slug200JSONResponse:
			assert.Equal(t, "Updated Title", r.Article.Title)
			assert.Equal(t, "Updated Excerpt", r.Article.Excerpt)
			assert.Equal(t, ptr.Int(2), r.Article.Version)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 404 when article not found", func(t *testing.T) {
		mock := &mockDB{
			GetArticleFunc: func(ctx context.Context, slug string) (articles.Article, error) {
				return articles.Article{}, articles.NewArticleDoesNotExistError("not found", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := PatchArticlesV1SlugRequestObject{Slug: "nonexistent"}
		resp, err := api.PatchArticlesV1Slug(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case PatchArticlesV1Slug404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("expected 404 response, got: %T", resp)
		}
	})

	t.Run("updates only provided fields", func(t *testing.T) {
		existing := domainArticleFixture("partial", articles.StatusDraft)
		mock := &mockDB{
			GetArticleFunc: func(ctx context.Context, slug string) (articles.Article, error) {
				return existing, nil
			},
			UpdateArticleFunc: func(ctx context.Context, article articles.Article) error {
				assert.Equal(t, "Only Title Changed", article.Title)
				assert.Equal(t, existing.Excerpt, article.Excerpt)
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		newTitle := "Only Title Changed"
		req := PatchArticlesV1SlugRequestObject{
			Slug: "partial",
			Body: &UpdateArticleRequest{Title: &newTitle},
		}
		resp, err := api.PatchArticlesV1Slug(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case PatchArticlesV1Slug200JSONResponse:
			assert.Equal(t, "Only Title Changed", r.Article.Title)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})
}

func TestDeleteArticlesV1Slug(t *testing.T) {
	t.Run("successfully delete an article", func(t *testing.T) {
		mock := &mockDB{
			DeleteArticleFunc: func(ctx context.Context, slug string) error {
				assert.Equal(t, "delete-me", slug)
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := DeleteArticlesV1SlugRequestObject{Slug: "delete-me"}
		resp, err := api.DeleteArticlesV1Slug(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch resp.(type) {
		case DeleteArticlesV1Slug204Response:
		default:
			t.Fatalf("expected 204 response, got: %T", resp)
		}
	})

	t.Run("returns 404 when article not found", func(t *testing.T) {
		mock := &mockDB{
			DeleteArticleFunc: func(ctx context.Context, slug string) error {
				return articles.NewArticleDoesNotExistError("not found", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := DeleteArticlesV1SlugRequestObject{Slug: "nonexistent"}
		resp, err := api.DeleteArticlesV1Slug(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case DeleteArticlesV1Slug404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("expected 404 response, got: %T", resp)
		}
	})
}

func TestPostArticlesV1SlugPublish(t *testing.T) {
	t.Run("successfully publish an article", func(t *testing.T) {
		existing := domainArticleFixture("publish-me", articles.StatusDraft)
		mock := &mockDB{
			GetArticleFunc: func(ctx context.Context, slug string) (articles.Article, error) {
				return existing, nil
			},
			UpdateArticleFunc: func(ctx context.Context, article articles.Article) error {
				assert.Equal(t, articles.StatusPublished, article.Status)
				assert.NotNil(t, article.PublishedAt)
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := PostArticlesV1SlugPublishRequestObject{Slug: "publish-me"}
		resp, err := api.PostArticlesV1SlugPublish(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case PostArticlesV1SlugPublish200JSONResponse:
			assert.Equal(t, Published, r.Article.Status)
			assert.NotNil(t, r.Article.PublishedAt)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 404 when article not found", func(t *testing.T) {
		mock := &mockDB{
			GetArticleFunc: func(ctx context.Context, slug string) (articles.Article, error) {
				return articles.Article{}, articles.NewArticleDoesNotExistError("not found", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := PostArticlesV1SlugPublishRequestObject{Slug: "nonexistent"}
		resp, err := api.PostArticlesV1SlugPublish(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case PostArticlesV1SlugPublish404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("expected 404 response, got: %T", resp)
		}
	})
}

func TestPostArticlesV1SlugUnpublish(t *testing.T) {
	t.Run("successfully unpublish an article", func(t *testing.T) {
		existing := domainArticleFixture("unpublish-me", articles.StatusPublished)
		mock := &mockDB{
			GetArticleFunc: func(ctx context.Context, slug string) (articles.Article, error) {
				return existing, nil
			},
			UpdateArticleFunc: func(ctx context.Context, article articles.Article) error {
				assert.Equal(t, articles.StatusDraft, article.Status)
				assert.Nil(t, article.PublishedAt)
				return nil
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := PostArticlesV1SlugUnpublishRequestObject{Slug: "unpublish-me"}
		resp, err := api.PostArticlesV1SlugUnpublish(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case PostArticlesV1SlugUnpublish200JSONResponse:
			assert.Equal(t, Draft, r.Article.Status)
			assert.Nil(t, r.Article.PublishedAt)
		default:
			t.Fatalf("unexpected response type: %T", resp)
		}
	})

	t.Run("returns 404 when article not found", func(t *testing.T) {
		mock := &mockDB{
			GetArticleFunc: func(ctx context.Context, slug string) (articles.Article, error) {
				return articles.Article{}, articles.NewArticleDoesNotExistError("not found", nil)
			},
		}
		api := NewAPI(mock, noopLogger, LOCAL, newTestTokenService(), func(ctx context.Context) error { return nil })

		req := PostArticlesV1SlugUnpublishRequestObject{Slug: "nonexistent"}
		resp, err := api.PostArticlesV1SlugUnpublish(ctxWithLogger(context.Background(), noopLogger), req)
		require.NoError(t, err)

		switch r := resp.(type) {
		case PostArticlesV1SlugUnpublish404JSONResponse:
			assert.Equal(t, NotFound, r.Code)
		default:
			t.Fatalf("expected 404 response, got: %T", resp)
		}
	})
}
