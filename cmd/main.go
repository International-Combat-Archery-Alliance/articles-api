package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/International-Combat-Archery-Alliance/articles-api/api"
	"github.com/International-Combat-Archery-Alliance/articles-api/dynamo"
	"github.com/International-Combat-Archery-Alliance/auth/token"
	"github.com/International-Combat-Archery-Alliance/telemetry"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"go.opentelemetry.io/otel"
)

const (
	newRelicLicenseEnvVar  = "NEW_RELIC_LICENSE_KEY"
	newRelicLicenseSSMPath = "/newrelic-license-key"
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
	articlesAPI, err := setupAPI(logger)
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

func setupAPI(logger *slog.Logger) (*api.API, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	env := getAPIEnvironment()

	licenseKey, err := getNewRelicLicenseKey(ctx, env)
	if err != nil {
		return nil, fmt.Errorf("new relic license key: %w", err)
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
		return nil, fmt.Errorf("telemetry init: %w", err)
	}

	ctx, startupSpan := tracer.Start(ctx, "startup")
	defer startupSpan.End()

	db, err := makeDB(ctx)
	if err != nil {
		startupSpan.RecordError(err)
		return nil, fmt.Errorf("db init: %w", err)
	}

	signingKeys, currentKeyID, err := getJWTSigningKeys(ctx, env)
	if err != nil {
		startupSpan.RecordError(err)
		return nil, fmt.Errorf("jwt signing keys: %w", err)
	}

	tokenService := token.NewTokenService(
		signingKeys[currentKeyID],
		token.WithSigningKeys(signingKeys, currentKeyID),
	)

	articlesAPI := api.NewAPI(db, logger, env, tokenService, flushTraces)

	_ = traceShutdown

	return articlesAPI, nil
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

func getEnvOrDefault(key string, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}

func makeDB(ctx context.Context) (*dynamo.DB, error) {
	var dynamoClient *dynamodb.Client
	var err error
	if isLocal() {
		dynamoClient, err = createLocalDynamoClient(ctx)
	} else {
		dynamoClient, err = createProdDynamoClient(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamo client: %w", err)
	}

	db := dynamo.NewDB(dynamoClient, os.Getenv("DYNAMO_TABLE_NAME"))
	return db, nil
}

func isLocal() bool {
	return getEnvOrDefault("AWS_SAM_LOCAL", "false") == "true"
}

func getAPIEnvironment() api.Environment {
	if isLocal() {
		return api.LOCAL
	}
	return api.PROD
}

func loadAWSConfig(ctx context.Context, optFns ...func(*config.LoadOptions) error) (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return aws.Config{}, err
	}
	telemetry.InstrumentAWSConfig(&cfg)
	return cfg, nil
}

func createLocalDynamoClient(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := loadAWSConfig(ctx,
		config.WithRegion("localhost"),
		config.WithCredentialsProvider(credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID: "local", SecretAccessKey: "local", SessionToken: "",
				Source: "Mock credentials used above for local instance",
			},
		}),
	)
	if err != nil {
		return nil, err
	}

	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://dynamodb:8000")
	}), nil
}

func createProdDynamoClient(ctx context.Context) (*dynamodb.Client, error) {
	cfg, err := loadAWSConfig(ctx)
	if err != nil {
		return nil, err
	}

	return dynamodb.NewFromConfig(cfg), nil
}

func getNewRelicLicenseKey(ctx context.Context, env api.Environment) (string, error) {
	if env == api.LOCAL {
		return os.Getenv(newRelicLicenseEnvVar), nil
	}

	cfg, err := loadAWSConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	client := ssm.NewFromConfig(cfg)

	result, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(newRelicLicenseSSMPath),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("failed to get New Relic license key from Parameter Store: %w", err)
	}

	return *result.Parameter.Value, nil
}

type jwtSigningKeysData struct {
	CurrentKey string            `json:"currentKey"`
	Keys       map[string]string `json:"keys"`
}

func getJWTSigningKeys(ctx context.Context, env api.Environment) (map[string]token.SigningKey, string, error) {
	if env == api.LOCAL {
		key := os.Getenv("JWT_SIGNING_KEY")
		if key == "" {
			key = "local-development-signing-key-minimum-32-characters-long"
		}
		return map[string]token.SigningKey{
			"local": {ID: "local", Key: []byte(key)},
		}, "local", nil
	}

	cfg, err := loadAWSConfig(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	client := ssm.NewFromConfig(cfg)

	result, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String("/jwtSigningKeys"),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get JWT signing keys from Parameter Store: %w", err)
	}

	var data jwtSigningKeysData
	if err := json.Unmarshal([]byte(*result.Parameter.Value), &data); err != nil {
		return nil, "", fmt.Errorf("failed to parse JWT signing keys JSON: %w", err)
	}

	signingKeys := make(map[string]token.SigningKey)
	for keyID, keyValue := range data.Keys {
		decodedKey, err := base64.StdEncoding.DecodeString(keyValue)
		if err != nil {
			return nil, "", fmt.Errorf("failed to decode base64 key %q: %w", keyID, err)
		}
		signingKeys[keyID] = token.SigningKey{
			ID:  keyID,
			Key: decodedKey,
		}
	}

	if _, ok := signingKeys[data.CurrentKey]; !ok {
		return nil, "", fmt.Errorf("current key ID %q not found in keys", data.CurrentKey)
	}

	return signingKeys, data.CurrentKey, nil
}
