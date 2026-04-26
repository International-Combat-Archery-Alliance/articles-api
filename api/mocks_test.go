package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/International-Combat-Archery-Alliance/articles-api/articles"
	"github.com/International-Combat-Archery-Alliance/auth"
	"github.com/International-Combat-Archery-Alliance/auth/token"
	"github.com/International-Combat-Archery-Alliance/middleware"
)

var noopLogger = slog.New(slog.DiscardHandler)

func newTestTokenService() *token.TokenService {
	testKey := token.SigningKey{
		ID:  "test",
		Key: []byte("test-signing-key-minimum-32-characters-long"),
	}
	return token.NewTokenService(testKey)
}

func generateTestToken(email string, isAdmin bool) string {
	ts := newTestTokenService()
	var roles []auth.Role
	if isAdmin {
		roles = []auth.Role{auth.RoleAdmin}
	}
	tokenStr, _ := ts.GenerateAccessToken(email, "", roles)
	return tokenStr
}

func ctxWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return middleware.CtxWithLogger(ctx, logger)
}

type mockAuthToken struct {
	email   string
	isAdmin bool
}

func (m *mockAuthToken) ExpiresAt() time.Time {
	return time.Now().Add(time.Hour)
}

func (m *mockAuthToken) ProfilePicURL() string {
	return "https://example.com/profile.jpg"
}

func (m *mockAuthToken) IsAdmin() bool {
	return m.isAdmin
}

func (m *mockAuthToken) UserEmail() string {
	return m.email
}

func (m *mockAuthToken) Roles() []auth.Role {
	if m.isAdmin {
		return []auth.Role{auth.RoleAdmin}
	}
	return nil
}

var _ DB = &mockDB{}

type mockDB struct {
	GetArticleFunc          func(ctx context.Context, slug string) (articles.Article, error)
	GetPublishedArticleFunc func(ctx context.Context, slug string) (articles.Article, error)
	GetArticlesFunc         func(ctx context.Context, limit int32, cursor *string, status string) (articles.GetArticlesResponse, error)
	CreateArticleFunc       func(ctx context.Context, article articles.Article) error
	UpdateArticleFunc       func(ctx context.Context, article articles.Article) error
	DeleteArticleFunc       func(ctx context.Context, slug string) error
}

func (m *mockDB) GetArticle(ctx context.Context, slug string) (articles.Article, error) {
	return m.GetArticleFunc(ctx, slug)
}

func (m *mockDB) GetPublishedArticle(ctx context.Context, slug string) (articles.Article, error) {
	return m.GetPublishedArticleFunc(ctx, slug)
}

func (m *mockDB) GetArticles(ctx context.Context, limit int32, cursor *string, status string) (articles.GetArticlesResponse, error) {
	return m.GetArticlesFunc(ctx, limit, cursor, status)
}

func (m *mockDB) CreateArticle(ctx context.Context, article articles.Article) error {
	return m.CreateArticleFunc(ctx, article)
}

func (m *mockDB) UpdateArticle(ctx context.Context, article articles.Article) error {
	return m.UpdateArticleFunc(ctx, article)
}

func (m *mockDB) DeleteArticle(ctx context.Context, slug string) error {
	return m.DeleteArticleFunc(ctx, slug)
}
