package core

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"strings"
)

// Options describes the project to generate. UIs fill it in; Normalize
// validates it and applies defaults and rules.
type Options struct {
	Name       string   // project directory and binary name, e.g. "myapi"
	ModulePath string   // Go module path; defaults to Name
	Framework  string   // one of Catalog.Frameworks IDs
	Database   string   // one of Catalog.Databases IDs
	Config     string   // one of Catalog.Configs IDs; defaults to "env"
	Logger     string   // one of Catalog.Loggers IDs; defaults to "slog"
	Features   []string // Catalog.Features IDs; Normalize sorts them in catalog order
	GoVersion  string   // "go" directive in go.mod; defaults to the running toolchain
	Git        bool     // run "git init" after generating
}

// FallbackGoVersion is used when the running toolchain version can't be parsed.
const FallbackGoVersion = "1.24"

var (
	nameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	moduleRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]*(/[A-Za-z0-9._~-]+)*$`)
	goVerRe  = regexp.MustCompile(`^1\.\d+(\.\d+)?$`)
)

// Normalize trims and canonicalizes o, fills in defaults, and validates it
// against the catalog. Every UI should call it so they all behave the same.
func Normalize(o Options) (Options, error) {
	cat := DefaultCatalog()

	o.Name = strings.TrimSpace(o.Name)
	o.ModulePath = strings.TrimSpace(o.ModulePath)
	o.Framework = canonical(o.Framework)
	o.Database = canonical(o.Database)
	o.Config = canonical(o.Config)
	o.Logger = canonical(o.Logger)
	o.GoVersion = strings.TrimPrefix(strings.TrimSpace(o.GoVersion), "go")

	if o.ModulePath == "" {
		o.ModulePath = o.Name
	}
	if o.GoVersion == "" {
		o.GoVersion = defaultGoVersion()
	}
	if o.Config == "" {
		o.Config = ConfigEnv
	}
	if o.Logger == "" {
		o.Logger = LoggerSlog
	}

	var errs []error
	if err := ValidateName(o.Name); err != nil {
		errs = append(errs, err)
	}
	if o.ModulePath != "" && !moduleRe.MatchString(o.ModulePath) {
		errs = append(errs, fmt.Errorf("invalid module path %q", o.ModulePath))
	}
	switch {
	case o.Framework == "":
		errs = append(errs, fmt.Errorf("framework is required (one of %s)", strings.Join(IDs(cat.Frameworks), ", ")))
	case !has(cat.Frameworks, o.Framework):
		errs = append(errs, fmt.Errorf("unknown framework %q (one of %s)", o.Framework, strings.Join(IDs(cat.Frameworks), ", ")))
	}
	switch {
	case o.Database == "":
		errs = append(errs, fmt.Errorf("database is required (one of %s)", strings.Join(IDs(cat.Databases), ", ")))
	case !has(cat.Databases, o.Database):
		errs = append(errs, fmt.Errorf("unknown database %q (one of %s)", o.Database, strings.Join(IDs(cat.Databases), ", ")))
	}
	features, err := normalizeFeatures(o.Database, o.Features)
	if err != nil {
		errs = append(errs, err)
	}
	o.Features = features
	if !has(cat.Loggers, o.Logger) {
		errs = append(errs, fmt.Errorf("unknown logger %q (one of %s)", o.Logger, strings.Join(IDs(cat.Loggers), ", ")))
	}
	if !has(cat.Configs, o.Config) {
		errs = append(errs, fmt.Errorf("unknown config source %q (one of %s)", o.Config, strings.Join(IDs(cat.Configs), ", ")))
	}
	if !goVerRe.MatchString(o.GoVersion) {
		errs = append(errs, fmt.Errorf("invalid Go version %q", o.GoVersion))
	}
	return o, errors.Join(errs...)
}

// ValidateFeatures reports whether features can be combined with database.
// UIs can use it for inline validation before calling Normalize.
func ValidateFeatures(database string, features []string) error {
	_, err := normalizeFeatures(canonical(database), features)
	return err
}

// normalizeFeatures canonicalizes and validates features and applies the
// rules between them. It returns them in catalog order, or nil if none.
func normalizeFeatures(database string, in []string) ([]string, error) {
	cat := DefaultCatalog()
	on := map[string]bool{}
	var errs []error
	for _, f := range in {
		f = canonical(f)
		switch {
		case f == "":
		case !has(cat.Features, f):
			errs = append(errs, fmt.Errorf("unknown feature %q (one of %s)", f, strings.Join(IDs(cat.Features), ", ")))
		default:
			on[f] = true
		}
	}

	// Asynq keeps its queues in Redis, so it brings Redis along.
	if on[FeatureAsynq] {
		on[FeatureRedis] = true
	}
	if on[FeatureRiver] && database != DatabasePostgres {
		errs = append(errs, errors.New("river stores jobs in PostgreSQL: use --database postgres"))
	}
	if on[FeatureWatermill] && !on[FeatureKafka] && !on[FeatureRabbitMQ] && !on[FeatureRedis] {
		errs = append(errs, errors.New("watermill needs a broker: add kafka, rabbitmq or redis"))
	}

	var out []string
	for _, c := range cat.Features {
		if on[c.ID] {
			out = append(out, c.ID)
		}
	}
	return out, errors.Join(errs...)
}

// ValidateName reports whether name can be used as a project directory.
// UIs can use it for inline validation before calling Normalize.
func ValidateName(name string) error {
	switch {
	case name == "":
		return errors.New("project name is required")
	case !nameRe.MatchString(name):
		return fmt.Errorf("invalid project name %q: use letters, digits, '.', '_' or '-', starting with a letter or digit", name)
	}
	return nil
}

func canonical(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	if a, ok := aliases[id]; ok {
		return a
	}
	return id
}

// defaultGoVersion returns "major.minor" of the running toolchain, e.g. "1.26".
func defaultGoVersion() string {
	v := strings.TrimPrefix(runtime.Version(), "go")
	if parts := strings.SplitN(v, ".", 3); len(parts) >= 2 {
		// Rejects devel builds and pre-releases like "1.27rc1".
		if mm := parts[0] + "." + parts[1]; goVerRe.MatchString(mm) {
			return mm
		}
	}
	return FallbackGoVersion
}
