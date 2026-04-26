package api

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/International-Combat-Archery-Alliance/articles-api/articles"
	"github.com/International-Combat-Archery-Alliance/articles-api/ptr"
	"github.com/International-Combat-Archery-Alliance/middleware"
	"go.opentelemetry.io/otel/codes"
)

func (a *API) GetArticlesV1(ctx context.Context, request GetArticlesV1RequestObject) (GetArticlesV1ResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "GetArticlesV1")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	limit := int32(10)
	if request.Params.Limit != nil {
		limit = int32(*request.Params.Limit)
	}

	result, err := a.db.GetArticles(ctx, limit, request.Params.Cursor, string(articles.StatusPublished))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to get published articles from DB", "error", err)

		var articleErr *articles.Error
		if errors.As(err, &articleErr) {
			switch articleErr.Reason {
			case articles.REASON_INVALID_CURSOR:
				return GetArticlesV1400JSONResponse{
					Code:    InvalidCursor,
					Message: "Passed in cursor is invalid",
				}, nil
			}
		}
		return GetArticlesV1500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get articles",
		}, nil
	}

	respArticles := make([]Article, len(result.Data))
	for i, v := range result.Data {
		respArticles[i] = articleToAPIArticle(v)
	}

	return GetArticlesV1200JSONResponse{
		Data:        respArticles,
		Cursor:      result.Cursor,
		HasNextPage: result.HasNextPage,
	}, nil
}

func (a *API) GetArticlesV1Slug(ctx context.Context, request GetArticlesV1SlugRequestObject) (GetArticlesV1SlugResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "GetArticlesV1Slug")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	article, err := a.db.GetPublishedArticle(ctx, request.Slug)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to get article by slug", "slug", request.Slug, "error", err)

		var articleErr *articles.Error
		if errors.As(err, &articleErr) {
			if articleErr.Reason == articles.REASON_ARTICLE_DOES_NOT_EXIST {
				return GetArticlesV1Slug404JSONResponse{
					Code:    NotFound,
					Message: "Article not found",
				}, nil
			}
		}
		return GetArticlesV1Slug500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get article",
		}, nil
	}

	return GetArticlesV1Slug200JSONResponse{
		Article: articleToAPIArticle(article),
	}, nil
}

func (a *API) GetArticlesV1Admin(ctx context.Context, request GetArticlesV1AdminRequestObject) (GetArticlesV1AdminResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "GetArticlesV1Admin")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	limit := int32(10)
	if request.Params.Limit != nil {
		limit = int32(*request.Params.Limit)
	}

	publishedResult, pubErr := a.db.GetArticles(ctx, limit, request.Params.Cursor, string(articles.StatusPublished))
	draftResult, draftErr := a.db.GetArticles(ctx, limit, request.Params.Cursor, string(articles.StatusDraft))

	if pubErr != nil && draftErr != nil {
		span.RecordError(pubErr)
		span.SetStatus(codes.Error, pubErr.Error())
		logger.Error("Failed to get all articles from DB", "error", pubErr)
		return GetArticlesV1Admin500JSONResponse{
			Code:    InternalError,
			Message: "Failed to get articles",
		}, nil
	}

	allArticles := make([]Article, 0, len(publishedResult.Data)+len(draftResult.Data))
	for _, v := range publishedResult.Data {
		allArticles = append(allArticles, articleToAPIArticle(v))
	}
	for _, v := range draftResult.Data {
		allArticles = append(allArticles, articleToAPIArticle(v))
	}

	sort.Slice(allArticles, func(i, j int) bool {
		return allArticles[i].UpdatedAt.After(*allArticles[j].UpdatedAt)
	})

	if int32(len(allArticles)) > limit {
		allArticles = allArticles[:limit]
	}

	hasNextPage := publishedResult.HasNextPage || draftResult.HasNextPage

	return GetArticlesV1Admin200JSONResponse{
		Data:        allArticles,
		HasNextPage: hasNextPage,
	}, nil
}

