# NDC HTTP Connector - Project Guide

## Project Overview

The NDC HTTP Connector is a configuration-based HTTP engine that converts HTTP APIs to Native Data Connector (NDC) schema, enabling seamless proxy requests from Hasura GraphQL Engine v3 to remote HTTP services. It automatically transforms OpenAPI 2.0 and 3.0 definitions into NDC schema without requiring code generation.

**Key Characteristics:**
- No-code, configuration-based approach
- Multi-API composition support
- Production-ready with retry, timeout, and authentication
- Written in Go 1.24+
- Part of the Hasura ecosystem

## Architecture

### High-Level Components

```
┌─────────────────────────────────────────────────────────────┐
│                    Hasura DDN Engine                         │
│                   (GraphQL Queries)                          │
└───────────────────────┬─────────────────────────────────────┘
                        │
                        │ NDC Protocol
                        │
┌───────────────────────▼─────────────────────────────────────┐
│              NDC HTTP Connector                              │
│  ┌─────────────────────────────────────────────────────┐   │
│  │   Connector Layer (connector/)                      │   │
│  │   - Query/Mutation handlers                          │   │
│  │   - Schema management                                │   │
│  │   - Request/Response transformation                  │   │
│  └──────────────┬──────────────────────────────────────┘   │
│                 │                                            │
│  ┌──────────────▼──────────────────────────────────────┐   │
│  │   Schema Processing (ndc-http-schema/)              │   │
│  │   - OpenAPI 2.0/3.0 conversion                       │   │
│  │   - NDC schema generation                            │   │
│  │   - Configuration management                         │   │
│  └──────────────┬──────────────────────────────────────┘   │
│                 │                                            │
│  ┌──────────────▼──────────────────────────────────────┐   │
│  │   HTTP Client (exhttp/)                             │   │
│  │   - Retry logic                                      │   │
│  │   - Telemetry                                        │   │
│  │   - TLS/mTLS                                         │   │
│  └──────────────┬──────────────────────────────────────┘   │
└─────────────────┼──────────────────────────────────────────┘
                  │
                  │ HTTP/HTTPS
                  │
┌─────────────────▼─────────────────────────────────────────┐
│              External REST APIs                            │
└────────────────────────────────────────────────────────────┘
```

### Module Structure

The project is organized into three main Go modules:

1. **`connector/`** - Core NDC connector implementation
2. **`ndc-http-schema/`** - Schema conversion and CLI tools
3. **`exhttp/`** - Extended HTTP client utilities

## Codebase Structure

```
.
├── connector/                      # Main NDC connector implementation
│   ├── internal/                   # Internal connector logic
│   │   ├── argument/               # Argument processing and presets
│   │   ├── contenttype/            # Content-type handling (JSON, XML, multipart, etc.)
│   │   ├── security/               # Authentication implementations
│   │   ├── request_builder.go      # HTTP request construction
│   │   ├── response_transform.go   # Response transformation logic
│   │   ├── upstream_*.go           # Upstream server management
│   │   └── metadata.go             # Metadata collection
│   ├── connector.go                # Main HTTPConnector type
│   ├── query.go                    # Query (GET) handler
│   ├── mutation.go                 # Mutation (POST/PUT/PATCH/DELETE) handler
│   └── schema.go                   # Schema management
│
├── ndc-http-schema/                # Schema conversion library & CLI
│   ├── command/                    # CLI command implementations
│   │   ├── convert.go              # OpenAPI to NDC conversion
│   │   ├── update.go               # Configuration update
│   │   └── json2yaml.go            # JSON/YAML conversion
│   ├── configuration/              # Configuration management
│   │   ├── schema.go               # Configuration schema
│   │   ├── convert.go              # OpenAPI conversion logic
│   │   └── template.go             # Template handling
│   ├── openapi/                    # OpenAPI parsers
│   │   ├── oas2.go                 # OpenAPI 2.0 parser
│   │   ├── oas3.go                 # OpenAPI 3.0/3.1 parser
│   │   └── internal/               # Shared OpenAPI conversion logic
│   ├── jsonschema/                 # JSON Schema generator
│   ├── ndc/                        # NDC schema types
│   ├── schema/                     # HTTP schema definitions
│   ├── utils/                      # Utility functions
│   └── main.go                     # CLI entry point
│
├── exhttp/                         # Extended HTTP client
│   ├── client.go                   # HTTP client configuration
│   ├── retry.go                    # Retry logic with backoff
│   ├── telemetry.go                # OpenTelemetry integration
│   ├── tls.go                      # TLS/mTLS configuration
│   └── transport.go                # Custom HTTP transport
│
├── server/                         # Server entry point
│   └── main.go                     # NDC server main
│
├── docs/                           # Documentation
│   ├── configuration.md            # Configuration guide
│   ├── authentication.md           # Authentication schemes
│   ├── dynamic_headers.md          # Header forwarding
│   ├── argument_presets.md         # Argument presets
│   ├── response_transform.md       # Response transformation
│   ├── schemaless_request.md       # GraphQL-to-REST proxy
│   └── distribution.md             # Distributed execution
│
├── tests/                          # Test data and integration tests
│   ├── configuration/              # Test configurations
│   └── engine/                     # Engine integration tests
│
├── connector/testdata/             # Connector test fixtures
│   ├── auth/                       # Authentication examples
│   ├── petstore3/                  # Petstore API example
│   ├── jsonplaceholder/            # JSONPlaceholder API example
│   └── ...
│
├── go.mod                          # Main module dependencies
├── go.work                         # Go workspace configuration
├── Makefile                        # Build and development tasks
├── Dockerfile                      # Container image
├── compose.yaml                    # Docker Compose configuration
└── README.md                       # User documentation
```

