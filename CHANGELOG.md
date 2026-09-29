# Changelog

krok follows [Semantic Versioning](https://semver.org) for its command line:
the flags and the layout of generated projects. Until v1.0.0, minor versions
may change them.

## v0.2.0

- The `items` example is laid out as ports and adapters: the domain package
  holds the model, `Service` and `Repository` interface; `memory`,
  `postgres`, `mysql` and `mongodb` adapters store items in a real table or
  collection; `itemstest` is a contract every adapter must pass (run against
  the real database with `make test-integration`); `handler` is the REST
  adapter. Shared HTTP helpers moved to `internal/httpx`.
- MySQL connections report rows matched rather than changed
  (`clientFoundRows`), so updating an item with the same values succeeds.
- `--log slog|zap|zerolog|charm` picks the logging backend (default `slog`).
  Generated code always logs through `*slog.Logger`, passed down from `main`;
  only `internal/logger/handler.go` knows the backend. `LOG_LEVEL` and
  `LOG_FORMAT` (`text` or `json`; the Docker image sets `json`) configure it.
  The interactive wizard asks for it next to the extras.
- One request logger for every framework (`httpx.Log`), which also turns
  panics into 500 responses, replaces Gin's, Echo's and Chi's own loggers and
  recoverers. River, Asynq, Watermill, Echo and go-redis log through the same
  logger, so a pod's output is one consistent stream. Each backend encodes
  `duration` its own way (nanoseconds, seconds or milliseconds).

## v0.1.1

- Moved `core` to `internal/core`. krok is a command, not a library, so other
  modules can no longer import it and it can change in any release.
- RabbitMQ: the client now connects lazily and reconnects, so `/readyz`
  recovers after a RabbitMQ restart instead of keeping every API pod out of
  the Service; the worker reconnects with backoff instead of exiting and
  stopping the other consumers.
- Docker images of Gin projects run with `GIN_MODE=release`.
- MySQL's Docker Compose healthcheck pings over TCP, so it no longer passes
  while the entrypoint's temporary init server is running.

## v0.1.0

First release.

- `krok new` generates a Go API project, interactively with Huh prompts or
  fully from flags.
- Frameworks: Gin, Echo v5, Chi. Databases: PostgreSQL, MySQL, MongoDB, none.
- Configuration from `.env`, or from Azure Key Vault with `--key-vault`.
- Extras: `--openapi` (Huma, OpenAPI 3.1 and docs), `--redis`, `--kafka`,
  `--rabbitmq`, `--asynq`, `--river`, `--watermill`. Consumers run in a
  separate `cmd/worker`.
- Every project has a REST example at `/api/v1/items`, RFC 9457 errors,
  `/livez`, `/readyz` and `/health` probes, draining on shutdown, air live
  reload, a Makefile, a Dockerfile, Docker Compose and Kustomize manifests
  for highly available Kubernetes deployments.
- `core` is a standard-library-only package other programs can use to plan
  and generate projects.
