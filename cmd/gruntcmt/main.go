package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/dknathalage/gruntcmt/internal/analyze"
	"github.com/dknathalage/gruntcmt/internal/gh"
	"github.com/dknathalage/gruntcmt/internal/input"
	"github.com/dknathalage/gruntcmt/internal/render"
	"github.com/dknathalage/gruntcmt/internal/ruleset"
	"gopkg.in/yaml.v3"
)

// version is bumped by release-please on each release (see the annotation below)
// and can also be overridden at build time via -ldflags "-X main.version=...".
// For older tags where it is still "dev", resolveVersion falls back to the module
// version embedded by `go install`/`go build`.
var version = "0.4.1" // x-release-please-version

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

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("gruntcmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		scope        = fs.String("scope", "", "scope label + marker key")
		name         = fs.String("name", "plan", "unit name for a bare single plan")
		inputMode    = fs.String("input", "", "auto|wrapped|plan")
		commit       = fs.String("commit", "", "commit SHA to stamp in footer")
		rulesetFlag  = fs.String("ruleset", "", "path to ruleset YAML file")
		printRuleset = fs.Bool("print-ruleset", false, "print resolved ruleset to stderr and exit")
		out          = fs.String("out", "stdout", "output destination: stdout|gh")
		repo         = fs.String("repo", "", "owner/name for --out gh (default $GITHUB_REPOSITORY)")
		prNum        = fs.Int("pr", 0, "pull request number for --out gh (default: auto-detect in GitHub Actions)")
		showVersion  = fs.Bool("version", false, "print version and exit")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVersion {
		fmt.Fprintf(stdout, "gruntcmt %s\n", resolveVersion())
		return 0
	}

	// Load ruleset: from --ruleset, else ./gruntcmt.yaml if it exists, else empty.
	var rs ruleset.Ruleset
	path := *rulesetFlag
	if path == "" {
		if _, err := os.Stat("gruntcmt.yaml"); err == nil {
			path = "gruntcmt.yaml"
		}
	}
	if path != "" {
		loaded, err := ruleset.Load(path)
		if err != nil {
			fmt.Fprintln(stderr, "ruleset:", err)
			return 1
		}
		rs = loaded
	}

	// Resolve base if set.
	if rs.Base != "" {
		f := &ruleset.Fetcher{HTTP: http.DefaultClient, APIURL: apiBaseURL(), Token: ruleset.DefaultToken()}
		merged, err := ruleset.Resolve(context.Background(), rs, f)
		if err != nil {
			fmt.Fprintln(stderr, "ruleset:", err)
			return 1
		}
		rs = merged
	}

	if *printRuleset {
		out, err := yaml.Marshal(rs)
		if err != nil {
			fmt.Fprintln(stderr, "ruleset: marshal:", err)
			return 1
		}
		stderr.Write(out) //nolint:errcheck
		return 0
	}

	// Resolve input mode.
	resolvedInput := *inputMode
	if resolvedInput == "" {
		resolvedInput = "auto"
	}

	mode := input.ModeAuto
	switch resolvedInput {
	case "", "auto":
		mode = input.ModeAuto
	case "wrapped":
		mode = input.ModeWrapped
	case "plan":
		mode = input.ModePlan
	default:
		fmt.Fprintln(stderr, "invalid --input (want auto|wrapped|plan)")
		return 1
	}

	units, loadErrs, err := input.Read(stdin, mode, *name)
	if err != nil {
		fmt.Fprintln(stderr, "gruntcmt:", err)
		return 1
	}

	reports := analyze.Analyze(units, loadErrs, rs, *scope)
	for i := range reports {
		reports[i].Commit = *commit
	}

	switch *out {
	case "", "stdout":
		for i, rep := range reports {
			if i > 0 {
				io.WriteString(stdout, "\n") //nolint:errcheck
			}
			io.WriteString(stdout, render.Render(rep)) //nolint:errcheck
		}
		return 0
	case "gh":
		return postReports(stderr, reports, *repo, *prNum)
	default:
		fmt.Fprintln(stderr, "invalid --out (want stdout|gh)")
		return 1
	}
}

// postReports creates or updates PR comments for each report via the GitHub REST API.
// Token comes from $GITHUB_TOKEN/$GH_TOKEN; repo/pr default to the GitHub Actions
// environment. On success it prints each comment URL to stderr. Returns 1 on any failure.
func postReports(stderr io.Writer, reports []analyze.Report, repo string, pr int) int {
	token := firstEnv("GITHUB_TOKEN", "GH_TOKEN")
	if token == "" {
		fmt.Fprintln(stderr, "gruntcmt: --out gh needs a token in $GITHUB_TOKEN or $GH_TOKEN")
		return 1
	}
	if repo == "" {
		repo = os.Getenv("GITHUB_REPOSITORY")
	}
	if repo == "" {
		fmt.Fprintln(stderr, "gruntcmt: --out gh needs --repo or $GITHUB_REPOSITORY (owner/name)")
		return 1
	}
	if pr == 0 {
		pr = detectPR()
	}
	if pr == 0 {
		fmt.Fprintln(stderr, "gruntcmt: --out gh needs --pr or a detectable pull request (GITHUB_REF/GITHUB_EVENT_PATH)")
		return 1
	}
	client := &gh.Client{HTTP: http.DefaultClient, APIURL: apiBaseURL(), Token: token}
	for _, rep := range reports {
		body := render.Render(rep)
		url, err := client.UpsertComment(context.Background(), repo, pr, render.Marker(rep.Scope), body)
		if err != nil {
			fmt.Fprintln(stderr, "gruntcmt:", err)
			return 1
		}
		fmt.Fprintln(stderr, "gruntcmt: commented at", url)
	}
	return 0
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

var pullRefRe = regexp.MustCompile(`^refs/pull/(\d+)/`)

// detectPR resolves the PR number from the GitHub Actions environment: first the
// GITHUB_REF (refs/pull/<n>/merge), then the event payload at GITHUB_EVENT_PATH.
func detectPR() int {
	if m := pullRefRe.FindStringSubmatch(os.Getenv("GITHUB_REF")); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	if p := os.Getenv("GITHUB_EVENT_PATH"); p != "" {
		if raw, err := os.ReadFile(p); err == nil {
			var ev struct {
				Number      int `json:"number"`
				PullRequest struct {
					Number int `json:"number"`
				} `json:"pull_request"`
			}
			if json.Unmarshal(raw, &ev) == nil {
				if ev.PullRequest.Number != 0 {
					return ev.PullRequest.Number
				}
				return ev.Number
			}
		}
	}
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
