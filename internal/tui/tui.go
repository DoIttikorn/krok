// Package tui is krok's interactive front end: Huh forms to collect options,
// Lip Gloss to preview the plan, and a spinner while core generates files.
// All project logic lives in core; this package only asks and shows.
package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"

	"github.com/DoIttikorn/krok/core"
)

// Fields selects which options Ask should prompt for. Fields already given
// on the command line are skipped.
type Fields struct {
	Name, Module, Framework, Database, Features bool
}

// Any reports whether at least one field needs asking.
func (f Fields) Any() bool { return f.Name || f.Module || f.Framework || f.Database || f.Features }

// ANSI 16-color palette so the output follows the user's terminal theme.
var (
	dirSt   = lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true)
	faint   = lipgloss.NewStyle().Faint(true)
	bold    = lipgloss.NewStyle().Bold(true)
	success = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	code    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

func newForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).
		WithTheme(huh.ThemeFunc(huh.ThemeCharm)).
		WithAccessible(accessible())
}

// accessible enables Huh's screen-reader friendly prompts when ACCESSIBLE is set.
func accessible() bool { return os.Getenv("ACCESSIBLE") != "" }

// Ask prompts for the selected fields and stores answers in o. It returns
// huh.ErrUserAborted if the user cancels.
func Ask(ctx context.Context, o *core.Options, f Fields) error {
	if !f.Any() {
		return nil
	}
	cat := core.DefaultCatalog()

	var project, stack []huh.Field
	if f.Name {
		project = append(project, huh.NewInput().
			Title("Project name").
			Description("Directory and binary name.").
			Placeholder("my-api").
			Validate(core.ValidateName).
			Value(&o.Name))
	}
	if f.Module {
		project = append(project, huh.NewInput().
			Title("Module path").
			Description("Leave empty to use the project name.").
			PlaceholderFunc(func() string {
				if o.Name == "" {
					return "github.com/you/my-api"
				}
				return o.Name
			}, &o.Name).
			Value(&o.ModulePath))
	}
	if f.Framework {
		stack = append(stack, huh.NewSelect[string]().
			Title("Framework").
			Options(options(cat.Frameworks)...).
			Value(&o.Framework))
	}
	if f.Database {
		stack = append(stack, huh.NewSelect[string]().
			Title("Database").
			Options(options(cat.Databases)...).
			Value(&o.Database))
	}

	var extras []huh.Field
	if f.Features {
		extras = append(extras, huh.NewMultiSelect[string]().
			Title("Extras").
			Description("Optional: toggle with x or space, enter to continue.").
			Options(options(cat.Features)...).
			Validate(func(sel []string) error { return core.ValidateFeatures(o.Database, sel) }).
			Value(&o.Features))
	}

	var groups []*huh.Group
	for _, fields := range [][]huh.Field{project, stack, extras} {
		if len(fields) > 0 {
			groups = append(groups, huh.NewGroup(fields...))
		}
	}
	return newForm(groups...).RunWithContext(ctx)
}

func options(choices []core.Choice) []huh.Option[string] {
	width := 0
	for _, c := range choices {
		width = max(width, len(c.Name))
	}
	opts := make([]huh.Option[string], len(choices))
	for i, c := range choices {
		opts[i] = huh.NewOption(fmt.Sprintf("%-*s  %s", width, c.Name, c.Description), c.ID)
	}
	return opts
}

// Confirm asks whether to create the project. It returns false if the user
// declines or cancels.
func Confirm(ctx context.Context) (bool, error) {
	ok := true
	err := newForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Create project?").
			Affirmative("Create").
			Negative("Cancel").
			Value(&ok),
	)).RunWithContext(ctx)
	return ok, err
}

// Generate runs core.Generate behind a spinner and returns the events it
// produced so the caller can summarize them.
func Generate(ctx context.Context, p core.Plan, dir string) ([]core.Event, error) {
	var events []core.Event
	err := spinner.New().
		Title(fmt.Sprintf(" Creating %s (writing files, %s)…", p.Options.Name, stepNames(p))).
		Context(ctx).
		WithAccessible(accessible()).
		ActionWithErr(func(ctx context.Context) error {
			// events is only read after Run returns, so no locking is needed.
			return core.Generate(ctx, p, dir, func(e core.Event) { events = append(events, e) })
		}).
		Run()
	return events, err
}

