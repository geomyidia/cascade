// Command cascade computes the reverse-transitive closure of a Go change-set
// under the imports relation. See https://github.com/geomyidia/cascade.
//
// All orchestration lives in internal/cli; main is a single-line delegation
// so the cli.Run pipeline is testable in-process without subprocess overhead.
package main

import (
	"os"

	"github.com/geomyidia/cascade/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
