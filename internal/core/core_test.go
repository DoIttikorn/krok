package core

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata/golden")

// combos returns one Options value per framework × database × config, plus
// feature sets covering each feature, each Watermill broker and everything
// at once.
func combos() []Options {
	cat := DefaultCatalog()
	base := Options{Name: "my-api", ModulePath: "github.com/example/my-api", GoVersion: "1.24", Git: true}

	var out []Options
	for _, fw := range cat.Frameworks {
		for _, db := range cat.Databases {
			for _, cfg := range cat.Configs {
				o := base
				o.Framework, o.Database, o.Config = fw.ID, db.ID, cfg.ID
				out = append(out, o)
			}
		}
	}
	for _, fw := range cat.Frameworks {
		o := base
		o.Framework, o.Database, o.Config, o.Features = fw.ID, "none", "env", []string{"openapi"}
		out = append(out, o)
	}
	for _, fs := range [][]string{
		{"redis"},
		{"kafka"},
		{"rabbitmq"},
		{"asynq"},
		{"river"},
		{"watermill", "kafka"},
		{"watermill", "rabbitmq"},
		{"watermill", "redis"},
		{"openapi", "redis", "kafka", "rabbitmq", "asynq", "river", "watermill"},
	} {
		for _, cfg := range cat.Configs {
			o := base
			o.Framework, o.Database, o.Config, o.Features = "chi", "postgres", cfg.ID, fs
			out = append(out, o)
		}
	}
	return out
}

