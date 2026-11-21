package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runStack(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printStackUsage(stderr)
		return 2
	}
	switch args[0] {
	case "up":
		return runStackUp(args[1:], stdout, stderr)
	case "down":
		return runStackDown(args[1:], stdout, stderr)
	case "ps":
		return runStackPS(args[1:], stdout, stderr)
	case "logs":
		return runStackLogs(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printStackUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown stack command: %s\n\n", args[0])
		printStackUsage(stderr)
		return 2
	}
}

func printStackUsage(out io.Writer) {
	fmt.Fprintln(out, "usage:")
	fmt.Fprintln(out, "  levier stack up [--file path] [--build] [--profile name]")
	fmt.Fprintln(out, "  levier stack down [--file path] [--volumes]")
	fmt.Fprintln(out, "  levier stack ps [--file path]")
	fmt.Fprintln(out, "  levier stack logs [--file path] [--follow]")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "The compose file defaults to --file, LEVIER_STACK_FILE, the path saved by")
	fmt.Fprintln(out, "`levier setup --stack-file`, or docker-compose.yml in the current directory.")
	fmt.Fprintln(out, "Commands run with the compose file's directory as the project directory.")
}

type stackTarget struct {
	file        string
	projectDir  string
	projectName string
	profiles    string
}

func resolveStackTarget(fileFlag, projectDirFlag, projectName string) (stackTarget, error) {
	file, err := resolveStackFile(fileFlag)
	if err != nil {
		return stackTarget{}, err
	}
	absolute, err := filepath.Abs(file)
	if err != nil {
		return stackTarget{}, err
	}
	projectDir := strings.TrimSpace(projectDirFlag)
	if projectDir == "" {
		projectDir = filepath.Dir(absolute)
	}
	projectDir, err = filepath.Abs(projectDir)
	if err != nil {
		return stackTarget{}, err
	}
	return stackTarget{
		file:        absolute,
		projectDir:  projectDir,
		projectName: strings.TrimSpace(projectName),
	}, nil
}

