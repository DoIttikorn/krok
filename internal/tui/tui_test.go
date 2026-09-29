package tui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/DoIttikorn/krok/internal/core"
)

// lineReader hands out one answer per Read. Huh's accessible prompts wrap the
// input in a new bufio.Scanner for every question, and a reader that returned
// everything at once would let the first question swallow the rest.
type lineReader struct{ lines []string }

func (r *lineReader) Read(p []byte) (int, error) {
	if len(r.lines) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.lines[0]+"\n")
	r.lines = r.lines[1:]
	return n, nil
}

// script runs fn against a Prompter fed with answers, one per line, and
// returns what it printed without styling. A prompt that keeps waiting after
// the answers run out fails the test instead of hanging it.
func script(t *testing.T, fn func(Prompter) error, answers ...string) string {
	t.Helper()
	var out bytes.Buffer
	p := Prompter{In: &lineReader{lines: answers}, Out: &out}
	done := make(chan error, 1)
	go func() { done <- fn(p) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("err = %v\n%s", err, ansi.Strip(out.String()))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("prompt still waiting after the scripted answers ran out")
	}
	return ansi.Strip(out.String())
}

func TestAskWholeWizard(t *testing.T) {
	var o core.Options
	all := Fields{Name: true, Module: true, Framework: true, Database: true, Logger: true, Features: true}
	script(t, func(p Prompter) error { return p.Ask(context.Background(), &o, all) },
		"my-api", // project name
		"",       // module path: empty keeps the project name
		"3",      // framework: gin, echo, chi
		"1",      // database: postgres, mysql, mongodb, none
		"2",      // logger: slog, zap, zerolog, charm
		"6", "0", // extras: toggle river, then done
	)
	want := core.Options{Name: "my-api", Framework: "chi", Database: "postgres", Logger: "zap", Features: []string{"river"}}
	if !reflect.DeepEqual(o, want) {
		t.Errorf("got  %+v\nwant %+v", o, want)
	}
}

func TestAskSkipsFieldsNotRequested(t *testing.T) {
	o := core.Options{Name: "given", Framework: "gin"}
	out := script(t, func(p Prompter) error {
		return p.Ask(context.Background(), &o, Fields{Database: true})
	}, "4")
	if o.Database != "none" || o.Name != "given" || o.Framework != "gin" {
		t.Errorf("got %+v, want only the database changed", o)
	}
	if strings.Contains(out, "Project name") || strings.Contains(out, "Framework") {
		t.Errorf("asked for fields that were not requested:\n%s", out)
	}
}

func TestAskRejectsAnInvalidName(t *testing.T) {
	var o core.Options
	out := script(t, func(p Prompter) error {
		return p.Ask(context.Background(), &o, Fields{Name: true})
	}, "my api", "my-api")
	if o.Name != "my-api" {
		t.Errorf("Name = %q, want the second, valid answer", o.Name)
	}
	if !strings.Contains(out, "invalid project name") {
		t.Errorf("no validation message for %q:\n%s", "my api", out)
	}
}

// The extras are validated against the database chosen earlier, so a rule
// violation is caught in the form rather than after it.
func TestAskValidatesExtrasAgainstTheDatabase(t *testing.T) {
	o := core.Options{Database: "mysql"}
	out := script(t, func(p Prompter) error {
		return p.Ask(context.Background(), &o, Fields{Features: true})
	},
		"6", "0", // river on MySQL: rejected
		"6", "0", // toggle it off again: accepted
	)
	if !strings.Contains(out, "river stores jobs in PostgreSQL") {
		t.Errorf("no validation message:\n%s", out)
	}
	if len(o.Features) != 0 {
		t.Errorf("Features = %v, want none", o.Features)
	}
}

func TestAskDefaultsTheLoggerToSlog(t *testing.T) {
	var o core.Options
	script(t, func(p Prompter) error {
		return p.Ask(context.Background(), &o, Fields{Logger: true})
	}, "") // just enter
	if o.Logger != core.LoggerSlog {
		t.Errorf("Logger = %q, want %q by default", o.Logger, core.LoggerSlog)
	}
}

