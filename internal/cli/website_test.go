package cli

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/DoIttikorn/krok/internal/core"
)

// The website's command builder (website/src/scripts/catalog.ts) offers every
// choice the catalog has, and only flags `krok new` accepts. It is a hand-kept
// copy in TypeScript, so this is what notices when a new framework, database,
// logger, config or extra reaches the CLI but not the website.
func TestWebsiteFollowsTheCatalog(t *testing.T) {
	src, err := os.ReadFile("../../website/src/scripts/catalog.ts")
	if err != nil {
		t.Fatal(err)
	}
	ts := string(src)
	cat := core.DefaultCatalog()

	list := func(name string) []string {
		m := regexp.MustCompile(`export const ` + name + `: \w+\[\] = \[([^\]]*)\];`).FindStringSubmatch(ts)
		if m == nil {
			t.Fatalf("catalog.ts has no %s list", name)
		}
		var ids []string
		for _, q := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(m[1], -1) {
			ids = append(ids, q[1])
		}
		return ids
	}
	same := func(what string, got, want []string) {
		t.Helper()
		g, w := slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))
		if !slices.Equal(g, w) {
			t.Errorf("website %s = %v, catalog has %v", what, g, w)
		}
	}

	same("frameworks", list("frameworks"), core.IDs(cat.Frameworks))
	same("databases", list("databases"), core.IDs(cat.Databases))
	loggers := list("loggers")
	same("loggers", loggers, core.IDs(cat.Loggers))
	if len(loggers) > 0 && loggers[0] != core.LoggerSlog {
		t.Errorf("website lists %q first, but the command leaves --log out for the first logger and the CLI default is %q", loggers[0], core.LoggerSlog)
	}

	// Extras are the features plus every config other than the default .env.
	var extras []string
	flags := map[string]string{}
	for _, m := range regexp.MustCompile(`\{ id: "([^"]+)", flag: "([^"]+)"`).FindAllStringSubmatch(ts, -1) {
		extras = append(extras, m[1])
		flags[m[1]] = m[2]
	}
	want := core.IDs(cat.Features)
	for _, id := range core.IDs(cat.Configs) {
		if id != core.ConfigEnv {
			want = append(want, id)
		}
	}
	same("extras", extras, want)

	cmd := newCmd(deps{})
	for id, flag := range flags {
		if cmd.Flags().Lookup(strings.TrimPrefix(flag, "--")) == nil {
			t.Errorf("website extra %q uses %s, which krok new does not accept", id, flag)
		}
	}
	for _, name := range []string{"framework", "database", "log"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("the website's command uses --%s, which krok new does not accept", name)
		}
	}
	for _, short := range []string{"f", "d"} {
		if cmd.Flags().ShorthandLookup(short) == nil {
			t.Errorf("the website's command uses -%s, which krok new does not accept", short)
		}
	}
}
