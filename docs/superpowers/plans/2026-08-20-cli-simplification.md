# gruntcmt CLI Simplification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn gruntcmt into a pure CLI that reads terragrunt plan JSON from file/directory arguments, renders a Markdown summary, and by default upserts it as a PR comment — working identically locally and in CI.

**Architecture:** Build three additive, independently-tested units first — a context-detection package (`internal/ghctx`), a default ruleset (`ruleset.Default()`), and a path-based input reader (`input.ReadPaths`). Then flip `cmd/gruntcmt/main.go` to the new world in one atomic cutover task that also deletes the dead base-fetch/merge code and the stdin/NDJSON input path. Finish with docs/examples cleanup.

**Tech Stack:** Go (stdlib `flag`, `os`, `path/filepath`, `os/exec`, `net/http`), `gopkg.in/yaml.v3`, `github.com/bmatcuk/doublestar/v4`.

**Spec:** `docs/superpowers/specs/2026-08-20-gruntcmt-cli-simplification-design.md`

## Global Constraints

- Module path: `github.com/dknathalage/gruntcmt`. Go per `go.mod` (1.x, stdlib only beyond existing deps).
- The core pipeline (input → analyze → render → file/summary/stdout) must not import `os/exec`, `git`, or `gh`. Detection helpers that shell out live only in `internal/ghctx` and are invoked only on the `gh` output path.
- Token precedence everywhere: `$GITHUB_TOKEN` → `$GH_TOKEN` → `gh auth token`.
- `$GITHUB_API_URL` still overrides the GitHub API base (Enterprise); default `https://api.github.com`.
- Keep `go build ./...` and `go test ./...` green at the end of every task.
- Commit after each task.

---

## File Structure

- Create: `internal/ghctx/ghctx.go` — repo/PR/commit/token/scope detection (env → git/gh).
- Create: `internal/ghctx/ghctx_test.go` — pure-parser tests.
- Create: `internal/input/read.go` — `ReadPaths` + discovery/name helpers.
- Modify: `internal/ruleset/ruleset.go` — drop `Base`, add `Default()`.
- Delete: `internal/ruleset/resolve.go`, `internal/ruleset/fetch.go`, `internal/ruleset/resolve_test.go`, `internal/ruleset/fetch_test.go`.
- Delete: `internal/input/input.go`, `internal/input/input_test.go` (old stdin/NDJSON reader) — replaced by `read.go`.
- Rewrite: `cmd/gruntcmt/main.go`, `cmd/gruntcmt/main_test.go`.
- Docs/examples: `README.md`, `examples/README.md`, `examples/terragrunt/README.md`, `examples/terragrunt/plan-scenarios.sh`; delete `examples/terragrunt/base.yaml`, `examples/workflows/terragrunt-plan.yml`, `.github/workflows/pr-demo.yml`; edit `examples/terragrunt/gruntcmt.yaml`.

---

## Task 1: Context detection package (`internal/ghctx`)

Additive. Provides repo/PR/commit/token/scope resolution with pure, testable parsers underneath.

**Files:**
- Create: `internal/ghctx/ghctx.go`
- Test: `internal/ghctx/ghctx_test.go`

**Interfaces:**
- Consumes: nothing (new leaf package; stdlib `os`, `os/exec`, `strings`, `strconv`, `encoding/json`, `path/filepath`, `regexp`).
- Produces:
  - `func Token() string`
  - `func Repo() (string, error)` — `"owner/name"`
  - `func PR() (int, error)`
  - `func Commit() string`
  - `func Scope(paths []string) string`
  - pure helpers: `func parseRepoRemote(url string) (string, bool)`, `func prFromRef(ref string) (int, bool)`, `func scopeFromPath(p string) string`

- [ ] **Step 1: Write failing tests for the pure parsers**