func comboName(o Options) string {
	name := o.Framework + "-" + o.Database + "-" + o.Config
	if len(o.Features) > 0 {
		name += "+" + strings.Join(o.Features, "+")
	}
	return name
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name    string
		in      Options
		want    Options
		wantErr string
	}{
		{
			name: "defaults module path",
			in:   Options{Name: "api", Framework: "chi", Database: "none", GoVersion: "1.24"},
			want: Options{Name: "api", ModulePath: "api", Framework: "chi", Database: "none", Config: "env", GoVersion: "1.24"},
		},
		{
			name: "canonicalizes case and aliases",
			in:   Options{Name: " api ", Framework: "GIN", Database: "PostgreSQL", Config: "Key-Vault", GoVersion: "go1.25"},
			want: Options{Name: "api", ModulePath: "api", Framework: "gin", Database: "postgres", Config: "keyvault", GoVersion: "1.25"},
		},
		{
			name: "mongo alias",
			in:   Options{Name: "api", Framework: "echo", Database: "mongo", GoVersion: "1.24"},
			want: Options{Name: "api", ModulePath: "api", Framework: "echo", Database: "mongodb", Config: "env", GoVersion: "1.24"},
		},
		{name: "missing name", in: Options{Framework: "chi", Database: "none"}, wantErr: "project name is required"},
		{name: "bad name", in: Options{Name: "my api", Framework: "chi", Database: "none"}, wantErr: "invalid project name"},
		{name: "bad module", in: Options{Name: "api", ModulePath: "github.com//x", Framework: "chi", Database: "none"}, wantErr: "invalid module path"},
		{name: "missing framework", in: Options{Name: "api", Database: "none"}, wantErr: "framework is required"},
		{name: "unknown framework", in: Options{Name: "api", Framework: "fiber", Database: "none"}, wantErr: `unknown framework "fiber"`},
		{name: "unknown database", in: Options{Name: "api", Framework: "chi", Database: "sqlite"}, wantErr: `unknown database "sqlite"`},
		{
			name: "features in catalog order, asynq brings redis",
			in:   Options{Name: "api", Framework: "chi", Database: "none", GoVersion: "1.24", Features: []string{"asynq", "Rabbit", "kafka", "kafka"}},
			want: Options{Name: "api", ModulePath: "api", Framework: "chi", Database: "none", Config: "env", GoVersion: "1.24", Features: []string{"redis", "kafka", "rabbitmq", "asynq"}},
		},
		{name: "unknown feature", in: Options{Name: "api", Framework: "chi", Database: "none", Features: []string{"nats"}}, wantErr: `unknown feature "nats"`},
		{name: "river needs postgres", in: Options{Name: "api", Framework: "chi", Database: "mysql", Features: []string{"river"}}, wantErr: "river stores jobs in PostgreSQL"},
		{name: "watermill needs a broker", in: Options{Name: "api", Framework: "chi", Database: "none", Features: []string{"watermill"}}, wantErr: "watermill needs a broker"},
		{name: "unknown config", in: Options{Name: "api", Framework: "chi", Database: "none", Config: "vault"}, wantErr: `unknown config source "vault"`},
		{name: "bad go version", in: Options{Name: "api", Framework: "chi", Database: "none", GoVersion: "2"}, wantErr: "invalid Go version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Normalize(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestNormalizeDefaultGoVersion(t *testing.T) {
	got, err := Normalize(Options{Name: "api", Framework: "chi", Database: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if !goVerRe.MatchString(got.GoVersion) {
		t.Errorf("GoVersion = %q, want major.minor", got.GoVersion)
	}
}

// TestGolden renders every combination and compares it with
// testdata/golden/<framework>-<database>.golden. Run with -update after
// changing templates, then review the diff.
func TestGolden(t *testing.T) {
	for _, o := range combos() {
		t.Run(comboName(o), func(t *testing.T) {
			p, err := BuildPlan(o)
			if err != nil {
				t.Fatal(err)
			}
			got := archive(p)
			golden := filepath.Join("testdata", "golden", comboName(o)+".golden")
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run: go test ./internal/core -update)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from %s; run: go test ./internal/core -update and review the diff", golden)
			}
		})
	}
}

// archive serializes a plan in a txtar-like format that is easy to diff.
func archive(p Plan) []byte {
	var b bytes.Buffer
	for _, s := range p.Steps {
		b.WriteString("# step: " + strings.Join(s.Args, " ") + "\n")
	}
	for _, f := range p.Files {
		b.WriteString("-- " + f.Path + " --\n")
		b.Write(f.Content)
		if !bytes.HasSuffix(f.Content, []byte("\n")) {
			b.WriteString("\n")
		}
	}
	return b.Bytes()
}

func TestPlanLayers(t *testing.T) {
	paths := func(o Options) map[string]bool {
		t.Helper()
		p, err := BuildPlan(o)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]bool{}
		for _, f := range p.Files {
			m[f.Path] = true
		}
		return m
	}
	check := func(name string, got map[string]bool, want, unwanted []string) {
		t.Helper()
		for _, w := range want {
			if !got[w] {
				t.Errorf("%s: missing %s", name, w)
			}
		}
		for _, u := range unwanted {
			if got[u] {
				t.Errorf("%s: should not contain %s", name, u)
			}
		}
	}
	always := []string{
		".air.toml", ".dockerignore", ".env", ".gitignore", "Dockerfile", "Makefile", "README.md",
		"docker-compose.yml", "go.mod", "cmd/api/main.go", "internal/config/config.go",
		"internal/httpx/httpx.go", "internal/server/health.go",
		"internal/items/items.go", "internal/items/repository.go", "internal/items/service.go",
		"internal/items/memory/memory.go", "internal/items/itemstest/itemstest.go",
		"internal/items/handler/handler.go",
		"internal/server/routes.go", "internal/server/server.go",
		"deploy/k8s/kustomization.yaml", "deploy/k8s/deployment.yaml", "deploy/k8s/service.yaml",
		"deploy/k8s/ingress.yaml", "deploy/k8s/pdb.yaml", "deploy/k8s/hpa.yaml",
	}

	check("postgres", paths(Options{Name: "api", Framework: "chi", Database: "postgres"}),
		append(always, "internal/database/database.go", "internal/items/postgres/repository.go", "internal/items/postgres/repository_test.go"),
		[]string{"internal/config/config_test.go", "internal/server/openapi.go", "deploy/k8s/worker-deployment.yaml", "internal/items/mongodb/repository.go"})
	check("mongodb", paths(Options{Name: "api", Framework: "echo", Database: "mongodb"}),
		append(always, "internal/items/mongodb/repository.go"),
		[]string{"internal/items/postgres/repository.go"})
	check("openapi", paths(Options{Name: "api", Framework: "gin", Database: "none", Features: []string{"openapi"}}),
		append(always, "internal/server/openapi.go"),
		[]string{"internal/items/postgres/repository.go"})
	check("no database", paths(Options{Name: "api", Framework: "chi", Database: "none"}),
		always,
		[]string{"internal/database/database.go"})
	check("key vault", paths(Options{Name: "api", Framework: "chi", Database: "none", Config: "keyvault"}),
		append(always, "internal/config/config_test.go", "deploy/k8s/serviceaccount.yaml"),
		nil)
	check("redis only", paths(Options{Name: "api", Framework: "chi", Database: "none", Features: []string{"redis"}}),
		append(always, "internal/redis/redis.go"),
		[]string{"cmd/worker/main.go"})
	check("kafka", paths(Options{Name: "api", Framework: "chi", Database: "none", Features: []string{"kafka"}}),
		append(always, "internal/kafka/kafka.go", "cmd/worker/main.go", "deploy/k8s/worker-deployment.yaml", "deploy/k8s/worker-pdb.yaml"),
		nil)
}