## Key Components

### 1. HTTPConnector ([connector/connector.go](connector/connector.go))

The core NDC connector implementation that:
- Implements the NDC SDK interface
- Parses and validates configuration
- Manages HTTP clients and upstream servers
- Handles schema generation and capabilities

**Key Methods:**
- `ParseConfiguration()` - Validates and loads configuration
- `TryInitState()` - Initializes connector state
- `GetCapabilities()` - Returns NDC capabilities
- `HealthCheck()` - Checks connector health

### 2. Request Builder ([connector/internal/request_builder.go](connector/internal/request_builder.go))

Constructs HTTP requests from NDC queries/mutations:
- Builds URLs with path/query parameters
- Handles different content types (JSON, XML, form-data, multipart)
- Applies authentication
- Manages headers and presets

### 3. Schema Converter ([ndc-http-schema/](ndc-http-schema/))

Converts API specifications to NDC schema:
- **OpenAPI 2.0/3.0 Parser** - Extracts operations, types, and security
- **NDC Generator** - Converts to NDC functions/procedures
- **Configuration Manager** - Handles multi-file configurations
- **Patch System** - Applies JSON patches to specs

### 4. Authentication ([connector/internal/security/](connector/internal/security/))

Implements multiple authentication schemes:
- **API Key** ([api_key.go](connector/internal/security/api_key.go))
- **Basic Auth** ([basic.go](connector/internal/security/basic.go))
- **Bearer/HTTP Auth** ([http.go](connector/internal/security/http.go))
- **OAuth 2.0** ([oauth2.go](connector/internal/security/oauth2.go))
- **Mutual TLS** ([tls.go](connector/internal/security/tls.go))

### 5. Content Type Handlers ([connector/internal/contenttype/](connector/internal/contenttype/))

Encode/decode different content types:
- **JSON** - Standard JSON serialization
- **XML** - XML encoding/decoding
- **URL-encoded** - Form data encoding
- **Multipart** - File uploads
- **Data URIs** - Base64 encoded content

### 6. Extended HTTP Client ([exhttp/](exhttp/))

Production-ready HTTP client with:
- **Retry Logic** - Exponential backoff with jitter
- **Telemetry** - OpenTelemetry tracing and metrics
- **TLS Configuration** - Certificate management and mTLS
- **Custom Transport** - Request/response interception

## Development Workflow

### Prerequisites

- **Go 1.24+** (as specified in [go.mod](go.mod:3))
- **golangci-lint** for linting
- **Docker** (optional, for integration tests)

### Common Commands

```bash
# Format code
make format

# Run tests
make test

# Run linter
make lint

# Fix linting issues
make lint-fix

# Build CLI tool
make build-cli

# Tidy dependencies
make go-tidy

# Generate JSON schema
make build-jsonschema

# Start DDN environment
make start-ddn

# Stop DDN environment
make stop-ddn
```

### Testing

The project has comprehensive tests across modules:

