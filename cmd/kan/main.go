// Command kan checks Kanon documents against their schema.
package main

import (
	"os"

	"github.com/func-it/kanon/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
