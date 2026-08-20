package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/dknathalage/gruntcmt/internal/analyze"
	"github.com/dknathalage/gruntcmt/internal/gh"
	"github.com/dknathalage/gruntcmt/internal/ghctx"
	"github.com/dknathalage/gruntcmt/internal/input"
	"github.com/dknathalage/gruntcmt/internal/render"
	"github.com/dknathalage/gruntcmt/internal/ruleset"
	"gopkg.in/yaml.v3"
)

// version is bumped by release-please on each release (see the annotation below)
// and can also be overridden at build time via -ldflags "-X main.version=...".
var version = "0.5.1" // x-release-please-version

func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return version
}

func apiBaseURL() string {
	if u := os.Getenv("GITHUB_API_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	return "https://api.github.com"
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gruntcmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		configFlag  = fs.String("config", "", "path to gruntcmt.yaml (default: ./gruntcmt.yaml, else built-in)")
		printConfig = fs.Bool("print-config", false, "print resolved config to stderr and exit")
		out         = fs.String("out", "", "output: <empty>=PR comment | summary | file path")
		showVersion = fs.Bool("version", false, "print version and exit")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "gruntcmt %s\n", resolveVersion())
		return 0
	}

	rs, err := loadConfig(*configFlag)
	if err != nil {
		fmt.Fprintln(stderr, "config:", err)
		return 1
	}
	if *printConfig {
		b, err := yaml.Marshal(rs)
		if err != nil {
			fmt.Fprintln(stderr, "config: marshal:", err)
			return 1
		}
		stderr.Write(b) //nolint:errcheck
		return 0
	}

	paths := fs.Args()
	units, loadErrs, err := input.ReadPaths(paths)
	if err != nil {
		fmt.Fprintln(stderr, "gruntcmt:", err)
		return 1
	}

	scope := ghctx.Scope(paths)
	reports := analyze.Analyze(units, loadErrs, rs, scope)
	commit := ghctx.Commit()
	for i := range reports {
		reports[i].Commit = commit
	}

	switch *out {
	case "gh", "":
		return postReports(stderr, reports)
	case "summary":
		return writeSummary(stderr, reports)
	case "-", "/dev/stdout":
		io.WriteString(stdout, renderAll(reports)) //nolint:errcheck
		return 0
	default:
		if err := os.WriteFile(*out, []byte(renderAll(reports)), 0o644); err != nil {
			fmt.Fprintln(stderr, "gruntcmt:", err)
			return 1
		}
		return 0
	}
}

// loadConfig resolves the ruleset: --config path, else ./gruntcmt.yaml, else Default().
func loadConfig(configPath string) (ruleset.Ruleset, error) {
	path := configPath
	if path == "" {
		if _, err := os.Stat("gruntcmt.yaml"); err == nil {
			path = "gruntcmt.yaml"
		}
	}
	if path == "" {
		return ruleset.Default(), nil
	}
	return ruleset.Load(path)
}

// renderAll concatenates report bodies with a blank line between them.
func renderAll(reports []analyze.Report) string {
	var b strings.Builder
	for i, rep := range reports {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(render.Render(rep))
	}
	return b.String()
}

func writeSummary(stderr io.Writer, reports []analyze.Report) int {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		fmt.Fprintln(stderr, "gruntcmt: --out summary needs $GITHUB_STEP_SUMMARY")
		return 1
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(stderr, "gruntcmt:", err)
		return 1
	}
	defer f.Close()
	if _, err := io.WriteString(f, renderAll(reports)); err != nil {
		fmt.Fprintln(stderr, "gruntcmt:", err)
		return 1
	}
	return 0
}

// postReports upserts one PR comment per report. repo/pr/token are auto-detected
// from the environment (CI) or local git/gh.
func postReports(stderr io.Writer, reports []analyze.Report) int {
	token := ghctx.Token()
	if token == "" {
		fmt.Fprintln(stderr, "gruntcmt: no GitHub token ($GITHUB_TOKEN/$GH_TOKEN or `gh auth login`)")
		return 1
	}
	repo, err := ghctx.Repo()
	if err != nil {
		fmt.Fprintln(stderr, "gruntcmt: cannot determine repo:", err)
		return 1
	}
	pr, err := ghctx.PR()
	if err != nil {
		fmt.Fprintln(stderr, "gruntcmt: cannot determine pull request:", err)
		return 1
	}
	client := &gh.Client{HTTP: http.DefaultClient, APIURL: apiBaseURL(), Token: token}
	for _, rep := range reports {
		url, err := client.UpsertComment(context.Background(), repo, pr, render.Marker(rep.Scope), render.Render(rep))
		if err != nil {
			fmt.Fprintln(stderr, "gruntcmt:", err)
			return 1
		}
		fmt.Fprintln(stderr, "gruntcmt: commented at", url)
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
