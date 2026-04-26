//go:generate go tool oapi-codegen --config openapi-codegen-config.yaml ../spec/api.yaml
package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/International-Combat-Archery-Alliance/articles-api/articles"
	"github.com/International-Combat-Archery-Alliance/auth/token"
	"github.com/International-Combat-Archery-Alliance/middleware"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

type Environment int

const (
	LOCAL Environment = iota
	PROD
)

type DB interface {
	articles.Repository
}

type API struct {
	db     DB
	logger *slog.Logger
	env    Environment
	tracer trace.Tracer

	tokenService *token.TokenService
	flushTraces  func(context.Context) error
}

var _ StrictServerInterface = (*API)(nil)

func NewAPI(
	db DB,
	logger *slog.Logger,
	env Environment,
	tokenService *token.TokenService,
	flushTraces func(context.Context) error,
) *API {
	return &API{
		db:           db,
		logger:       logger,
		env:          env,
		tracer:       otel.Tracer("github.com/International-Combat-Archery-Alliance/articles-api/api"),
		tokenService: tokenService,
		flushTraces:  flushTraces,
	}
}

func (a *API) ListenAndServe(host string, port string) error {
	swagger, err := GetSwagger()
	if err != nil {
		return fmt.Errorf("Error loading swagger spec: %w", err)
	}

	swagger.Servers = nil

	strictHandler := NewStrictHandler(a, []StrictMiddlewareFunc{})

	r := http.NewServeMux()

	HandlerFromMux(strictHandler, r)

	swaggerUIMiddleware, err := middleware.HostSwaggerUI("/articles", swagger)
	if err != nil {
		return fmt.Errorf("failed to create swagger ui middleware: %w", err)
	}

	corsConfig := middleware.DefaultCorsConfig()
	corsConfig.IsProduction = a.env == PROD
	corsMiddleware := middleware.CorsMiddleware(corsConfig)

	middlewares := []middleware.MiddlewareFunc{
		a.openapiValidateMiddleware(swagger),
		corsMiddleware,
		swaggerUIMiddleware,
		middleware.AccessLogging(a.logger),
		middleware.OTELHandler,
		middleware.FlushTraces(a.flushTraces, a.logger, 3*time.Second),
	}

	if a.env == PROD {
		middlewares = append(middlewares, middleware.BaseNamePrefix(a.logger, "/articles"))
	}

	h := middleware.UseMiddlewares(r, middlewares...)

	s := &http.Server{
		Handler: h,
		Addr:    net.JoinHostPort(host, port),
	}

	return s.ListenAndServe()
}

func (a *API) getLoggerOrBaseLogger(ctx context.Context) *slog.Logger {
	logger, ok := middleware.GetLoggerFromCtx(ctx)
	if !ok {
		a.logger.Error("tried to get logger and it wasnt in the context")
		return a.logger
	}
	return logger
}
