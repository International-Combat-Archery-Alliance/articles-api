# Agent Guidelines for Articles API

This is a Go-based articles backend service using AWS SAM and DynamoDB.

## Build Commands

```bash
make build    # Generate code and build SAM app
make local    # Build and start SAM local API on port 3004
go generate ./...  # Generate Go code from OpenAPI spec
go test ./... # Run all tests
```

## Architecture

Follows the same Hexagonal Architecture (Ports and Adapters) pattern as other ICAA services:

```
cmd/       - Entry point, wires dependencies
api/       - Driving adapters (HTTP handlers, middleware)
articles/  - Domain: Article aggregate, business logic, repository port
dynamo/    - Driven adapters (DynamoDB repository implementation)
```

## Code Style

- Same conventions as event-registration service
- Error types with ErrorReason constants
- Optimistic locking via version attribute
- Base64-encoded JSON cursors for pagination
- No OpenAI/LLM dependencies needed
