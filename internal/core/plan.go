package core

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"io/fs"
	"path"
	"sort"
	"strings"
	"text/template"
)

//go:embed all:templates
var embedded embed.FS

// Templates returns the built-in template tree. Pass a different fs.FS to
// BuildPlanFS to use your own templates with the same layout.
func Templates() fs.FS {
	sub, err := fs.Sub(embedded, "templates")
	if err != nil {
		panic(err) // the embed directive guarantees the directory exists
	}
	return sub
}

// File is one file the plan will write, with its content already rendered.
type File struct {
	Path    string // slash-separated, relative to the project directory
	Content []byte
}

// Step is a command run inside the project directory after files are written.
type Step struct {
	Name string   // short label for UIs, e.g. "go mod tidy"
	Args []string // argv, e.g. {"go", "mod", "tidy"}
}

// Plan is everything Generate will do. Because content is rendered up front,
// what a UI previews is exactly what gets written.
type Plan struct {
	Options Options
	Files   []File
	Steps   []Step
}

// TemplateData is the value templates are executed with.
type TemplateData struct {
	Options
	HasDB      bool
	KeyVault   bool     // Config is Azure Key Vault
	DBName     string   // Name made safe for database identifiers, e.g. "my-api" → "my_api"
	K8sName    string   // Name made safe for Kubernetes resource names, e.g. "My_API" → "my-api"
	ItemsStore string   // adapter package for the items repository: the database ID, or "memory"
	DBPort     string   // default port of the chosen database, "" without one
	HasDeps    bool     // the server connects to at least one external service
	HasWorker  bool     // a feature needs cmd/worker to consume messages or run jobs
	Services   []string // docker-compose services the app depends on, e.g. db, redis
	Watermill  string   // broker Watermill uses: kafka, rabbitmq or redis
}

// Has reports whether feature id is enabled: {{if .Has "redis"}}.
func (d TemplateData) Has(id string) bool {
	for _, f := range d.Features {
		if f == id {
			return true
		}
	}
	return false
}

var dbPorts = map[string]string{
	DatabasePostgres: "5432",
	DatabaseMySQL:    "3306",
	DatabaseMongoDB:  "27017",
}

// NewTemplateData derives the template values for normalized options. UIs
// can use it too, e.g. to list the services a project depends on.
func NewTemplateData(o Options) TemplateData {
	dbName := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		}
		return '_'
	}, o.Name)
	d := TemplateData{
		Options:  o,
		HasDB:    o.Database != DatabaseNone,
		KeyVault: o.Config == ConfigKeyVault,
		DBName:   dbName,
		K8sName:  k8sName(o.Name),
		DBPort:   dbPorts[o.Database],
	}
	d.ItemsStore = "memory"
	if d.HasDB {
		d.ItemsStore = o.Database
	}
	d.HasDeps = d.HasDB
	for _, f := range []string{FeatureRedis, FeatureKafka, FeatureRabbitMQ, FeatureAsynq, FeatureRiver, FeatureWatermill} {
		d.HasDeps = d.HasDeps || d.Has(f)
	}
	if d.HasDB {
		d.Services = append(d.Services, "db")
	}
	for _, f := range []string{FeatureRedis, FeatureKafka, FeatureRabbitMQ} {
		if d.Has(f) {
			d.Services = append(d.Services, f)
		}
	}
	for _, f := range []string{FeatureKafka, FeatureRabbitMQ, FeatureAsynq, FeatureRiver, FeatureWatermill} {
		d.HasWorker = d.HasWorker || d.Has(f)
	}
	if d.Has(FeatureWatermill) {
		// Prefer the heavier-duty broker when several are enabled.
		for _, f := range []string{FeatureKafka, FeatureRabbitMQ, FeatureRedis} {
			if d.Has(f) {
				d.Watermill = f
				break
			}
		}
	}
	return d
}

// BuildPlan renders the built-in templates for o. It does not touch the disk.
func BuildPlan(o Options) (Plan, error) {
	return BuildPlanFS(Templates(), o)
}

