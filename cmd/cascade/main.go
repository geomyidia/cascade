// Command cascade computes the reverse-transitive closure of a Go change-set
// under the imports relation. See https://github.com/geomyidia/cascade.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/geomyidia/cascade/internal/project"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is the entry point factored out of main so it can be exercised
// in-process by tests. It returns the desired process exit code rather
// than calling os.Exit directly.
//
// Exit-code contract:
//
//	0  --version (prints metadata to stdout) or --help
//	1  flag parse error other than flag.ErrHelp
//	2  no flags / unknown positional args (M1 placeholder; M5 will
//	   replace this with the real pipeline)
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("cascade", flag.ContinueOnError)
	fs.SetOutput(stderr)

	showVersion := fs.Bool("version", false, "print version information and exit")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if *showVersion {
		fmt.Fprintf(stdout, "cascade %s (build %s)\n",
			project.VersionString(), project.BuildString())
		return 0
	}

	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "cascade: unexpected argument %q\n", fs.Arg(0))
		return 2
	}

	fmt.Fprintln(stderr, "cascade: not yet implemented (this is the M1 placeholder)")
	return 2
}
