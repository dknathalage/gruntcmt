package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dknathalage/gruntcmt/internal/analyze"
	"github.com/dknathalage/gruntcmt/internal/config"
	"github.com/dknathalage/gruntcmt/internal/input"
	"github.com/dknathalage/gruntcmt/internal/plan"
	"github.com/dknathalage/gruntcmt/internal/render"
)

var version = "dev"

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gruntcmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		scope       = fs.String("scope", "", "scope label + marker key")
		name        = fs.String("name", "plan", "unit name for a bare single plan")
		groupBy     = fs.Int("group-by", 1, "leading path segments to group by")
		detail      = fs.String("detail", "", "summary|resource|attribute (overrides config)")
		inputMode   = fs.String("input", "", "auto|wrapped|plan")
		commit      = fs.String("commit", "", "commit SHA to stamp in footer")
		configPath  = fs.String("config", "", "explicit config file")
		noConfig    = fs.Bool("no-config", false, "ignore all config files")
		printConfig = fs.Bool("print-config", false, "print resolved config to stderr and exit")
		showVersion = fs.Bool("version", false, "print version and exit")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "gruntcmt %s\n", version)
		return 0
	}

	// Resolve config layers.
	var layers []config.File
	if !*noConfig {
		if home, err := os.UserConfigDir(); err == nil {
			if f, err := config.LoadFile(home + "/gruntcmt/config.yaml"); err == nil {
				layers = append(layers, f)
			}
		}
		if p, ok := config.Discover(cwd()); ok {
			f, err := config.LoadFile(p)
			if err != nil {
				fmt.Fprintln(stderr, "config:", err)
				return 1
			}
			layers = append(layers, f)
		}
		if *configPath != "" {
			f, err := config.LoadFile(*configPath)
			if err != nil {
				fmt.Fprintln(stderr, "config:", err)
				return 1
			}
			layers = append(layers, f)
		}
	}
	merged := config.Merge(layers...)

	s := config.Settings{
		Scope: *scope, Name: *name, Commit: *commit,
		GroupBy: *groupBy, Render: merged.Render, Overrides: merged.Overrides,
		Detail: plan.FidelityResource,
	}
	if merged.GroupBy != nil && !flagSet(fs, "group-by") {
		s.GroupBy = *merged.GroupBy
	}
	if merged.Detail != "" {
		if f, err := config.ParseFidelity(merged.Detail); err == nil {
			s.Detail = f
		}
	}
	if *detail != "" {
		f, err := config.ParseFidelity(*detail)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		s.Detail, s.DetailSet = f, true
	}

	if *printConfig {
		fmt.Fprintf(stderr, "%+v\n", s)
		return 0
	}

	mode := input.ModeAuto
	switch pick(*inputMode, merged.Input) {
	case "wrapped":
		mode = input.ModeWrapped
	case "plan":
		mode = input.ModePlan
	}

	units, loadErrs, err := input.Read(stdin, mode, *name)
	if err != nil {
		fmt.Fprintln(stderr, "gruntcmt:", err)
		return 1
	}
	report := analyze.Analyze(units, loadErrs, s)
	io.WriteString(stdout, render.Render(report, s))
	return 0
}

func flagSet(fs *flag.FlagSet, name string) bool {
	seen := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			seen = true
		}
	})
	return seen
}

func pick(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func cwd() string {
	if d, err := os.Getwd(); err == nil {
		return d
	}
	return "."
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