```go
package ghctx

import "testing"

func TestParseRepoRemote(t *testing.T) {
	cases := map[string]string{
		"git@github.com:dknathalage/gruntcmt.git":     "dknathalage/gruntcmt",
		"https://github.com/dknathalage/gruntcmt.git": "dknathalage/gruntcmt",
		"https://github.com/dknathalage/gruntcmt":     "dknathalage/gruntcmt",
		"ssh://git@github.com/dknathalage/gruntcmt.git": "dknathalage/gruntcmt",
	}
	for in, want := range cases {
		got, ok := parseRepoRemote(in)
		if !ok || got != want {
			t.Errorf("parseRepoRemote(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := parseRepoRemote("not-a-remote"); ok {
		t.Error("expected failure for junk remote")
	}
}

func TestPRFromRef(t *testing.T) {
	if n, ok := prFromRef("refs/pull/42/merge"); !ok || n != 42 {
		t.Errorf("got %d,%v want 42", n, ok)
	}
	if _, ok := prFromRef("refs/heads/main"); ok {
		t.Error("expected no PR for branch ref")
	}
}

func TestScopeFromPath(t *testing.T) {
	cases := map[string]string{
		"envs/prod":       "prod",
		"envs/prod/":      "prod",
		"out":             "out",
		".":               "plan",
		"":                "plan",
		"/":               "plan",
	}
	for in, want := range cases {
		if got := scopeFromPath(in); got != want {
			t.Errorf("scopeFromPath(%q) = %q want %q", in, got, want)
		}
	}
}

func TestScopeUsesFirstPath(t *testing.T) {
	if got := Scope([]string{"envs/staging", "envs/prod"}); got != "staging" {
		t.Errorf("Scope = %q want staging", got)
	}
	if got := Scope(nil); got != "plan" {
		t.Errorf("Scope(nil) = %q want plan", got)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail to compile (functions undefined)**

Run: `go test ./internal/ghctx/...`
Expected: FAIL (undefined: parseRepoRemote, prFromRef, scopeFromPath, Scope)

- [ ] **Step 3: Implement `internal/ghctx/ghctx.go`**

```go
// Package ghctx resolves the repository, pull request, commit, token, and
// comment scope for gruntcmt. Every value prefers a CI environment variable and
// falls back to a local git/gh invocation, so behavior is identical locally and
// in CI. Only the --out gh path calls into this package.
package ghctx

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Token resolves a GitHub token: $GITHUB_TOKEN, then $GH_TOKEN, then `gh auth token`.
func Token() string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Repo resolves "owner/name" from $GITHUB_REPOSITORY, else the local git origin.
func Repo() (string, error) {
	if v := os.Getenv("GITHUB_REPOSITORY"); v != "" {
		return v, nil
	}
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	if r, ok := parseRepoRemote(strings.TrimSpace(string(out))); ok {
		return r, nil
	}
	return "", &os.PathError{Op: "parse", Path: "origin", Err: os.ErrInvalid}
}

var repoRe = regexp.MustCompile(`[:/]([^/:]+)/([^/]+?)(?:\.git)?$`)

func parseRepoRemote(url string) (string, bool) {
	m := repoRe.FindStringSubmatch(strings.TrimSpace(url))
	if m == nil {
		return "", false
	}
	return m[1] + "/" + m[2], true
}

var pullRefRe = regexp.MustCompile(`^refs/pull/(\d+)/`)

func prFromRef(ref string) (int, bool) {
	if m := pullRefRe.FindStringSubmatch(ref); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n, true
		}
	}
	return 0, false
}

// PR resolves the pull-request number: $GITHUB_REF, then the event payload at
// $GITHUB_EVENT_PATH, then `gh pr view` for the current branch.
func PR() (int, error) {
	if n, ok := prFromRef(os.Getenv("GITHUB_REF")); ok {
		return n, nil
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
					return ev.PullRequest.Number, nil
				}
				if ev.Number != 0 {
					return ev.Number, nil
				}
			}
		}
	}
	out, err := exec.Command("gh", "pr", "view", "--json", "number", "-q", ".number").Output()
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0, err
	}
	return n, nil
}

