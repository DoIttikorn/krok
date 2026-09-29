package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DoIttikorn/krok/internal/core"
	"github.com/DoIttikorn/krok/internal/tui"
)

// offline generates like core.Generate but skips the plan's steps (go mod
// tidy, git init), so tests need no network.
func offline(ctx context.Context, p core.Plan, dir string, on func(core.Event)) error {
	p.Steps = nil
	return core.Generate(ctx, p, dir, on)
}

// fakePrompt answers questions without a terminal and records what it was
// asked to do.
type fakePrompt struct {
	answer     func(o *core.Options) // fills in what Ask was asked for
	askErr     error
	confirm    bool
	confirmErr error

	asked     *tui.Fields // nil until Ask is called
	confirmed bool        // Confirm was called
	spun      bool        // Spin was called
}

func (f *fakePrompt) Ask(_ context.Context, o *core.Options, fields tui.Fields) error {
	f.asked = &fields
	if f.askErr != nil {
		return f.askErr
	}
	if f.answer != nil {
		f.answer(o)
	}
	return nil
}

func (f *fakePrompt) Confirm(context.Context) (bool, error) {
	f.confirmed = true
	return f.confirm, f.confirmErr
}

func (f *fakePrompt) Spin(ctx context.Context, _ string, action func(context.Context) error) error {
	f.spun = true
	return action(ctx)
}

// answerDefaults fills in whatever is still missing.
func answerDefaults(o *core.Options) {
	if o.Name == "" {
		o.Name = "my-api"
	}
	if o.Framework == "" {
		o.Framework = core.FrameworkChi
	}
	if o.Database == "" {
		o.Database = core.DatabaseNone
	}
}

// run executes krok with args in a fresh working directory and returns its
// output without ANSI styling.
func run(t *testing.T, d deps, args ...string) (string, error) {
	t.Helper()
	t.Chdir(t.TempDir())
	if d.generate == nil {
		d.generate = offline
	}
	if d.prompt == nil {
		d.prompt = &fakePrompt{}
	}
	root := newRootCmd(d)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return ansi.Strip(out.String()), err
}

func exists(path string) bool {
	_, err := os.Stat(filepath.FromSlash(path))
	return err == nil
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNewFromFlags(t *testing.T) {
	out, err := run(t, deps{}, "new", "my-api",
		"-f", "chi", "-d", "postgres", "-m", "github.com/example/my-api",
		"--asynq", "--log", "zap", "--key-vault", "--git=false")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}

	if got := read(t, "my-api/go.mod"); !strings.Contains(got, "module github.com/example/my-api") {
		t.Errorf("go.mod = %q, want the module path from -m", got)
	}
	for path, want := range map[string]string{
		"my-api/internal/logger/handler.go":            "zapslog",              // --log zap
		"my-api/.env":                                  "AZURE_KEY_VAULT_NAME", // --key-vault
		"my-api/internal/tasks/tasks.go":               "asynq",                // --asynq
		"my-api/internal/redis/redis.go":               "go-redis",             // asynq brings redis
		"my-api/internal/items/postgres/repository.go": "package postgres",     // -d postgres
	} {
		if got := read(t, path); !strings.Contains(got, want) {
			t.Errorf("%s does not contain %q", path, want)
		}
	}

	// Non-interactive runs print plain progress and the next steps.
	for _, want := range []string{"write go.mod", "created my-api", "az login", "make deps-up", "make run-worker"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	out, err := run(t, deps{}, "new", "my-api", "-f", "gin", "-d", "none", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	if exists("my-api") {
		t.Error("--dry-run created the project directory")
	}
	for _, want := range []string{"Dry run", "go mod tidy", "git init", "go.mod", "Gin"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestMissingFlagsWithoutTerminal(t *testing.T) {
	tests := []struct {
		args        []string
		want, avoid []string
	}{
		{[]string{"new"}, []string{"<name>", "--framework", "--database"}, nil},
		{[]string{"new", "my-api", "-f", "chi"}, []string{"--database"}, []string{"<name>", "--framework"}},
	}
	for _, tt := range tests {
		_, err := run(t, deps{terminal: false}, tt.args...)
		if err == nil {
			t.Fatalf("%v: want an error without a terminal", tt.args)
		}
		for _, w := range tt.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%v: error %q does not mention %s", tt.args, err, w)
			}
		}
		for _, a := range tt.avoid {
			if strings.Contains(err.Error(), a) {
				t.Errorf("%v: error %q mentions %s, which was given", tt.args, err, a)
			}
		}
	}
}

func TestInvalidOptions(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"new", "x", "-f", "fiber", "-d", "none"}, `unknown framework "fiber"`},
		{[]string{"new", "x", "-f", "chi", "-d", "sqlite"}, `unknown database "sqlite"`},
		{[]string{"new", "x", "-f", "chi", "-d", "none", "--log", "logrus"}, `unknown logger "logrus"`},
		{[]string{"new", "x", "-f", "chi", "-d", "mysql", "--river"}, "river stores jobs in PostgreSQL"},
		{[]string{"new", "x", "-f", "chi", "-d", "none", "--watermill"}, "watermill needs a broker"},
		{[]string{"new", "my api", "-f", "chi", "-d", "none"}, "invalid project name"},
		{[]string{"new", "a", "b"}, "accepts at most 1 arg"},
	}
	for _, tt := range tests {
		_, err := run(t, deps{}, tt.args...)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%v: err = %v, want containing %q", tt.args, err, tt.want)
		}
	}
}