```bash
# Run all tests
go test -v -race -timeout 3m ./...
cd ndc-http-schema && go test -v -race -timeout 3m ./...
cd exhttp && go test -v -race -timeout 3m ./...

# Run specific package tests
go test -v ./connector/internal/contenttype/
```

Test fixtures are in:
- [connector/testdata/](connector/testdata/) - Connector test configurations
- [ndc-http-schema/command/testdata/](ndc-http-schema/command/testdata/) - CLI test data
- [tests/](tests/) - Integration tests

### Building

```bash
# Build connector server
go build -o _output/ndc-http ./server

# Build CLI tool
go build -o _output/ndc-http-schema ./ndc-http-schema

# Build for multiple platforms (CI)
make ci-build-cli
```

## Configuration Guide

### Main Configuration File

The connector reads `config.{json,yaml}` from the configuration directory:

```yaml
files:
  - file: swagger.json
    spec: openapi2
    timeout:
      value: 30
    retry:
      times:
        value: 1
      delay:
        value: 500
      httpStatus: [429, 500, 502, 503]
  - file: openapi.yaml
    spec: openapi3
    trimPrefix: /v1
    envPrefix: PET_STORE
  - file: schema.json
    spec: ndc
```

### Supported Specifications

- **`oas2`/`openapi2`** - OpenAPI 2.0 (Swagger)
- **`oas3`/`openapi3`** - OpenAPI 3.0/3.1
- **`ndc`** - Native NDC HTTP schema

### Schema Conversion

Convert OpenAPI to NDC schema:

```bash
# From file
ndc-http-schema convert -f swagger.yaml --spec oas2 -o schema.json

# From URL
ndc-http-schema convert -f https://example.com/openapi.yaml --spec oas3 -o schema.json

# With config file
ndc-http-schema convert -c config.yaml
```

### Authentication Configuration

Configure security schemes in your schema:

```yaml
settings:
  securitySchemes:
    api_key:
      type: apiKey
      value:
        env: API_KEY
      in: header
      name: X-API-Key

    bearer:
      type: http
      scheme: bearer
      value:
        env: BEARER_TOKEN
      header: Authorization

    oauth2:
      type: oauth2
      flows:
        clientCredentials:
          tokenUrl:
            value: https://auth.example.com/token
          clientId:
            env: OAUTH2_CLIENT_ID
          clientSecret:
            env: OAUTH2_CLIENT_SECRET
```

### Environment Variables

Environment variables follow the pattern `{{VAR_NAME}}` or `{{VAR_NAME:-default}}`:

```yaml
settings:
  servers:
    - url:
        env: API_SERVER_URL
        value: https://api.example.com
```

## Key Features

### 1. Multi-API Composition

Combine multiple API specifications into a single connector:
- Conflicting types from later files are ignored
- First file type definitions take precedence
- Composable API collections

### 2. Request Types & Content Types

**Supported HTTP Methods:**
- GET (mapped to NDC functions)
- POST, PUT, PATCH, DELETE (mapped to NDC procedures)

**Supported Content Types:**
- `application/json`
- `application/xml`
- `application/x-www-form-urlencoded`
- `multipart/form-data`
- `application/octet-stream` (base64 encoded)
- `text/*`
- `application/x-ndjson`
- `image/*` (base64 encoded)

### 3. Advanced Features

- **Retry with Backoff** - Configurable retry strategy with exponential backoff
- **Timeout Management** - Per-request and global timeouts
- **Header Forwarding** - Forward headers from Hasura engine
- **Argument Presets** - Set default argument values
- **Response Transforms** - Transform API responses before returning
- **Distributed Execution** - Send requests to multiple servers
- **Schemaless Requests** - GraphQL-to-REST proxy without schema

### 4. Observability

- OpenTelemetry integration for traces and metrics
- Structured logging with slog
- Request/response telemetry
- Performance monitoring

## Development Guidelines

### Code Organization

- **Internal packages** - Use for connector implementation details
- **Public APIs** - Keep minimal and stable
- **Tests** - Co-locate tests with implementation (`*_test.go`)
- **Test data** - Use `testdata/` directories

### Error Handling

- Use `fmt.Errorf` with `%w` for error wrapping
- Return errors, don't panic
- Provide context in error messages

### Naming Conventions

- **Types** - PascalCase (e.g., `HTTPConnector`)
- **Functions** - camelCase (e.g., `parseConfiguration`)
- **Constants** - SCREAMING_SNAKE_CASE for env vars
- **Files** - snake_case (e.g., `request_builder.go`)