// Commit resolves the commit SHA: $GITHUB_SHA, else `git rev-parse HEAD`.
func Commit() string {
	if v := os.Getenv("GITHUB_SHA"); v != "" {
		return v
	}
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Scope derives the comment scope from the first path argument's basename.
func Scope(paths []string) string {
	if len(paths) == 0 {
		return "plan"
	}
	return scopeFromPath(paths[0])
}

func scopeFromPath(p string) string {
	base := filepath.Base(filepath.Clean(p))
	if base == "." || base == "/" || base == "" {
		return "plan"
	}
	return base
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/ghctx/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ghctx/
git commit -m "feat(ghctx): add repo/pr/commit/token/scope detection"
```

---

## Task 2: Default ruleset (`ruleset.Default`)

Additive change to the ruleset package: add `Default()`. Do **not** remove `Base` yet (that happens in Task 4, atomically with main.go).

**Files:**
- Modify: `internal/ruleset/ruleset.go`
- Test: `internal/ruleset/ruleset_test.go` (append)

**Interfaces:**
- Consumes: existing `Ruleset`, `Rule`, `(Ruleset).Detail`, `plan.Fidelity`, `plan.Action`.
- Produces: `func Default() Ruleset`.

- [ ] **Step 1: Write failing test**

```go
func TestDefaultRulesetDetail(t *testing.T) {
	rs := Default()
	// destructive + create/update → attribute; noop/read → summary
	if d := rs.Detail("any/unit", plan.ActionDelete); d != plan.FidelityAttribute {
		t.Errorf("delete detail = %v want attribute", d)
	}
	if d := rs.Detail("any/unit", plan.ActionReplace); d != plan.FidelityAttribute {
		t.Errorf("replace detail = %v want attribute", d)
	}
	if d := rs.Detail("any/unit", plan.ActionCreate); d != plan.FidelityAttribute {
		t.Errorf("create detail = %v want attribute", d)
	}
	if d := rs.Detail("any/unit", plan.ActionUpdate); d != plan.FidelityAttribute {
		t.Errorf("update detail = %v want attribute", d)
	}
	if d := rs.Detail("any/unit", plan.ActionNoOp); d != plan.FidelitySummary {
		t.Errorf("noop detail = %v want summary", d)
	}
}

func TestDefaultRulesetGrouping(t *testing.T) {
	rs := Default()
	if gb := rs.GroupBy("", false); gb != 0 {
		t.Errorf("group-by = %d want 0", gb)
	}
	if title := rs.Title("", false); title != "Terragrunt plan" {
		t.Errorf("title = %q want Terragrunt plan", title)
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

Run: `go test ./internal/ruleset/ -run TestDefault`
Expected: FAIL (undefined: Default)

- [ ] **Step 3: Implement `Default()` in `ruleset.go`**

Add near the top-level funcs:

```go
// Default is the built-in ruleset used when no config file is present:
// a single flat group with full attribute detail for every real change.
func Default() Ruleset {
	zero := 0
	return Ruleset{Rules: []Rule{{
		Path:    "**",
		Title:   "Terragrunt plan",
		GroupBy: &zero,
		Create:  "attribute",
		Update:  "attribute",
		Delete:  "attribute",
		Replace: "attribute",
		Noop:    "summary",
	}}}
}
```

- [ ] **Step 4: Run test, verify pass**

Run: `go test ./internal/ruleset/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ruleset/ruleset.go internal/ruleset/ruleset_test.go
git commit -m "feat(ruleset): add built-in Default ruleset"
```

---

## Task 3: Path-based input reader (`input.ReadPaths`)

Additive: create `internal/input/read.go` with the new reader alongside the old `input.go`. The old `Read`/`Mode` stay until Task 4 so the tree keeps building.

**Files:**
- Create: `internal/input/read.go`
- Test: `internal/input/read_test.go`

**Interfaces:**
- Consumes: `plan.ParsePlan(name string, raw []byte) (plan.Unit, error)`, `plan.Unit`, `plan.LoadError`.
- Produces:
  - `func ReadPaths(paths []string) ([]plan.Unit, []plan.LoadError, error)`
  - pure helper: `func unitName(root, file string) string`

- [ ] **Step 1: Write failing tests**

```go
package input

import (
	"os"
	"path/filepath"
	"testing"
)

const barePlan = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestUnitNameFromTree(t *testing.T) {
	if got := unitName("out", filepath.FromSlash("out/aws/prod/networking/tfplan.json")); got != "aws/prod/networking" {
		t.Errorf("unitName = %q want aws/prod/networking", got)
	}
}

func TestReadPathsDirRecursive(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "aws/prod/networking/tfplan.json"), barePlan)
	writeFile(t, filepath.Join(dir, "aws/prod/db/tfplan.json"), barePlan)
	writeFile(t, filepath.Join(dir, "gcp/staging/tfplan.json"), barePlan)
	units, le, err := ReadPaths([]string{dir})
	if err != nil || len(le) != 0 {
		t.Fatalf("err=%v le=%v", err, le)
	}
	names := map[string]bool{}
	for _, u := range units {
		names[u.Name] = true
	}
	for _, want := range []string{"aws/prod/networking", "aws/prod/db", "gcp/staging"} {
		if !names[want] {
			t.Errorf("missing unit %q in %v", want, names)
		}
	}
}

func TestReadPathsSingleFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "vpc.json")
	writeFile(t, f, barePlan)
	units, _, err := ReadPaths([]string{f})
	if err != nil || len(units) != 1 || units[0].Name != filepath.Join(dir, "vpc") {
		t.Fatalf("units=%+v err=%v", units, err)
	}
}

func TestReadPathsBadPlanIsolated(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "good/tfplan.json"), barePlan)
	writeFile(t, filepath.Join(dir, "bad/tfplan.json"), `{"no":"format_version"}`)
	units, le, err := ReadPaths([]string{dir})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(units) != 1 || len(le) != 1 {
		t.Fatalf("units=%+v le=%+v", units, le)
	}
}

func TestReadPathsNoArgsErrors(t *testing.T) {
	if _, _, err := ReadPaths(nil); err == nil {
		t.Fatal("expected error for no paths")
	}
}

func TestReadPathsEmptyDirErrors(t *testing.T) {
	if _, _, err := ReadPaths([]string{t.TempDir()}); err == nil {
		t.Fatal("expected error when no plans found")
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

Run: `go test ./internal/input/ -run ReadPaths`
Expected: FAIL (undefined: ReadPaths, unitName)

- [ ] **Step 3: Implement `internal/input/read.go`**

```go
package input

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dknathalage/gruntcmt/internal/plan"
)

// ReadPaths loads plan units from file and directory arguments. Directories are
// walked recursively for terragrunt's tfplan.json files; each unit's name is its
// location within the tree. A file that cannot be read or parsed becomes an
// isolated LoadError rather than failing the whole run.
func ReadPaths(paths []string) ([]plan.Unit, []plan.LoadError, error) {
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("no plan files given")
	}
	var units []plan.Unit
	var loadErrs []plan.LoadError
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, nil, fmt.Errorf("stat %s: %w", p, err)
		}
		if info.IsDir() {
			err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() || d.Name() != "tfplan.json" {
					return nil
				}
				addUnit(&units, &loadErrs, unitName(p, path), path)
				return nil
			})
			if err != nil {
				return nil, nil, fmt.Errorf("walk %s: %w", p, err)
			}
		} else {
			addUnit(&units, &loadErrs, unitName("", p), p)
		}
	}
	if len(units) == 0 && len(loadErrs) == 0 {
		return nil, nil, fmt.Errorf("no plans found")
	}
	return units, loadErrs, nil
}

