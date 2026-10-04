// Command app is the single entry point of the boilerplate: it wires the
// command line to the application layer and maps errors onto exit codes.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/codingseeker/go-starter/internal/buildinfo"
	"github.com/codingseeker/go-starter/internal/cli"
)

func main() {
	os.Exit(run(context.Background(), os.Args, os.Stdout, os.Stderr))
}

// run keeps main testable by isolating every side effect behind parameters.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	command := cli.New(args, buildinfo.Version)
	command.Out = stdout
	command.Err = stderr

	if err := command.Run(ctx); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", buildinfo.Version, err)
		if cli.ExitCode(err) == cli.ExitUsage {
			return cli.ExitUsage
		}
		return cli.ExitError
	}
	return cli.ExitOK
}