// BuildPlanFS renders templates from tfs for o.
//
// Layout of tfs:
//
//	base/                 always applied
//	framework/<id>/       applied for the chosen framework
//	database/<id>/        applied for the chosen database (skipped for "none")
//	config/<id>/          applied for the chosen config source
//	logger/<id>/          applied for the chosen logger
//	feature/<id>/         applied for each enabled feature
//
// Every file must end in ".tmpl"; the suffix is stripped from the output
// path. A leading "dot_" in a file name becomes "." (dot_gitignore.tmpl →
// .gitignore), so dotfiles don't affect this repository. Rendered .go files
// are gofmt'ed. A template that renders to only whitespace produces no file,
// so a file can be made conditional by wrapping it in {{if}}; two layers may
// then offer the same path as long as only one renders it. Two layers
// rendering the same path is an error.
func BuildPlanFS(tfs fs.FS, o Options) (Plan, error) {
	o, err := Normalize(o)
	if err != nil {
		return Plan{}, err
	}
	data := NewTemplateData(o)

	layers := []string{"base", path.Join("framework", o.Framework)}
	if data.HasDB {
		layers = append(layers, path.Join("database", o.Database))
	}
	layers = append(layers, path.Join("config", o.Config), path.Join("logger", o.Logger))
	for _, f := range o.Features {
		layers = append(layers, path.Join("feature", f))
	}

	var files []File
	seen := map[string]string{}
	for _, layer := range layers {
		err := fs.WalkDir(tfs, layer, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel := strings.TrimPrefix(p, layer+"/")
			out, ok := outputPath(rel)
			if !ok {
				return fmt.Errorf("template %s: file name must end in .tmpl", p)
			}
			content, err := render(tfs, p, data)
			if err != nil {
				return err
			}
			if len(bytes.TrimSpace(content)) == 0 {
				return nil
			}
			if prev, dup := seen[out]; dup {
				return fmt.Errorf("templates %s and %s both produce %s", prev, p, out)
			}
			seen[out] = p
			if path.Ext(out) == ".go" {
				formatted, err := format.Source(content)
				if err != nil {
					return fmt.Errorf("template %s: rendered Go code does not parse: %w", p, err)
				}
				content = formatted
			}
			files = append(files, File{Path: out, Content: content})
			return nil
		})
		if err != nil {
			return Plan{}, fmt.Errorf("layer %s: %w", layer, err)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	steps := []Step{{Name: "go mod tidy", Args: []string{"go", "mod", "tidy"}}}
	if o.Git {
		steps = append(steps, Step{Name: "git init", Args: []string{"git", "init", "--quiet"}})
	}
	return Plan{Options: o, Files: files, Steps: steps}, nil
}

func outputPath(rel string) (string, bool) {
	if !strings.HasSuffix(rel, ".tmpl") {
		return "", false
	}
	rel = strings.TrimSuffix(rel, ".tmpl")
	dir, name := path.Split(rel)
	if strings.HasPrefix(name, "dot_") {
		name = "." + strings.TrimPrefix(name, "dot_")
	}
	return dir + name, true
}

// k8sName turns name into a valid Kubernetes resource name (RFC 1123 label):
// lowercase letters, digits and '-', at most 63 characters.
func k8sName(name string) string {
	n := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		}
		return '-'
	}, name)
	if len(n) > 63 {
		n = n[:63]
	}
	return strings.Trim(n, "-")
}

// funcs are available in every template.
var funcs = template.FuncMap{
	"lower": strings.ToLower, // e.g. Docker image names, which must be lowercase
	"join":  strings.Join,
}

func render(tfs fs.FS, p string, data TemplateData) ([]byte, error) {
	src, err := fs.ReadFile(tfs, p)
	if err != nil {
		return nil, err
	}
	t, err := template.New(path.Base(p)).Option("missingkey=error").Funcs(funcs).Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", p, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("template %s: %w", p, err)
	}
	return buf.Bytes(), nil
}
