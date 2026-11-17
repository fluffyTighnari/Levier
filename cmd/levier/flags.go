package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

// parseFlagSet parses a subcommand flag set.
//
// --help is a successful command: usage goes to stdout and the exit code is 0.
// Invalid flags remain usage errors: diagnostics and usage go to stderr, exit 2.
func parseFlagSet(flags *flag.FlagSet, args []string, stdout, stderr io.Writer) (bool, int) {
	flags.SetOutput(stderr)
	flags.Usage = func() {}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printFlagUsage(stdout, flags)
			return false, 0
		}
		printFlagUsage(stderr, flags)
		return false, 2
	}
	return true, 0
}

func printFlagUsage(out io.Writer, flags *flag.FlagSet) {
	fmt.Fprintf(out, "usage: levier %s [flags]\n\n", flags.Name())
	previous := flags.Output()
	flags.SetOutput(out)
	flags.PrintDefaults()
	flags.SetOutput(previous)
}