func addUnit(units *[]plan.Unit, loadErrs *[]plan.LoadError, name, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		*loadErrs = append(*loadErrs, plan.LoadError{Name: name, Message: err.Error()})
		return
	}
	u, err := plan.ParsePlan(name, data)
	if err != nil {
		*loadErrs = append(*loadErrs, plan.LoadError{Name: name, Message: err.Error()})
		return
	}
	*units = append(*units, u)
}

// unitName derives a unit path. For a tfplan.json under a directory root, it is
// the parent directory relative to root. For an explicit file, it is the path
// with a .json suffix stripped (with the same tfplan.json → parent-dir rule).
func unitName(root, file string) string {
	if filepath.Base(file) == "tfplan.json" {
		dir := filepath.Dir(file)
		if root != "" {
			if rel, err := filepath.Rel(root, dir); err == nil {
				return filepath.ToSlash(rel)
			}
		}
		return filepath.ToSlash(dir)
	}
	return filepath.ToSlash(strings.TrimSuffix(filepath.Clean(file), ".json"))
}
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/input/`
Expected: PASS (old input_test.go still passes too)

- [ ] **Step 5: Commit**

```bash
git add internal/input/read.go internal/input/read_test.go
git commit -m "feat(input): add path/directory plan reader"
```

---

## Task 4: Cutover — rewrite `main.go`, delete dead code

The atomic flip. Wires ghctx + Default + ReadPaths into `main.go`, implements the new flags and `--out` behavior, and removes everything the old world used: ruleset `Base`/`resolve.go`/`fetch.go`, old `input.go` reader, and their tests.

**Files:**
- Rewrite: `cmd/gruntcmt/main.go`
- Rewrite: `cmd/gruntcmt/main_test.go`
- Modify: `internal/ruleset/ruleset.go` (remove `Base` field)
- Delete: `internal/ruleset/resolve.go`, `internal/ruleset/fetch.go`, `internal/ruleset/resolve_test.go`, `internal/ruleset/fetch_test.go`
- Delete: `internal/input/input.go`, `internal/input/input_test.go`

**Interfaces:**
- Consumes: `ghctx.Token/Repo/PR/Commit/Scope`, `ruleset.Default/Load`, `input.ReadPaths`, `analyze.Analyze`, `render.Render/Marker`, `gh.Client.UpsertComment`.
- Produces: `func run(args []string, stdout, stderr io.Writer) int` (stdin parameter removed).

- [ ] **Step 1: Delete dead ruleset/input files and the `Base` field**

```bash
git rm internal/ruleset/resolve.go internal/ruleset/fetch.go \
       internal/ruleset/resolve_test.go internal/ruleset/fetch_test.go \
       internal/input/input.go internal/input/input_test.go
