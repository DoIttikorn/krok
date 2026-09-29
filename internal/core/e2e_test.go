package core

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E generates every combination with its real steps (go mod tidy,
// git init), then vets and tests the result, and validates docker-compose.yml
// and the Kustomize manifests when Docker Compose and kubectl are installed. It needs network access to download
// dependencies, so it only runs when KROK_E2E=1:
//
//	KROK_E2E=1 go test ./internal/core -run E2E
//
// With KROK_E2E_DOCKER=1 as well, one project per database also starts that
// database with Docker Compose and runs the items repository contract
// against it (INTEGRATION=1), so every real adapter is exercised.
func TestE2E(t *testing.T) {
	if os.Getenv("KROK_E2E") != "1" {
		t.Skip("set KROK_E2E=1 to generate and build every combination")
	}
	compose := exec.Command("docker", "compose", "version").Run() == nil
	withDocker := os.Getenv("KROK_E2E_DOCKER") == "1"
	_, kubectlErr := exec.LookPath("kubectl")
	for _, o := range combos() {
		t.Run(comboName(o), func(t *testing.T) {
			t.Parallel()
			o.GoVersion = "" // use the running toolchain
			p, err := BuildPlan(o)
			if err != nil {
				t.Fatal(err)
			}
			dir := filepath.Join(t.TempDir(), o.Name)
			if err := Generate(context.Background(), p, dir, nil); err != nil {
				t.Fatal(err)
			}
			cmds := [][]string{
				{"go", "vet", "./..."},
				{"go", "test", "./..."},
			}
			if compose {
				cmds = append(cmds, []string{"docker", "compose", "config", "--quiet"})
			}
			if kubectlErr == nil {
				cmds = append(cmds, []string{"kubectl", "kustomize", "deploy/k8s"})
			}
			for _, args := range cmds {
				cmd := exec.Command(args[0], args[1:]...)
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
			}
			if withDocker && o.Framework == "chi" && o.Database != DatabaseNone && o.Config == ConfigEnv && len(o.Features) == 0 {
				integrationTest(t, dir, comboName(o))
			}
		})
	}
}

// integrationTest starts the project's database with Docker Compose on a free
// host port and runs the items repository contract against it.
func integrationTest(t *testing.T, dir, name string) {
	port := freePort(t)
	env := append(os.Environ(),
		"COMPOSE_PROJECT_NAME=krok-e2e-"+strings.ToLower(name), // parallel projects must not collide
		fmt.Sprintf("DB_PORT=%d", port),
		"INTEGRATION=1",
	)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	t.Cleanup(func() {
		cmd := exec.Command("docker", "compose", "down", "-v")
		cmd.Dir, cmd.Env = dir, env
		_ = cmd.Run()
	})
	run("docker", "compose", "up", "-d", "--wait", "db")
	out := run("go", "test", "-count=1", "-v", "-run", "TestContract", "./internal/items/...")
	// A skipped contract would pass silently, so make sure it really ran.
	if strings.Contains(out, "--- SKIP: TestContract") || strings.Count(out, "--- PASS: TestContract ") < 2 {
		t.Fatalf("the contract did not run for both the memory and the database adapter:\n%s", out)
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
