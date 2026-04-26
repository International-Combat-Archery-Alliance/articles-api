package articles

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("github.com/International-Combat-Archery-Alliance/articles-api/articles")

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
)

type Article struct {
	Slug        string    `json:"slug"`
	Version     int       `json:"version"`
	Title       string    `json:"title"`
	Excerpt     string    `json:"excerpt"`
	Content     any       `json:"content"`
	Status      Status    `json:"status"`
	Author      string    `json:"author"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
	PublishedAt *time.Time `json:"publishedAt,omitempty"`
}

type GetArticlesResponse struct {
	Data        []Article
	Cursor      *string
	HasNextPage bool
}

type Repository interface {
	GetArticle(ctx context.Context, slug string) (Article, error)
	GetPublishedArticle(ctx context.Context, slug string) (Article, error)
	GetArticles(ctx context.Context, limit int32, cursor *string, status string) (GetArticlesResponse, error)
	CreateArticle(ctx context.Context, article Article) error
	UpdateArticle(ctx context.Context, article Article) error
	DeleteArticle(ctx context.Context, slug string) error
}
