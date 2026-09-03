package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/International-Combat-Archery-Alliance/articles-api/api"
	"github.com/International-Combat-Archery-Alliance/auth/token"
	"github.com/International-Combat-Archery-Alliance/telemetry"
	"go.opentelemetry.io/otel"
)

var tracer = otel.Tracer("github.com/International-Combat-Archery-Alliance/articles-api/cmd")

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	logger.Info("starting up")
	if err := run(logger); err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	articlesAPI, traceShutdown, err := setupAPI(logger)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := traceShutdown(shutdownCtx); err != nil {
			logger.Error("failed to shutdown telemetry", "error", err)
		}
	}()
	if err != nil {
		return err
	}

	serverSettings := getServerSettingsFromEnv()

	sigCtx, sigStop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer sigStop()

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- articlesAPI.ListenAndServe(serverSettings.Host, serverSettings.Port)
	}()

	select {
	case <-sigCtx.Done():
		logger.Info("shutting down gracefully")
		return nil
	case err := <-serverErrCh:
		if err != nil {
			logger.Error("error running server", "error", err)
			return err
		}
		return nil
	}
}

func setupAPI(logger *slog.Logger) (*api.API, func(context.Context) error, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	env := getAPIEnvironment()

	licenseKey, err := getNewRelicLicenseKey(ctx, env)
	if err != nil {
		return nil, func(context.Context) error { return nil }, fmt.Errorf("new relic license key: %w", err)
	}

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "otlp.nr-data.net:4317"
	}

	traceShutdown, flushTraces, err := telemetry.Init(ctx, telemetry.Options{
		ServiceName: "articles-api",
		Endpoint:    endpoint,
		APIKey:      licenseKey,
		Lambda:      telemetry.LambdaInfoFromEnv(),
	})
	if err != nil {
		return nil, traceShutdown, fmt.Errorf("telemetry init: %w", err)
	}

	ctx, startupSpan := tracer.Start(ctx, "startup")
	defer startupSpan.End()

	var db api.DB
	if err := func() error {
		ctx, span := tracer.Start(ctx, "init-db")
		defer span.End()

		var err error
		db, err = makeDB(ctx)
		if err != nil {
			span.RecordError(err)
		}
		return err
	}(); err != nil {
		startupSpan.RecordError(err)
		return nil, traceShutdown, err
	}

	validator := token.NewKeyCache(jwksURLForEnv(env))
	if err := validator.StartupFetch(ctx); err != nil {
		logger.Warn("jwks startup fetch failed (non-fatal); user token verification will fail closed until keys are fetched", "error", err)
	}

	articlesAPI := api.NewAPI(db, logger, env, validator, flushTraces)

	return articlesAPI, traceShutdown, nil
}

type ServerSettings struct {
	Host string
	Port string
}

func getServerSettingsFromEnv() ServerSettings {
	return ServerSettings{
		Host: getEnvOrDefault("HOST", "0.0.0.0"),
		Port: getEnvOrDefault("PORT", "8080"),
	}
}