func stepNames(p core.Plan) string {
	names := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}

// Summary renders the chosen options as aligned label/value lines.
func Summary(p core.Plan) string {
	cat := core.DefaultCatalog()
	o := p.Options
	git := "no"
	if o.Git {
		git = "yes"
	}
	rows := [][2]string{
		{"Name", o.Name},
		{"Module", o.ModulePath},
		{"Framework", choiceName(cat.Frameworks, o.Framework)},
		{"Database", choiceName(cat.Databases, o.Database)},
		{"Config", choiceName(cat.Configs, o.Config)},
		{"Extras", extras(cat, o.Features)},
		{"Go", o.GoVersion},
		{"Git", git},
	}
	var b strings.Builder
	for _, r := range rows {
		fmt.Fprintf(&b, "%s %s\n", faint.Render(fmt.Sprintf("%-10s", r[0])), bold.Render(r[1]))
	}
	return strings.TrimRight(b.String(), "\n")
}

func extras(cat core.Catalog, ids []string) string {
	if len(ids) == 0 {
		return "none"
	}
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = choiceName(cat.Features, id)
	}
	return strings.Join(names, ", ")
}

func choiceName(choices []core.Choice, id string) string {
	for _, c := range choices {
		if c.ID == id {
			return c.Name
		}
	}
	return id
}

// Tree renders the files in p as a directory tree.
func Tree(p core.Plan) string {
	paths := make([]string, len(p.Files))
	for i, f := range p.Files {
		paths[i] = f.Path
	}
	t := newTree(p.Options.Name)
	addPaths(t, paths)
	return t.String()
}

func newTree(dir string) *tree.Tree {
	return tree.Root(dirSt.Render(dir + "/")).
		Enumerator(tree.RoundedEnumerator).
		EnumeratorStyle(faint.PaddingRight(1))
}

// addPaths adds slash-separated paths under t, directories first.
func addPaths(t *tree.Tree, paths []string) {
	children := map[string][]string{}
	var dirs, files []string
	for _, p := range paths {
		dir, rest, ok := strings.Cut(p, "/")
		if !ok {
			files = append(files, p)
			continue
		}
		if _, seen := children[dir]; !seen {
			dirs = append(dirs, dir)
		}
		children[dir] = append(children[dir], rest)
	}
	sort.Strings(dirs)
	sort.Strings(files)
	for _, d := range dirs {
		sub := newTree(d)
		addPaths(sub, children[d])
		t.Child(sub)
	}
	for _, f := range files {
		t.Child(f)
	}
}

// Done renders the checklist of what Generate did and the next commands.
func Done(p core.Plan, dir string, events []core.Event) string {
	files := 0
	var steps []string
	for _, e := range events {
		switch e.Kind {
		case core.FileWritten:
			files++
		case core.StepDone:
			steps = append(steps, e.Step.Name)
		}
	}
	var b strings.Builder
	check := success.Render("✓")
	fmt.Fprintf(&b, "%s wrote %d files\n", check, files)
	for _, s := range steps {
		fmt.Fprintf(&b, "%s %s\n", check, s)
	}
	fmt.Fprintf(&b, "\n%s %s\n", success.Render("Created"), bold.Render(dir))
	if p.Options.Config == core.ConfigKeyVault {
		fmt.Fprintf(&b, "\n%s set %s in %s\n", faint.Render("Before running:"), code.Render("AZURE_KEY_VAULT_NAME"), code.Render(dir+"/.env"))
	}
	b.WriteString("\n")
	b.WriteString(faint.Render("Next steps:") + "\n")
	for _, c := range NextSteps(p, dir) {
		b.WriteString("  " + code.Render(c) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// NextSteps lists the commands to run after generating.
func NextSteps(p core.Plan, dir string) []string {
	cmds := []string{"cd " + dir}
	if p.Options.Config == core.ConfigKeyVault {
		cmds = append(cmds, "az login")
	}
	d := core.NewTemplateData(p.Options)
	if len(d.Services) > 0 {
		cmds = append(cmds, "make deps-up")
	}
	cmds = append(cmds, "make watch")
	if d.HasWorker {
		cmds = append(cmds, "make run-worker")
	}
	return cmds
}