func TestK8sName(t *testing.T) {
	tests := map[string]string{
		"my-api":    "my-api",
		"My_API.v2": "my-api-v2",
		"-x-":       "x",
	}
	for in, want := range tests {
		if got := k8sName(in); got != want {
			t.Errorf("k8sName(%q) = %q, want %q", in, got, want)
		}
	}
}

// The framework layer and the openapi layer both offer
// internal/items/handler/handler.go; exactly one of them renders it.
func TestItemsHandlerVariant(t *testing.T) {
	content := func(o Options) string {
		t.Helper()
		p, err := BuildPlan(o)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range p.Files {
			if f.Path == "internal/items/handler/handler.go" {
				return string(f.Content)
			}
		}
		t.Fatalf("%+v: no handler", o)
		return ""
	}
	if c := content(Options{Name: "api", Framework: "echo", Database: "none"}); !strings.Contains(c, "*echo.Group") {
		t.Error("echo project without openapi should get the Echo handler")
	}
	if c := content(Options{Name: "api", Framework: "echo", Database: "none", Features: []string{"openapi"}}); !strings.Contains(c, "huma.Register") || strings.Contains(c, "echo.Group") {
		t.Error("echo project with openapi should get the Huma handler only")
	}
}

func TestTemplateData(t *testing.T) {
	o, err := Normalize(Options{Name: "api", Framework: "chi", Database: "postgres", Features: []string{"watermill", "redis", "rabbitmq"}})
	if err != nil {
		t.Fatal(err)
	}
	d := NewTemplateData(o)
	if want := []string{"db", "redis", "rabbitmq"}; !reflect.DeepEqual(d.Services, want) {
		t.Errorf("Services = %v, want %v", d.Services, want)
	}
	if !d.HasWorker || !d.HasDeps {
		t.Errorf("HasWorker = %v, HasDeps = %v, want both true", d.HasWorker, d.HasDeps)
	}
	if NewTemplateData(Options{Name: "api", Framework: "chi", Database: "none", Features: []string{"openapi"}}).HasDeps {
		t.Error("openapi alone should not count as an external dependency")
	}
	if d.ItemsStore != "postgres" {
		t.Errorf("ItemsStore = %q, want postgres", d.ItemsStore)
	}
	if got := NewTemplateData(Options{Name: "api", Framework: "chi", Database: "none"}).ItemsStore; got != "memory" {
		t.Errorf("ItemsStore without a database = %q, want memory", got)
	}
	if d.Watermill != "rabbitmq" {
		t.Errorf("Watermill = %q, want rabbitmq (preferred over redis)", d.Watermill)
	}
}

