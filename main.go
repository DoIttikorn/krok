// Command krok scaffolds Go API projects.
//
//	go install github.com/DoIttikorn/krok@latest
//	krok new my-api --framework chi --database postgres
package main

import (
	"context"
	"os"

	"github.com/DoIttikorn/krok/internal/cli"
)

func main() {
	if err := cli.Execute(context.Background()); err != nil {
		os.Exit(1)
	}
}
