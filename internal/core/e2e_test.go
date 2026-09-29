package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestE2E generates every combination with its real steps (go mod tidy,
// git init), then vets and tests the result, and validates docker-compose.yml
// and the Kustomize manifests when Docker Compose and kubectl are installed. It needs network access to download
// dependencies, so it only runs when KROK_E2E=1:
//
//	KROK_E2E=1 go test ./internal/core -run E2E
func TestE2E(t *testing.T) {
	if os.Getenv("KROK_E2E") != "1" {
		t.Skip("set KROK_E2E=1 to generate and build every combination")
	}
	compose := exec.Command("docker", "compose", "version").Run() == nil
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
		})
	}
}