func TestEmptyTemplateSkipsFile(t *testing.T) {
	tfs := fstest.MapFS{
		"base/keep.txt.tmpl":      {Data: []byte("x")},
		"base/skip.txt.tmpl":      {Data: []byte("{{if .HasWorker}}worker{{end}}\n")},
		"framework/chi/a.go.tmpl": {Data: []byte("package a\n")},
		"config/env/b.txt.tmpl":   {Data: []byte("b")},
		// Same output path as base/keep.txt.tmpl, but renders empty: allowed.
		"framework/chi/keep.txt.tmpl": {Data: []byte("{{if .HasWorker}}other{{end}}")},
	}
	p, err := BuildPlanFS(tfs, Options{Name: "api", Framework: "chi", Database: "none"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range p.Files {
		if f.Path == "skip.txt" {
			t.Error("skip.txt rendered empty but was still planned")
		}
	}
}

func TestSteps(t *testing.T) {
	p, err := BuildPlan(Options{Name: "api", Framework: "chi", Database: "none"})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Steps) != 1 {
		t.Errorf("Git=false: steps = %v, want only go mod tidy", p.Steps)
	}
}

func TestBuildPlanFSErrors(t *testing.T) {
	o := Options{Name: "api", Framework: "chi", Database: "postgres"}
	tests := []struct {
		name    string
		fs      fstest.MapFS
		wantErr string
	}{
		{
			name: "missing .tmpl suffix",
			fs: fstest.MapFS{
				"base/README.md":           {Data: []byte("x")},
				"framework/chi/a.tmpl":     {Data: []byte("x")},
				"database/postgres/b.tmpl": {Data: []byte("x")},
			},
			wantErr: "must end in .tmpl",
		},
		{
			name: "two layers produce the same file",
			fs: fstest.MapFS{
				"base/x.tmpl":              {Data: []byte("a")},
				"framework/chi/x.tmpl":     {Data: []byte("b")},
				"database/postgres/y.tmpl": {Data: []byte("c")},
				"config/env/c.tmpl":        {Data: []byte("x")},
			},
			wantErr: "both produce x",
		},
		{
			name: "unknown field",
			fs: fstest.MapFS{
				"base/x.tmpl":              {Data: []byte("{{.Nope}}")},
				"framework/chi/y.tmpl":     {Data: []byte("")},
				"database/postgres/z.tmpl": {Data: []byte("")},
				"config/env/c.tmpl":        {Data: []byte("x")},
			},
			wantErr: "Nope",
		},
		{
			name: "invalid Go output",
			fs: fstest.MapFS{
				"base/main.go.tmpl":        {Data: []byte("package main\nfunc {")},
				"framework/chi/y.tmpl":     {Data: []byte("")},
				"database/postgres/z.tmpl": {Data: []byte("")},
				"config/env/c.tmpl":        {Data: []byte("x")},
			},
			wantErr: "does not parse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildPlanFS(tt.fs, o)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestGenerate(t *testing.T) {
	p, err := BuildPlan(Options{Name: "api", Framework: "gin", Database: "mysql"})
	if err != nil {
		t.Fatal(err)
	}
	p.Steps = nil // keep the unit test offline; e2e_test.go covers the steps

	dir := filepath.Join(t.TempDir(), "api")
	var written []string
	err = Generate(context.Background(), p, dir, func(e Event) {
		if e.Kind == FileWritten {
			written = append(written, e.Path)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != len(p.Files) {
		t.Errorf("got %d FileWritten events, want %d", len(written), len(p.Files))
	}
	for _, f := range p.Files {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f.Path)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, f.Content) {
			t.Errorf("%s: content differs from plan", f.Path)
		}
	}

	// A second run into the same directory must refuse to overwrite.
	if err := Generate(context.Background(), p, dir, nil); !errors.Is(err, ErrDirNotEmpty) {
		t.Errorf("second Generate: err = %v, want ErrDirNotEmpty", err)
	}
}

func TestGenerateStepFailure(t *testing.T) {
	p := Plan{Steps: []Step{{Name: "fail", Args: []string{"go", "no-such-subcommand"}}}}
	err := Generate(context.Background(), p, t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), "fail") {
		t.Fatalf("err = %v, want step failure", err)
	}
}

// TestStdlibOnly keeps core free of UI dependencies: every non-test file may
// import only standard library packages.
func TestStdlibOnly(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			if first, _, _ := strings.Cut(path, "/"); strings.Contains(first, ".") {
				t.Errorf("%s imports %s; core must use only the standard library", name, path)
			}
		}
	}
}