func resolveStackFile(explicit string) (string, error) {
	candidates := []string{}
	if value := strings.TrimSpace(explicit); value != "" {
		candidates = append(candidates, value)
	} else {
		// A cloned repository in the current directory should win over an old
		// profile path. Installed stacks are the fallback for other directories.
		candidates = append(candidates, "docker-compose.yml", "docker-compose.yaml")
		if value := defaultStackFileValue(); value != "" {
			candidates = append(candidates, value)
		}
		if value := installedStackFileValue(); value != "" {
			candidates = append(candidates, value)
		}
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if strings.TrimSpace(explicit) != "" {
		return "", fmt.Errorf("compose file not found: %s", explicit)
	}
	return "", fmt.Errorf("no compose file found; pass --file or run `levier setup --stack-file <path>`")
}

func runStackUp(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("stack up", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "docker-compose file")
	projectDir := flags.String("project-directory", "", "compose project directory")
	projectName := flags.String("project-name", "", "compose project name")
	profiles := flags.String("profile", "", "comma-separated compose profiles")
	build := flags.Bool("build", false, "build images before starting")
	detach := flags.Bool("detach", true, "run in the background")
	dryRun := flags.Bool("dry-run", false, "print the docker compose command")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	target, err := resolveStackTarget(*file, *projectDir, *projectName)
	if err != nil {
		fmt.Fprintln(stderr, "stack:", err)
		return 2
	}
	target.profiles = *profiles
	compose, err := composeCommand(target)
	if err != nil {
		fmt.Fprintln(stderr, "stack:", err)
		return 2
	}
	compose = append(compose, "up")
	if *build {
		compose = append(compose, "--build")
	}
	if *detach {
		compose = append(compose, "--detach")
	}
	return runCompose(compose, *dryRun, stdout, stderr)
}

func runStackDown(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("stack down", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "docker-compose file")
	projectDir := flags.String("project-directory", "", "compose project directory")
	projectName := flags.String("project-name", "", "compose project name")
	volumes := flags.Bool("volumes", false, "remove named volumes")
	removeOrphans := flags.Bool("remove-orphans", true, "remove orphan containers")
	dryRun := flags.Bool("dry-run", false, "print the docker compose command")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	target, err := resolveStackTarget(*file, *projectDir, *projectName)
	if err != nil {
		fmt.Fprintln(stderr, "stack:", err)
		return 2
	}
	compose, err := composeCommand(target)
	if err != nil {
		fmt.Fprintln(stderr, "stack:", err)
		return 2
	}
	compose = append(compose, "down")
	if *volumes {
		compose = append(compose, "--volumes")
	}
	if *removeOrphans {
		compose = append(compose, "--remove-orphans")
	}
	return runCompose(compose, *dryRun, stdout, stderr)
}

func runStackPS(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("stack ps", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "docker-compose file")
	projectDir := flags.String("project-directory", "", "compose project directory")
	projectName := flags.String("project-name", "", "compose project name")
	dryRun := flags.Bool("dry-run", false, "print the docker compose command")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	target, err := resolveStackTarget(*file, *projectDir, *projectName)
	if err != nil {
		fmt.Fprintln(stderr, "stack:", err)
		return 2
	}
	compose, err := composeCommand(target)
	if err != nil {
		fmt.Fprintln(stderr, "stack:", err)
		return 2
	}
	compose = append(compose, "ps")
	return runCompose(compose, *dryRun, stdout, stderr)
}

func runStackLogs(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("stack logs", flag.ContinueOnError)
	flags.SetOutput(stderr)
	file := flags.String("file", "", "docker-compose file")
	projectDir := flags.String("project-directory", "", "compose project directory")
	projectName := flags.String("project-name", "", "compose project name")
	follow := flags.Bool("follow", false, "follow log output")
	dryRun := flags.Bool("dry-run", false, "print the docker compose command")
	if ok, code := parseFlagSet(flags, args, stdout, stderr); !ok {
		return code
	}
	target, err := resolveStackTarget(*file, *projectDir, *projectName)
	if err != nil {
		fmt.Fprintln(stderr, "stack:", err)
		return 2
	}
	compose, err := composeCommand(target)
	if err != nil {
		fmt.Fprintln(stderr, "stack:", err)
		return 2
	}
	compose = append(compose, "logs")
	if *follow {
		compose = append(compose, "--follow")
	}
	return runCompose(compose, *dryRun, stdout, stderr)
}

func composeCommand(target stackTarget) ([]string, error) {
	base := []string{"docker", "compose"}
	if _, err := exec.LookPath("docker"); err != nil {
		if _, composeErr := exec.LookPath("docker-compose"); composeErr != nil {
			return nil, fmt.Errorf("docker compose is not installed or not on PATH")
		}
		base = []string{"docker-compose"}
	}
	arguments := append([]string{}, base[1:]...)
	arguments = append(arguments,
		"--file", target.file,
		"--project-directory", target.projectDir,
	)
	if target.projectName != "" {
		arguments = append(arguments, "--project-name", target.projectName)
	}
	for _, profile := range splitCSV(target.profiles) {
		arguments = append(arguments, "--profile", profile)
	}
	if len(base) == 1 {
		return append(base, arguments...), nil
	}
	return append(base[:1], arguments...), nil
}

func runCompose(command []string, dryRun bool, stdout, stderr io.Writer) int {
	if dryRun {
		fmt.Fprintln(stdout, formatCommand(command))
		return 0
	}
	fmt.Fprintln(stdout, "running:", formatCommand(command))
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(stderr, "stack: %v\n", err)
		return 1
	}
	return 0
}

func formatCommand(command []string) string {
	parts := make([]string, 0, len(command))
	for _, item := range command {
		if strings.ContainsAny(item, " \t\"") {
			parts = append(parts, `"`+strings.ReplaceAll(item, `"`, `\"`)+`"`)
			continue
		}
		parts = append(parts, item)
	}
	return strings.Join(parts, " ")
}
