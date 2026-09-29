# Changelog

krok follows [Semantic Versioning](https://semver.org) for its command line:
the flags and the layout of generated projects. Until v1.0.0, minor versions
may change them.

## Unreleased

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
