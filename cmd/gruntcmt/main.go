package main

import (
	"fmt"
	"io"
	"os"
)

var version = "dev"

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	for _, a := range args {
		if a == "--version" {
			fmt.Fprintf(stdout, "gruntcmt %s\n", version)
			return 0
		}
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
