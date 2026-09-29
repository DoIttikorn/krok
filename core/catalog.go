// Package core is the UI-independent heart of krok. It knows every choice a
// generated project can have, the rules between them, and how to turn a set of
// options into files on disk. It imports only the standard library and never
// writes to the terminal, so any front end (CLI flags, Huh forms, Bubble Tea,
// or another Go program) can drive it.
package core

// Choice is one selectable value, such as a framework or a database.
type Choice struct {
	ID          string // value used in flags and Options, e.g. "chi"
	Name        string // human-friendly label, e.g. "Chi"
	Description string
}

// Catalog lists every choice krok supports. UIs read it to build their
// prompts and flag help, so adding an entry here makes it appear everywhere.
type Catalog struct {
	Frameworks []Choice
	Databases  []Choice
	Configs    []Choice // where the generated app reads its settings from
	Features   []Choice // optional extras; any number can be enabled
}

// Framework and database IDs.
const (
	FrameworkGin  = "gin"
	FrameworkEcho = "echo"
	FrameworkChi  = "chi"

	DatabaseNone     = "none"
	DatabasePostgres = "postgres"
	DatabaseMySQL    = "mysql"
	DatabaseMongoDB  = "mongodb"

	ConfigEnv      = "env"
	ConfigKeyVault = "keyvault"

	FeatureOpenAPI   = "openapi"
	FeatureRedis     = "redis"
	FeatureKafka     = "kafka"
	FeatureRabbitMQ  = "rabbitmq"
	FeatureAsynq     = "asynq"
	FeatureRiver     = "river"
	FeatureWatermill = "watermill"
)

// DefaultCatalog returns the built-in choices.
func DefaultCatalog() Catalog {
	return Catalog{
		Frameworks: []Choice{
			{ID: FrameworkGin, Name: "Gin", Description: "Fast, batteries-included HTTP framework"},
			{ID: FrameworkEcho, Name: "Echo", Description: "Minimal, extensible web framework (v5)"},
			{ID: FrameworkChi, Name: "Chi", Description: "Lightweight router built on net/http"},
		},
		Databases: []Choice{
			{ID: DatabasePostgres, Name: "PostgreSQL", Description: "database/sql with the pgx driver"},
			{ID: DatabaseMySQL, Name: "MySQL", Description: "database/sql with go-sql-driver/mysql"},
			{ID: DatabaseMongoDB, Name: "MongoDB", Description: "Official mongo-driver v2"},
			{ID: DatabaseNone, Name: "None", Description: "No database"},
		},
		Configs: []Choice{
			{ID: ConfigEnv, Name: ".env file", Description: "Environment variables, loaded from .env if present"},
			{ID: ConfigKeyVault, Name: "Azure Key Vault", Description: "Secrets from Azure Key Vault, named by AZURE_KEY_VAULT_NAME"},
		},
		Features: []Choice{
			{ID: FeatureOpenAPI, Name: "OpenAPI", Description: "Typed routes with Huma: OpenAPI 3.1 spec and docs at /docs"},
			{ID: FeatureRedis, Name: "Redis", Description: "Redis client (go-redis v9)"},
			{ID: FeatureKafka, Name: "Kafka", Description: "Kafka producer (franz-go), consumer in cmd/worker"},
			{ID: FeatureRabbitMQ, Name: "RabbitMQ", Description: "Publisher for RabbitMQ (amqp091-go), consumer in cmd/worker"},
			{ID: FeatureAsynq, Name: "Asynq", Description: "Background tasks on Redis (enables Redis)"},
			{ID: FeatureRiver, Name: "River", Description: "Background jobs in PostgreSQL (needs --database postgres)"},
			{ID: FeatureWatermill, Name: "Watermill", Description: "Messaging over Kafka, RabbitMQ or Redis Streams"},
		},
	}
}

// aliases maps alternative spellings users may type to canonical IDs.
var aliases = map[string]string{
	"postgresql": DatabasePostgres,
	"pg":         DatabasePostgres,
	"mongo":      DatabaseMongoDB,
	"mariadb":    DatabaseMySQL,
	"key-vault":  ConfigKeyVault,
	"akv":        ConfigKeyVault,
	"rabbit":     FeatureRabbitMQ,
}

// IDs returns the IDs of the given choices, in order.
func IDs(choices []Choice) []string {
	ids := make([]string, len(choices))
	for i, c := range choices {
		ids[i] = c.ID
	}
	return ids
}

func has(choices []Choice, id string) bool {
	for _, c := range choices {
		if c.ID == id {
			return true
		}
	}
	return false
}