### Testing Best Practices

- Use table-driven tests
- Test error cases
- Use `testdata/` for fixtures
- Mock external dependencies
- Use `gotest.tools/v3` for assertions

### Dependency Management

The project uses Go workspaces ([go.work](go.work:1)):
- Main module: `github.com/hasura/ndc-http`
- Local modules: `./ndc-http-schema`, `./exhttp`
- Run `make go-tidy` after dependency changes

## Important Files

### Configuration Files

- [go.mod](go.mod) - Main module dependencies
- [go.work](go.work) - Workspace configuration
- [.golangci.yml](.golangci.yml) - Linter configuration
- [Dockerfile](Dockerfile) - Container image definition
- [compose.yaml](compose.yaml) - Docker Compose setup

### Entry Points

- [server/main.go](server/main.go) - NDC connector server
- [ndc-http-schema/main.go](ndc-http-schema/main.go) - CLI tool

### Documentation

- [README.md](README.md) - User-facing documentation
- [docs/configuration.md](docs/configuration.md) - Configuration guide
- [docs/authentication.md](docs/authentication.md) - Authentication setup
- [ndc-http-schema/README.md](ndc-http-schema/README.md) - CLI documentation

## Dependencies

### Key Dependencies

- **NDC SDK** - `github.com/hasura/ndc-sdk-go/v2` - NDC protocol implementation
- **OpenAPI Parser** - `github.com/pb33f/libopenapi` - OpenAPI parsing
- **OAuth2** - `golang.org/x/oauth2` - OAuth 2.0 flows
- **OpenTelemetry** - `go.opentelemetry.io/otel` - Observability
- **JSON Schema** - `github.com/invopop/jsonschema` - Schema generation
- **Testing** - `gotest.tools/v3` - Test utilities

### Internal Dependencies

- `github.com/hasura/ndc-http/ndc-http-schema` - Schema conversion library
- `github.com/hasura/ndc-http/exhttp` - HTTP client utilities

## Common Development Tasks

### Adding a New Content Type

1. Create handler in [connector/internal/contenttype/](connector/internal/contenttype/)
2. Implement encoder/decoder
3. Add tests with fixtures
4. Register in content type router

### Adding Authentication Scheme

1. Create handler in [connector/internal/security/](connector/internal/security/)
2. Implement the security interface
3. Add configuration schema
4. Update [docs/authentication.md](docs/authentication.md)

### Adding OpenAPI Extension

1. Update parser in [ndc-http-schema/openapi/](ndc-http-schema/openapi/)
2. Add to schema conversion logic
3. Update tests in [ndc-http-schema/openapi/internal/](ndc-http-schema/openapi/internal/)

### Updating NDC Schema

1. Modify types in [ndc-http-schema/ndc/](ndc-http-schema/ndc/)
2. Update conversion logic in [ndc-http-schema/configuration/](ndc-http-schema/configuration/)
3. Regenerate JSON schema: `make build-jsonschema`

## Testing with Hasura DDN

### Local Development

```bash
# Build and start connector
docker compose up -d --build ndc-http

# Update connector link
cd tests/engine
ddn connector-link update myapi --add-all-resources

# Build supergraph
ddn supergraph build local

# Start DDN engine
make start-ddn
```

### Test Configuration

Example configurations in [tests/configuration/](tests/configuration/):
- API specifications
- Security configurations
- Multi-file setups
- Patch examples

## Troubleshooting

### Common Issues

1. **Schema validation errors** - Check OpenAPI spec validity
2. **Authentication failures** - Verify environment variables
3. **Timeout errors** - Adjust timeout configuration
4. **Type conflicts** - Check file order in config

### Debug Mode

Enable debug logging:

```bash
# CLI
ndc-http-schema convert --log-level=debug -f openapi.yaml

# Server
HASURA_LOG_LEVEL=debug ./server/main.go serve
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Run `make lint` and `make test`
5. Submit a pull request

## Resources

- [NDC Specification](https://hasura.io/docs/3.0/connectors/introduction/)
- [OpenAPI Specification](https://swagger.io/specification/)
- [Hasura DDN Documentation](https://hasura.io/docs/3.0/)
- [Recipe Repository](https://github.com/hasura/ndc-http-recipes)

## License

Apache License 2.0 - See [LICENSE](LICENSE)