func TestTargetDirectoryNotEmpty(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("my-api", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("my-api/keep.txt", []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := newRootCmd(deps{prompt: &fakePrompt{}, generate: offline})
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"new", "my-api", "-f", "chi", "-d", "none"})
	if err := root.ExecuteContext(context.Background()); !errors.Is(err, core.ErrDirNotEmpty) {
		t.Fatalf("err = %v, want ErrDirNotEmpty", err)
	}
	if got := read(t, "my-api/keep.txt"); got != "mine" {
		t.Errorf("existing file changed to %q", got)
	}
}

// With a terminal, krok asks only for what the flags left out. The logger and
// the extras are only offered while it is asking anyway.
func TestInteractiveAsksOnlyWhatIsMissing(t *testing.T) {
	all := tui.Fields{Name: true, Module: true, Framework: true, Database: true, Logger: true, Features: true}
	tests := []struct {
		name string
		args []string
		want *tui.Fields // nil: no questions at all
	}{
		{"nothing given", []string{"new"}, &all},
		{"name given", []string{"new", "my-api"},
			&tui.Fields{Framework: true, Database: true, Logger: true, Features: true}},
		{"module given", []string{"new", "-m", "github.com/example/x"},
			&tui.Fields{Name: true, Framework: true, Database: true, Logger: true, Features: true}},
		{"extras and logger given", []string{"new", "my-api", "-d", "none", "--redis", "--log", "zap"},
			&tui.Fields{Framework: true}},
		{"everything required given", []string{"new", "my-api", "-f", "chi", "-d", "none"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &fakePrompt{answer: answerDefaults, confirm: true}
			out, err := run(t, deps{terminal: true, prompt: p}, append(tt.args, "--git=false")...)
			if err != nil {
				t.Fatalf("err = %v\n%s", err, out)
			}
			switch {
			case tt.want == nil && p.asked != nil:
				t.Errorf("asked %+v, want no questions", *p.asked)
			case tt.want != nil && p.asked == nil:
				t.Errorf("asked nothing, want %+v", *tt.want)
			case tt.want != nil && *p.asked != *tt.want:
				t.Errorf("asked %+v\nwant  %+v", *p.asked, *tt.want)
			}
			if !exists("my-api/go.mod") {
				t.Error("project was not generated")
			}
		})
	}
}

func TestInteractiveConfirmation(t *testing.T) {
	t.Run("declined", func(t *testing.T) {
		p := &fakePrompt{confirm: false}
		out, err := run(t, deps{terminal: true, prompt: p}, "new", "my-api", "-f", "chi", "-d", "none")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "Cancelled.") || exists("my-api") || p.spun {
			t.Errorf("declining should cancel without writing; spun=%v output:\n%s", p.spun, out)
		}
	})
	t.Run("accepted", func(t *testing.T) {
		p := &fakePrompt{confirm: true}
		out, err := run(t, deps{terminal: true, prompt: p}, "new", "my-api", "-f", "chi", "-d", "none", "--git=false")
		if err != nil {
			t.Fatal(err)
		}
		if !p.spun || !exists("my-api/go.mod") {
			t.Errorf("accepting should generate behind the spinner; spun=%v", p.spun)
		}
		for _, want := range []string{"Created my-api", "Next steps", "make watch"} {
			if !strings.Contains(out, want) {
				t.Errorf("output lacks %q:\n%s", want, out)
			}
		}
	})
	t.Run("--yes skips it", func(t *testing.T) {
		p := &fakePrompt{}
		if _, err := run(t, deps{terminal: true, prompt: p}, "new", "my-api", "-f", "chi", "-d", "none", "--yes", "--git=false"); err != nil {
			t.Fatal(err)
		}
		if p.confirmed || !exists("my-api/go.mod") {
			t.Errorf("confirmed=%v, want no confirmation and a generated project", p.confirmed)
		}
	})
}

func TestInteractiveCancelIsNotAnError(t *testing.T) {
	for _, cause := range []error{huh.ErrUserAborted, context.Canceled} {
		p := &fakePrompt{askErr: cause}
		out, err := run(t, deps{terminal: true, prompt: p}, "new")
		if err != nil {
			t.Errorf("%v: err = %v, want a clean exit", cause, err)
		}
		if !strings.Contains(out, "Cancelled.") {
			t.Errorf("%v: output lacks Cancelled.:\n%s", cause, out)
		}
	}
}

func TestGenerateErrorIsReported(t *testing.T) {
	boom := errors.New("disk full")
	failing := func(context.Context, core.Plan, string, func(core.Event)) error { return boom }
	_, err := run(t, deps{generate: failing}, "new", "my-api", "-f", "chi", "-d", "none")
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "generate my-api") {
		t.Errorf("err = %v, want it wrapped with the directory", err)
	}
}

// Every catalog entry is reachable from the command line: a flag per extra,
// and shell completion for the choice flags.
func TestFlagsFollowTheCatalog(t *testing.T) {
	cat := core.DefaultCatalog()

	help, err := run(t, deps{}, "new", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cat.Features {
		if !strings.Contains(help, "--"+c.ID) {
			t.Errorf("help lacks --%s", c.ID)
		}
	}

	for flag, choices := range map[string][]core.Choice{
		"--framework": cat.Frameworks,
		"--database":  cat.Databases,
		"--log":       cat.Loggers,
	} {
		out, err := run(t, deps{}, "__complete", "new", flag, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range choices {
			if !strings.Contains(out, c.ID+"\t") {
				t.Errorf("completion of %s lacks %q:\n%s", flag, c.ID, out)
			}
		}
	}
}
