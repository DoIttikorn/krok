package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EventKind identifies what an Event reports.
type EventKind int

const (
	FileWritten EventKind = iota // Path was written
	StepStarted                  // Step began running
	StepDone                     // Step finished successfully
)

// Event reports progress from Generate. UIs decide how to show it: a
// spinner, a Bubble Tea message, or a plain log line.
type Event struct {
	Kind EventKind
	Path string // FileWritten: path relative to the project directory
	Step Step   // StepStarted, StepDone
}

// ErrDirNotEmpty is returned when the target directory already has content.
var ErrDirNotEmpty = errors.New("target directory exists and is not empty")

// Generate writes p into dir and then runs its steps inside dir. dir must not
// exist or must be empty. on may be nil. If a step fails, the files written so
// far are left in place so the user can inspect them.
func Generate(ctx context.Context, p Plan, dir string, on func(Event)) error {
	if on == nil {
		on = func(Event) {}
	}
	if err := CheckDir(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	for _, f := range p.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, f.Content, 0o644); err != nil {
			return err
		}
		on(Event{Kind: FileWritten, Path: f.Path})
	}

	for _, s := range p.Steps {
		on(Event{Kind: StepStarted, Step: s})
		if err := run(ctx, dir, s); err != nil {
			return err
		}
		on(Event{Kind: StepDone, Step: s})
	}
	return nil
}

// CheckDir reports whether dir can be generated into: it must not exist or
// must be empty. It does not modify anything, so UIs can call it before
// asking the user to confirm.
func CheckDir(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case len(entries) > 0:
		return fmt.Errorf("%w: %s", ErrDirNotEmpty, dir)
	}
	return nil
}

func run(ctx context.Context, dir string, s Step) error {
	if len(s.Args) == 0 {
		return fmt.Errorf("step %q has no command", s.Name)
	}
	cmd := exec.CommandContext(ctx, s.Args[0], s.Args[1:]...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(out.String())
		if msg == "" {
			return fmt.Errorf("%s: %w", s.Name, err)
		}
		return fmt.Errorf("%s: %w\n%s", s.Name, err, msg)
	}
	return nil
}
