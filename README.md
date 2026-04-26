# ICAA Articles API

An API for managing articles (blog posts, news) for the ICAA website. Built with Go and deployed as an AWS Lambda function using AWS SAM.

## Tech Stack

- **Language**: Go 1.25+
- **Infrastructure**: AWS Lambda, API Gateway, DynamoDB
- **Deployment**: AWS SAM (Serverless Application Model)
- **API Spec**: OpenAPI 3.0 with code generation via [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
- **Authentication**: JWT (cookie and bearer token) with admin scope
- **Observability**: OpenTelemetry tracing, structured JSON logging

## Code Structure

The project follows a Hexagonal Architecture (Ports and Adapters) pattern:

- `api/` - HTTP handlers, OpenAPI validation middleware, and generated code
- `articles/` - Domain layer: Article aggregate, business logic, and repository port
- `cmd/` - Application entry point, dependency wiring
- `dynamo/` - DynamoDB repository implementation (driven adapter)
- `ptr/` - Pointer utility functions
- `spec/` - OpenAPI 3.0 specification (`api.yaml`)

## API Endpoints

| Endpoint | Auth | Description |
|---|---|---|
| `GET /articles/v1` | Public | List published articles (paginated) |
| `GET /articles/v1/{slug}` | Public | Get a single published article by slug |
| `GET /articles/v1/admin` | Admin | List all articles including drafts |
| `POST /articles/v1` | Admin | Create a new article |
| `PATCH /articles/v1/{slug}` | Admin | Update an article |
| `DELETE /articles/v1/{slug}` | Admin | Delete an article |
| `POST /articles/v1/{slug}/publish` | Admin | Publish a draft article |
| `POST /articles/v1/{slug}/unpublish` | Admin | Unpublish an article |

## Prerequisites

- Go 1.25+
- [AWS SAM CLI](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/install-sam-cli.html)
- Docker
- AWS CLI (configured with appropriate credentials for deployment)

## Local Development

1. **Create an `env.json` file** for local environment variables:

   ```json
   {
     "ICAAArticles": {
       "DYNAMO_TABLE_NAME": "articles-api",
       "OTEL_EXPORTER_OTLP_ENDPOINT": ""
     }
   }
   ```

2. **Start shared infrastructure**:
   Shared infrastructure (DynamoDB, Jaeger) is managed in `icaa.world/docker-compose.yml`.
   ```bash
   cd ../icaa.world && docker compose up -d
   ```

3. **Build and run the API locally**:

   ```bash
   make local
   ```

   This will:
   - Generate API code from the OpenAPI spec
   - Build the SAM application
   - Start the local API server with hot reloading

The local API will be available at `http://localhost:3004`.

## Building

```bash
make build
```

This generates the API code from the OpenAPI spec and builds the SAM application.

## Deployment

The API is deployed via AWS SAM. The CI/CD pipeline is configured in `.github/workflows/go.yml`.

For manual deployment:

```bash
sam deploy --guided
```

## Configuration

| Environment Variable | Description | Default |
|---|---|---|
| `DYNAMO_TABLE_NAME` | DynamoDB table name | `articles-api` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OpenTelemetry collector endpoint | `""` |
| `HOST` | Server host | `0.0.0.0` |
| `PORT` | Server port | `8000` |

## License

See [LICENSE](LICENSE) for details.
