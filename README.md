# krok

[![CI](https://github.com/DoIttikorn/krok/actions/workflows/ci.yml/badge.svg)](https://github.com/DoIttikorn/krok/actions/workflows/ci.yml)

Scaffold a ready-to-run Go API project with the framework and database you choose.

> krok is at v0.x: flags and the generated layout may still change between
> minor versions. See [CHANGELOG.md](CHANGELOG.md).

```bash
go install github.com/DoIttikorn/krok@latest
krok new
```

`krok new` with no flags asks everything interactively. Pass flags to skip
questions, or pass them all to run without a terminal (CI, scripts):

```bash
krok new my-api --framework chi --database postgres --module github.com/you/my-api
krok new my-api -f echo -d mysql --key-vault   # settings from Azure Key Vault
krok new my-api -f chi -d postgres --redis --kafka --river
krok new my-api -f gin -d none --dry-run       # preview files, write nothing
```

| Flag | Values |
| --- | --- |
| `-f, --framework` | `gin`, `echo`, `chi` |
| `-d, --database` | `postgres`, `mysql`, `mongodb`, `none` |
| `-m, --module` | Go module path (default: project name) |
| `--key-vault` | read settings from Azure Key Vault instead of `.env` (default: `.env`) |
| `--openapi` | register routes with [Huma](https://huma.rocks): OpenAPI 3.1 spec and docs at `/docs` |
| `--redis` | Redis client (go-redis v9) |
| `--kafka` | Kafka producer and consumer (franz-go) |
| `--rabbitmq` | RabbitMQ publisher and consumer (amqp091-go) |
| `--asynq` | background tasks on Redis ([Asynq](https://github.com/hibiken/asynq)); turns on `--redis` |
| `--river` | background jobs in PostgreSQL ([River](https://riverqueue.com)); needs `-d postgres` |
| `--watermill` | pub/sub with [Watermill](https://watermill.io) over Kafka, RabbitMQ or Redis Streams; needs one of them |
| `--git` | run `git init` (default `true`; disable with `--git=false`) |
| `-y, --yes` | skip the confirmation prompt |
| `--dry-run` | show the plan without writing |

If `krok` is not found after installing, add Go's bin directory to your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

## Generated project

```
my-api/
├── cmd/api/main.go            entry point; drains on SIGTERM for rolling updates
├── internal/config/           loads settings: .env, or Azure Key Vault with --key-vault
├── internal/server/           routes, probes (/livez, /readyz, /health), RFC 9457 errors
├── internal/items/            example domain (/api/v1/items), ports and adapters:
│   ├── memory/, postgres/ …   repository adapters, one per store
│   ├── itemstest/             contract every adapter must pass
│   └── handler/               REST adapter for the chosen framework
├── internal/httpx/            JSON and RFC 9457 helpers for handlers
├── internal/database/         connection + health check (if a database is chosen)
├── internal/redis, kafka, …   one package per extra (--redis, --kafka, …)
├── cmd/worker/main.go         background consumers, when an extra needs one
├── Dockerfile                 multi-stage build to a distroless image
├── docker-compose.yml         the app, plus the database if one is chosen
├── .air.toml                  live reload for `make watch`
├── deploy/k8s/                Kustomize: Deployment, Service, Ingress (nginx), PDB, HPA
├── .env, Makefile, README.md
└── go.mod
```

### REST API, probes and high availability

Every project serves a CRUD example at `/api/v1/items` (`GET`, `POST` → `201`
with `Location`, `GET`/`PUT`/`DELETE` by ID) with RFC 9457 problem details for
errors. `items` is laid out as ports and adapters, as a model for your own
domains:

- the domain package (`internal/items`) holds the model, the `Service` and
  the `Repository` interface, and imports no driver or framework;
- `memory` and the adapter for the chosen database (`postgres`, `mysql` or
  `mongodb`, with a real table or collection) implement `Repository`;
  `internal/server` is the one place that picks which;
- `itemstest` is the contract every adapter must pass. It runs for the memory
  adapter in `make test` and against the real database in
  `make test-integration`, so the service can be tested with the memory
  adapter and trusted with the real one;
- `handler` is the REST adapter. With `--openapi` it registers the routes with
  Huma, which validates requests from the Go types and serves the spec at
  `/openapi.json` and docs at `/docs`, for Gin, Echo and Chi alike.

The probes are mounted outside the framework router:

- `/livez` only says the process is up, so a database outage doesn't make
  Kubernetes restart every pod.
- `/readyz` checks every dependency and fails during shutdown, so Kubernetes
  stops routing to a pod without restarting it.
- `/health` shows each dependency's status.

On SIGTERM the server fails `/readyz`, waits `SHUTDOWN_DELAY` for
ingress-nginx to notice, then drains in-flight requests. `deploy/k8s`
runs at least two replicas spread across nodes and zones, with
`maxUnavailable: 0` rollouts, a PodDisruptionBudget and an HPA; the worker,
when there is one, gets its own Deployment with probes on port 8081.

Every project gets `make run`, `make watch` (air live reload), `make test`,
`make docker-build`, `make up` / `make down` (Docker Compose), `make k8s` /
`make k8s-apply` (Kustomize), and
`make deps-up` to start only its databases and brokers in Docker. Projects
with a worker also get `make run-worker`.

### Extras and the worker

Each extra adds a package under `internal/`, its settings to `.env`, a
check to `/health`, and a service to `docker-compose.yml`. The API publishes
messages and enqueues work. Consumers run in `cmd/worker`, which ships in the
same Docker image with entrypoint `/worker`, so on Kubernetes the API and the
worker can be separate Deployments that scale independently.

| Extra | API side | Worker side |
| --- | --- | --- |
| Kafka | `kafka.Client.Publish` | `kafka.RunWorker` (consumer group) |
| RabbitMQ | `rabbitmq.Client.Publish` | `rabbitmq.RunWorker` |
| Asynq | `tasks.Client.Enqueue` | `tasks.RunWorker` |
| River | `jobs.Client.Insert` | `jobs.RunWorker` (migrates on startup under an advisory lock) |
| Watermill | `events.Publish` | `events.RunWorker` (router) |

When Watermill is combined with several brokers it uses Kafka first, then
RabbitMQ, then Redis Streams. Its Kafka backend uses Sarama, so a project
with both `--kafka` and `--watermill` includes two Kafka clients.

### Configuration: `.env` or Azure Key Vault

Both modes put settings into environment variables before the server starts,
so the rest of the code just calls `os.Getenv`.

- **Default:** `.env` is loaded if present.
- **`--key-vault`:** at startup the app reads every enabled secret from the
  vault named by `AZURE_KEY_VAULT_NAME` and turns `DB-PASSWORD` into
  `DB_PASSWORD`. It authenticates with `DefaultAzureCredential` (`az login`
  locally, managed identity in Azure). Values already set in the environment
  or `.env` win over the vault.

## Layout of this repository

```
krok/
├── main.go            entry point for go install
├── internal/core/     options, rules, plan, generate (standard library only)
│   └── templates/     embedded project templates
├── internal/cli/      Cobra + Fang: flags → core.Options
└── internal/tui/      Huh forms, Lip Gloss preview, spinner
```

`internal/core` holds every choice, rule and template and never imports Charm
or prints to the terminal (`TestStdlibOnly` enforces this). The CLI and the
TUI only collect options and show progress:

```go
p, err := core.BuildPlan(core.Options{Name: "my-api", Framework: "chi", Database: "postgres"})
// preview p.Files, then:
err = core.Generate(ctx, p, "my-api", func(e core.Event) { /* progress */ })
```

It lives under `internal/` because krok is a command, not a library: other
modules can't import it, so it can change in any release.

## Templates

`internal/core/templates` has three layers, applied in order:

- `base/`: always
- `framework/<id>/`: the chosen framework
- `database/<id>/`: the chosen database (skipped for `none`)
- `config/<id>/`: the chosen config source (`env` or `keyvault`)
- `feature/<id>/`: each enabled extra

A template that renders to only whitespace produces no file, so wrapping a
whole file in `{{if}}` makes it conditional (`cmd/worker/main.go.tmpl` does
this). Templates get the options plus helpers such as `.HasDB`, `.HasWorker`,
`.Services` and `.Has "redis"` (see `TemplateData` in `internal/core/plan.go`).

Every file ends in `.tmpl` and is rendered with `text/template`. A `dot_`
prefix becomes `.` (`dot_env.tmpl` → `.env`). Rendered `.go` files are
gofmt'ed, and two layers producing the same path is an error. To add a
framework or database, add it to `DefaultCatalog` in `internal/core/catalog.go` and
create its template directory. It then shows up in the flags, the prompts
and the tests automatically. Templates can use `lower` and `join`, e.g.
`{{lower .Name}}`. Rules between extras live in `normalizeFeatures` in
`internal/core/options.go`.

## Development

```bash
go test ./...                                  # unit + golden tests (offline)
go test ./internal/core -update                # rewrite golden files after changing templates
KROK_E2E=1 go test ./internal/core -run E2E    # generate every combination: go mod tidy, vet and
                                               # test each, and check docker-compose.yml and
                                               # deploy/k8s when Docker Compose and kubectl exist
KROK_E2E=1 KROK_E2E_DOCKER=1 go test ./internal/core -run E2E
                                               # also run each database adapter's contract
                                               # against a real database in Docker
```

## License

[MIT](LICENSE)