func (a *API) PostArticlesV1(ctx context.Context, request PostArticlesV1RequestObject) (PostArticlesV1ResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostArticlesV1")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	jwtToken, ok := middleware.GetJWTFromCtx(ctx)
	if !ok {
		return PostArticlesV1401JSONResponse{
			Code:    AuthError,
			Message: "Authentication required",
		}, nil
	}

	authorEmail := jwtToken.UserEmail()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	now := time.Now().UTC()
	article := articles.Article{
		Slug:      request.Body.Slug,
		Version:   1,
		Title:     request.Body.Title,
		Excerpt:   request.Body.Excerpt,
		Content:   request.Body.Content,
		Status:    articles.StatusDraft,
		Author:    authorEmail,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := a.db.CreateArticle(ctx, article)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to create article", "error", err)

		var articleErr *articles.Error
		if errors.As(err, &articleErr) {
			if articleErr.Reason == articles.REASON_ARTICLE_ALREADY_EXISTS {
				return PostArticlesV1409JSONResponse{
					Code:    AlreadyExists,
					Message: "Article with this slug already exists",
				}, nil
			}
		}
		return PostArticlesV1500JSONResponse{
			Code:    InternalError,
			Message: "Failed to create article",
		}, nil
	}

	logger.Info("created new article", "slug", article.Slug)

	return PostArticlesV1201JSONResponse{
		Article: articleToAPIArticle(article),
	}, nil
}

func (a *API) PatchArticlesV1Slug(ctx context.Context, request PatchArticlesV1SlugRequestObject) (PatchArticlesV1SlugResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PatchArticlesV1Slug")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	existing, err := a.db.GetArticle(ctx, request.Slug)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to get existing article", "slug", request.Slug, "error", err)

		var articleErr *articles.Error
		if errors.As(err, &articleErr) {
			if articleErr.Reason == articles.REASON_ARTICLE_DOES_NOT_EXIST {
				return PatchArticlesV1Slug404JSONResponse{
					Code:    NotFound,
					Message: "Article not found",
				}, nil
			}
		}
		return PatchArticlesV1Slug500JSONResponse{
			Code:    InternalError,
			Message: "Failed to update article",
		}, nil
	}

	updated := existing
	updated.Version = existing.Version + 1
	updated.UpdatedAt = time.Now().UTC()

	if request.Body.Title != nil {
		updated.Title = *request.Body.Title
	}
	if request.Body.Excerpt != nil {
		updated.Excerpt = *request.Body.Excerpt
	}
	if request.Body.Content != nil {
		updated.Content = *request.Body.Content
	}

	err = a.db.UpdateArticle(ctx, updated)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to update article", "slug", request.Slug, "error", err)

		var articleErr *articles.Error
		if errors.As(err, &articleErr) {
			if articleErr.Reason == articles.REASON_ARTICLE_DOES_NOT_EXIST {
				return PatchArticlesV1Slug404JSONResponse{
					Code:    NotFound,
					Message: "Article not found",
				}, nil
			}
		}
		return PatchArticlesV1Slug500JSONResponse{
			Code:    InternalError,
			Message: "Failed to update article",
		}, nil
	}

	return PatchArticlesV1Slug200JSONResponse{
		Article: articleToAPIArticle(updated),
	}, nil
}

func (a *API) DeleteArticlesV1Slug(ctx context.Context, request DeleteArticlesV1SlugRequestObject) (DeleteArticlesV1SlugResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "DeleteArticlesV1Slug")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := a.db.DeleteArticle(ctx, request.Slug)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to delete article", "slug", request.Slug, "error", err)

		var articleErr *articles.Error
		if errors.As(err, &articleErr) {
			if articleErr.Reason == articles.REASON_ARTICLE_DOES_NOT_EXIST {
				return DeleteArticlesV1Slug404JSONResponse{
					Code:    NotFound,
					Message: "Article not found",
				}, nil
			}
		}
		return DeleteArticlesV1Slug500JSONResponse{
			Code:    InternalError,
			Message: "Failed to delete article",
		}, nil
	}

	return DeleteArticlesV1Slug204Response{}, nil
}

