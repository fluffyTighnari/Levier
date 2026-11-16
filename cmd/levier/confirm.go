package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func confirm(reader io.Reader, writer io.Writer, prompt string) bool {
	fmt.Fprintf(writer, "%s [y/N]: ", prompt)
	line, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