func TestConfirm(t *testing.T) {
	for answer, want := range map[string]bool{"y": true, "n": false, "": true} {
		var got bool
		script(t, func(p Prompter) error {
			var err error
			got, err = p.Confirm(context.Background())
			return err
		}, answer)
		if got != want {
			t.Errorf("answer %q: Confirm = %v, want %v", answer, got, want)
		}
	}
}

func TestSpinRunsTheAction(t *testing.T) {
	boom := errors.New("boom")
	ran := false
	err := Prompter{Out: io.Discard}.Spin(context.Background(), "working", func(context.Context) error {
		ran = true
		return boom
	})
	if !ran || !errors.Is(err, boom) {
		t.Errorf("ran=%v err=%v, want the action run and its error returned", ran, err)
	}
}

func TestTreeListsDirectoriesFirst(t *testing.T) {
	p := core.Plan{
		Options: core.Options{Name: "app"},
		Files:   []core.File{{Path: "b.txt"}, {Path: "a/x.go"}, {Path: "a/b/y.go"}, {Path: ".env"}},
	}
	out := ansi.Strip(Tree(p))
	order := []string{"app/", "a/", "b/", "y.go", "x.go", ".env", "b.txt"}
	last := -1
	for _, name := range order {
		i := strings.Index(out, name)
		if i <= last {
			t.Fatalf("%q is out of order in:\n%s", name, out)
		}
		last = i
	}
}

func plan(t *testing.T, o core.Options) core.Plan {
	t.Helper()
	p, err := core.BuildPlan(o)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNextSteps(t *testing.T) {
	simple := plan(t, core.Options{Name: "x", Framework: "chi", Database: "none"})
	if got, want := NextSteps(simple, "x"), []string{"cd x", "make watch"}; !slices.Equal(got, want) {
		t.Errorf("no database: %v, want %v", got, want)
	}

	// Brokers need deps-up too, even without a database.
	redisOnly := plan(t, core.Options{Name: "x", Framework: "chi", Database: "none", Features: []string{"redis"}})
	if got, want := NextSteps(redisOnly, "x"), []string{"cd x", "make deps-up", "make watch"}; !slices.Equal(got, want) {
		t.Errorf("redis, no database: %v, want %v", got, want)
	}

	full := plan(t, core.Options{Name: "x", Framework: "chi", Database: "postgres", Config: "keyvault", Features: []string{"kafka"}})
	want := []string{"cd x", "az login", "make deps-up", "make watch", "make run-worker"}
	if got := NextSteps(full, "x"); !slices.Equal(got, want) {
		t.Errorf("postgres, Key Vault, Kafka: %v, want %v", got, want)
	}
}

func TestSummary(t *testing.T) {
	p := plan(t, core.Options{Name: "x", Framework: "chi", Database: "postgres", Logger: "zap", Features: []string{"river", "redis"}})
	out := ansi.Strip(Summary(p))
	for _, want := range []string{"x", "Chi", "PostgreSQL", ".env file", "zap", "Redis, River"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
}

func TestDone(t *testing.T) {
	p := plan(t, core.Options{Name: "x", Framework: "chi", Database: "none", Config: "keyvault"})
	events := []core.Event{
		{Kind: core.FileWritten, Path: "go.mod"},
		{Kind: core.FileWritten, Path: "main.go"},
		{Kind: core.StepStarted, Step: core.Step{Name: "go mod tidy"}},
		{Kind: core.StepDone, Step: core.Step{Name: "go mod tidy"}},
	}
	out := ansi.Strip(Done(p, "x", events))
	for _, want := range []string{"wrote 2 files", "go mod tidy", "Created x", "AZURE_KEY_VAULT_NAME", "az login"} {
		if !strings.Contains(out, want) {
			t.Errorf("Done lacks %q:\n%s", want, out)
		}
	}
}
