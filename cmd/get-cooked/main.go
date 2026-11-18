// Command get-cooked builds a recipe website from programmable YAML recipes.
package main

import (
	"os"

	"github.com/RashKash103/get-cooked/internal/cli"
)

func main() {
	os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr))
}