```

Then edit `internal/ruleset/ruleset.go`: remove the `Base string yaml:"base"` line from the `Ruleset` struct so it reads:

```go
type Ruleset struct {
	Rules []Rule `yaml:"rules"`
}
```

At this point `go build ./...` will fail in `cmd/gruntcmt` — that is expected until Step 3.

- [ ] **Step 2: Write the new `cmd/gruntcmt/main_test.go`**

Replace the file entirely. Keep the version tests; rewrite the rest around file args and `--out`.

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const barePlan = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`

func writePlan(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(barePlan), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunVersion(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"--version"}, &out, &errBuf); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out.String(), "gruntcmt") {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestResolveVersionPrefersLdflags(t *testing.T) {
	orig := version
	defer func() { version = orig }()
	version = "v9.9.9"
	if got := resolveVersion(); got != "v9.9.9" {
		t.Fatalf("got %q", got)
	}
}

func TestRunToFile(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, filepath.Join(dir, "prod/networking/tfplan.json"))
	outFile := filepath.Join(t.TempDir(), "report.md")
	var out, errBuf bytes.Buffer
	code := run([]string{"--out", outFile, dir}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	body, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	// scope derived from input dir basename
	if !strings.Contains(string(body), "gruntcmt:scope="+filepath.Base(dir)) {
		t.Errorf("missing scope marker:\n%s", body)
	}
}

func TestRunToStdoutDevice(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, filepath.Join(dir, "prod/tfplan.json"))
	var out, errBuf bytes.Buffer
	if code := run([]string{"--out", "/dev/stdout", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if !strings.HasPrefix(out.String(), "<!-- gruntcmt:scope=") {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestRunSummary(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, filepath.Join(dir, "prod/tfplan.json"))
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summary)
	var out, errBuf bytes.Buffer
	if code := run([]string{"--out", "summary", dir}, &out, &errBuf); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	body, err := os.ReadFile(summary)
	if err != nil || !strings.Contains(string(body), "gruntcmt:scope=") {
		t.Fatalf("summary body=%q err=%v", body, err)
	}
}

func TestRunSummaryWithoutEnvFails(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, filepath.Join(dir, "prod/tfplan.json"))
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	var out, errBuf bytes.Buffer
	if code := run([]string{"--out", "summary", dir}, &out, &errBuf); code == 0 {
		t.Fatal("expected failure without GITHUB_STEP_SUMMARY")
	}
}

func TestRunNoArgsFails(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run([]string{"--out", "/dev/stdout"}, &out, &errBuf); code == 0 {
		t.Fatal("expected failure with no plan paths")
	}
}

func TestRunRejectsRemovedFlag(t *testing.T) {
	dir := t.TempDir()
	writePlan(t, filepath.Join(dir, "prod/tfplan.json"))
	var out, errBuf bytes.Buffer
	if code := run([]string{"--scope", "x", "--out", "/dev/stdout", dir}, &out, &errBuf); code == 0 {
		t.Fatal("expected failure: --scope was removed")
	}
}

func TestRunPrintConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "gruntcmt.yaml")
	os.WriteFile(cfg, []byte("rules:\n  - path: \"**\"\n    delete: attribute\n"), 0o644)
	var out, errBuf bytes.Buffer
	if code := run([]string{"--config", cfg, "--print-config"}, &out, &errBuf); code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "rules:") {
		t.Fatalf("expected ruleset yaml on stderr, got %q", errBuf.String())
	}
}
```

- [ ] **Step 3: Rewrite `cmd/gruntcmt/main.go`**

```go
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

var version = "0.4.2" // x-release-please-version

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

