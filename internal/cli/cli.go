// Package cli wires krok's commands with Cobra and Fang. It turns flags into
// core.Options, falls back to Huh prompts for anything missing when running
// in a terminal, and fails with a clear message when it can't prompt.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"charm.land/fang/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/DoIttikorn/krok/internal/core"
	"github.com/DoIttikorn/krok/internal/tui"
)

// Execute runs the krok command line.
func Execute(ctx context.Context) error {
	root := &cobra.Command{
		Use:   "krok",
		Short: "Scaffold Go API projects",
		Long:  "krok generates a ready-to-run Go API project with the framework and database you choose.",
	}
	root.AddCommand(newCmd())
	return fang.Execute(ctx, root, fang.WithNotifySignal(os.Interrupt, syscall.SIGTERM))
}

type newFlags struct {
	module, framework, database string
	git, keyVault, yes, dryRun  bool
	features                    map[string]*bool // one --<id> flag per catalog feature
}

func newCmd() *cobra.Command {
	cat := core.DefaultCatalog()
	f := newFlags{features: map[string]*bool{}}
	cmd := &cobra.Command{
		Use:   "new [name]",
		Short: "Create a new project",
		Long: "Create a new project in ./<name>.\n\n" +
			"Options not given as flags are asked interactively. " +
			"Without a terminal, every option must be passed as a flag.",
		Example: "  krok new\n" +
			"  krok new my-api --framework chi --database postgres\n" +
			"  krok new my-api -f echo -d mysql --key-vault\n" +
			"  krok new my-api -f chi -d postgres --redis --kafka --river\n" +
			"  krok new my-api -f gin -d none -m github.com/you/my-api --dry-run",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNew(cmd, args, f)
		},
	}

	fl := cmd.Flags()
	fl.StringVarP(&f.framework, "framework", "f", "", "web framework: "+strings.Join(core.IDs(cat.Frameworks), ", "))
	fl.StringVarP(&f.database, "database", "d", "", "database: "+strings.Join(core.IDs(cat.Databases), ", "))
	fl.StringVarP(&f.module, "module", "m", "", "Go module path (default: project name)")
	fl.BoolVar(&f.keyVault, "key-vault", false, "read settings from Azure Key Vault instead of .env")
	for _, c := range cat.Features {
		f.features[c.ID] = fl.Bool(c.ID, false, c.Description)
	}
	fl.BoolVar(&f.git, "git", true, "initialize a git repository")
	fl.BoolVarP(&f.yes, "yes", "y", false, "skip the confirmation prompt")
	fl.BoolVar(&f.dryRun, "dry-run", false, "show what would be generated without writing anything")

	complete := func(choices []core.Choice) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			out := make([]string, len(choices))
			for i, c := range choices {
				out[i] = c.ID + "\t" + c.Description
			}
			return out, cobra.ShellCompDirectiveNoFileComp
		}
	}
	_ = cmd.RegisterFlagCompletionFunc("framework", complete(cat.Frameworks))
	_ = cmd.RegisterFlagCompletionFunc("database", complete(cat.Databases))
	return cmd
}

func runNew(cmd *cobra.Command, args []string, f newFlags) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	o := core.Options{
		ModulePath: f.module,
		Framework:  f.framework,
		Database:   f.database,
		Git:        f.git,
	}
	if f.keyVault {
		o.Config = core.ConfigKeyVault
	}
	featureFlags := false
	for _, c := range core.DefaultCatalog().Features {
		if *f.features[c.ID] {
			o.Features = append(o.Features, c.ID)
		}
		featureFlags = featureFlags || cmd.Flags().Changed(c.ID)
	}
	if len(args) == 1 {
		o.Name = args[0]
	}

	ask := tui.Fields{
		Name:      o.Name == "",
		Module:    o.Name == "" && !cmd.Flags().Changed("module"),
		Framework: o.Framework == "",
		Database:  o.Database == "",
	}
	// Extras are optional, so only offer them while already asking questions.
	ask.Features = ask.Any() && !featureFlags
	interactive := isTerminal()
	if ask.Any() {
		if !interactive {
			return missingFlagsError(ask)
		}
		if err := tui.Ask(ctx, &o, ask); err != nil {
			return cancelled(out, err)
		}
	}

	p, err := core.BuildPlan(o)
	if err != nil {
		return err
	}
	dir := p.Options.Name
	if err := core.CheckDir(dir); err != nil {
		return err
	}

	lipgloss.Fprintln(out, tui.Summary(p))
	lipgloss.Fprintln(out)
	lipgloss.Fprintln(out, tui.Tree(p))
	lipgloss.Fprintln(out)

	if f.dryRun {
		fmt.Fprintln(out, "Dry run: nothing was written. Steps that would run:")
		for _, s := range p.Steps {
			fmt.Fprintln(out, "  "+strings.Join(s.Args, " "))
		}
		return nil
	}

	if interactive && !f.yes {
		ok, err := tui.Confirm(ctx)
		if err != nil {
			return cancelled(out, err)
		}
		if !ok {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
	}

	var events []core.Event
	if interactive {
		events, err = tui.Generate(ctx, p, dir)
	} else {
		err = core.Generate(ctx, p, dir, func(e core.Event) {
			events = append(events, e)
			logEvent(out, e)
		})
	}
	if err != nil {
		return fmt.Errorf("generate %s: %w", dir, err)
	}

	if interactive {
		lipgloss.Fprintln(out, tui.Done(p, dir, events))
	} else {
		fmt.Fprintf(out, "created %s\nnext steps:\n", dir)
		for _, c := range tui.NextSteps(p, dir) {
			fmt.Fprintln(out, "  "+c)
		}
	}
	return nil
}

// logEvent prints plain progress lines for non-interactive runs (CI, pipes).
func logEvent(w io.Writer, e core.Event) {
	switch e.Kind {
	case core.FileWritten:
		fmt.Fprintln(w, "write", e.Path)
	case core.StepStarted:
		fmt.Fprintln(w, "run  ", strings.Join(e.Step.Args, " "))
	}
}

func missingFlagsError(f tui.Fields) error {
	var missing []string
	if f.Name {
		missing = append(missing, "<name>")
	}
	if f.Framework {
		missing = append(missing, "--framework")
	}
	if f.Database {
		missing = append(missing, "--database")
	}
	return fmt.Errorf("no terminal for interactive prompts; pass %s", strings.Join(missing, ", "))
}

// cancelled turns a user abort (Esc / Ctrl+C in a form) into a clean exit.
func cancelled(w io.Writer, err error) error {
	if errors.Is(err, huh.ErrUserAborted) || errors.Is(err, context.Canceled) {
		fmt.Fprintln(w, "Cancelled.")
		return nil
	}
	return err
}

func isTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}