func (a *API) PostArticlesV1SlugPublish(ctx context.Context, request PostArticlesV1SlugPublishRequestObject) (PostArticlesV1SlugPublishResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostArticlesV1SlugPublish")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	existing, err := a.db.GetArticle(ctx, request.Slug)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to get article for publish", "slug", request.Slug, "error", err)

		var articleErr *articles.Error
		if errors.As(err, &articleErr) {
			if articleErr.Reason == articles.REASON_ARTICLE_DOES_NOT_EXIST {
				return PostArticlesV1SlugPublish404JSONResponse{
					Code:    NotFound,
					Message: "Article not found",
				}, nil
			}
		}
		return PostArticlesV1SlugPublish500JSONResponse{
			Code:    InternalError,
			Message: "Failed to publish article",
		}, nil
	}

	now := time.Now().UTC()
	existing.Status = articles.StatusPublished
	existing.PublishedAt = ptr.Time(now)
	existing.UpdatedAt = now
	existing.Version = existing.Version + 1

	err = a.db.UpdateArticle(ctx, existing)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to publish article", "slug", request.Slug, "error", err)

		return PostArticlesV1SlugPublish500JSONResponse{
			Code:    InternalError,
			Message: "Failed to publish article",
		}, nil
	}

	logger.Info("published article", "slug", existing.Slug)

	return PostArticlesV1SlugPublish200JSONResponse{
		Article: articleToAPIArticle(existing),
	}, nil
}

func (a *API) PostArticlesV1SlugUnpublish(ctx context.Context, request PostArticlesV1SlugUnpublishRequestObject) (PostArticlesV1SlugUnpublishResponseObject, error) {
	ctx, span := a.tracer.Start(ctx, "PostArticlesV1SlugUnpublish")
	defer span.End()

	logger := a.getLoggerOrBaseLogger(ctx)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	existing, err := a.db.GetArticle(ctx, request.Slug)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to get article for unpublish", "slug", request.Slug, "error", err)

		var articleErr *articles.Error
		if errors.As(err, &articleErr) {
			if articleErr.Reason == articles.REASON_ARTICLE_DOES_NOT_EXIST {
				return PostArticlesV1SlugUnpublish404JSONResponse{
					Code:    NotFound,
					Message: "Article not found",
				}, nil
			}
		}
		return PostArticlesV1SlugUnpublish500JSONResponse{
			Code:    InternalError,
			Message: "Failed to unpublish article",
		}, nil
	}

	existing.Status = articles.StatusDraft
	existing.PublishedAt = nil
	existing.UpdatedAt = time.Now().UTC()
	existing.Version = existing.Version + 1

	err = a.db.UpdateArticle(ctx, existing)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		logger.Error("Failed to unpublish article", "slug", request.Slug, "error", err)

		return PostArticlesV1SlugUnpublish500JSONResponse{
			Code:    InternalError,
			Message: "Failed to unpublish article",
		}, nil
	}

	logger.Info("unpublished article", "slug", existing.Slug)

	return PostArticlesV1SlugUnpublish200JSONResponse{
		Article: articleToAPIArticle(existing),
	}, nil
}

func articleToAPIArticle(a articles.Article) Article {
	return Article{
		Slug:        a.Slug,
		Version:     &a.Version,
		Title:       a.Title,
		Excerpt:     a.Excerpt,
		Content:     a.Content.(map[string]interface{}),
		Status:      ArticleStatus(a.Status),
		Author:      ptr.String(a.Author),
		CreatedAt:   ptr.Time(a.CreatedAt),
		UpdatedAt:   ptr.Time(a.UpdatedAt),
		PublishedAt: a.PublishedAt,
	}
}