// postReports upserts one PR comment per report. repo/pr/token are auto-detected.
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
```

- [ ] **Step 4: Rename the new input reader to the canonical name**

The public reader is now the only one, so rename `ReadPaths` → keep as `ReadPaths` (used by main). No action required if main calls `input.ReadPaths`. (Leave the name `ReadPaths`; it is descriptive and already referenced.)

- [ ] **Step 5: Build and run the whole test suite**

Run: `go build ./... && go test ./...`
Expected: PASS. If `analyze` or `render` tests referenced removed behavior, none should — they operate on `Ruleset`/`Report` unchanged.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: pure-CLI cutover — path args, auto-detect, --out, drop base/stdin"
```

---

## Task 5: Docs, examples, and workflow cleanup

**Files:**
- Delete: `examples/terragrunt/base.yaml`, `examples/workflows/terragrunt-plan.yml`, `.github/workflows/pr-demo.yml`
- Modify: `examples/terragrunt/gruntcmt.yaml`, `examples/terragrunt/README.md`, `examples/terragrunt/plan-scenarios.sh`, `examples/README.md`, `README.md`

- [ ] **Step 1: Delete Action-oriented files**

```bash
git rm examples/terragrunt/base.yaml examples/workflows/terragrunt-plan.yml .github/workflows/pr-demo.yml
```

- [ ] **Step 2: Inline the base rule into `examples/terragrunt/gruntcmt.yaml`**

Replace the `base:` line with the rule that used to live in `base.yaml`. Final file (no `base:`):

```yaml
rules:
  - path: "**"
    title: "gruntcmt scenarios"
    group-by: 1
    create: summary
    update: resource
    delete: attribute
    replace: attribute
    noop: summary
  - path: "**/06-security"
    dedicated-comment: true
    scope: security
    title: "Security plan"
    create: attribute
    delete: attribute
```

- [ ] **Step 3: Update `examples/terragrunt/plan-scenarios.sh` and READMEs to the new flow**

Change the invocation from the piped `--ruleset` form to:

```sh
terragrunt run --all plan --json-out-dir out
gruntcmt --config gruntcmt.yaml out
```

Update prose in `examples/terragrunt/README.md` and `examples/README.md` accordingly (remove `--scope`/`--ruleset`/stdin references; describe `tfplan.json` discovery).

- [ ] **Step 4: Rewrite the relevant `README.md` sections**

Remove the GitHub Action / marketplace framing. Document:
- Install (`go install github.com/dknathalage/gruntcmt/cmd/gruntcmt@latest`).
- Usage: `terragrunt run --all plan --json-out-dir out && gruntcmt out`.
- Config: optional `./gruntcmt.yaml` or `--config`; built-in default when absent.
- Output: default PR comment; `--out summary`; `--out <file>`; `--out /dev/stdout`.
- Detection: repo/PR/commit/token resolved from CI env or local `git`/`gh` — same behavior either way; scope derived from the input directory name.

- [ ] **Step 5: Verify nothing references removed flags/features**

Run: `grep -rn -- "--ruleset\|--scope\|--input\|--commit\|--repo\|--pr\|base:\|action.yml\|uses:" README.md examples/ | grep -v release-please`
Expected: no stale references (fix any that remain).

- [ ] **Step 6: Final build/test and commit**

```bash
go build ./... && go test ./...
git add -A
git commit -m "docs: retarget README and examples to the pure CLI flow"
```

---

## Self-Review

- **Spec coverage:** §1 config→Task 2+4; §2 input→Task 3+4; §3 output→Task 4; §4 detection→Task 1+4; §5 default ruleset→Task 2; §6 remove action→Task 5 (action.yml already deleted upstream). All covered.
- **Placeholders:** none — every step has concrete code/commands.
- **Type consistency:** `ReadPaths`, `ghctx.Scope/Repo/PR/Commit/Token`, `ruleset.Default`, `run(args, stdout, stderr)`, `renderAll`, `render.Marker/Render`, `gh.Client.UpsertComment` are used consistently across tasks.

## Notes / risks

- `action.yml` was already removed and committed before this plan; Task 5 only handles the remaining Action-flavored example/workflow files.
- Detection helpers that shell out (`git`/`gh`) are not exercised by unit tests; only their pure parsers are. The `gh` default output path is covered manually (see the verify skill) rather than in unit tests.
- `render` already renders the empty group key as `(all)` and stamps `Report.Commit`, so group-by 0 and the always-on commit footer need no render changes.
