package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
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

// splitArgs lets flags appear before or after positional arguments, which is
// how people type `levier run "hello" --runtime echo1`.
func splitArgs(args []string, valueFlags map[string]bool) ([]string, []string) {
	var flags []string
	var positionals []string
	for index := 0; index < len(args); index++ {
		item := args[index]
		if !strings.HasPrefix(item, "-") {
			positionals = append(positionals, item)
			continue
		}
		flags = append(flags, item)
		name := strings.TrimLeft(item, "-")
		if equals := strings.Index(name, "="); equals >= 0 {
			name = name[:equals]
		}
		if valueFlags[name] && !strings.Contains(item, "=") && index+1 < len(args) {
			index++
			flags = append(flags, args[index])
		}
	}
	return flags, positionals
}

func printFlagUsage(out io.Writer, flags *flag.FlagSet) {
	fmt.Fprintf(out, "usage: levier %s [flags]\n\n", flags.Name())
	previous := flags.Output()
	flags.SetOutput(out)
	flags.PrintDefaults()
	flags.SetOutput(previous)
}
