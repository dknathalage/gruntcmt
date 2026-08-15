# gruntcmt Ruleset Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace gruntcmt's layered config with a composable **ruleset** that assigns render fidelity per resource change (path × action), splits output into multiple PR comments, and can extend a `base` ruleset fetched from another GitHub repo.

**Architecture:** New `internal/ruleset` package (types, YAML load, per-change resolution, GitHub base fetch + merge) replaces `internal/config`. `analyze` partitions units into one `Report` per comment (main + dedicated) and stamps each `ResourceChange.Detail`. `render` renders one `Report` using per-change detail. `cmd` loads+merges the ruleset and emits N documents (stdout concatenated, or one `--out gh` post per marker).

**Tech Stack:** Go 1.24, stdlib, `gopkg.in/yaml.v3`, `github.com/bmatcuk/doublestar/v4`. `net/http` + `httptest` for the base fetch.

**Spec:** `docs/superpowers/specs/2026-08-15-gruntcmt-ruleset-design.md`

## Global Constraints

- Module `github.com/dknathalage/gruntcmt`; Go 1.24; breaking release **v0.4.0**.
- Config file is `gruntcmt.yaml` (non-hidden); CLI flag `--ruleset` (no upward discovery, no global config, no env/flag overrides of ruleset values).
- Removed flags: `--config`, `--detail`, `--group-by`, `--no-config`; renamed `--print-config` → `--print-ruleset`. Kept: `--ruleset --scope --name --input --commit --out --repo --pr --print-ruleset --version`.
- Detail is **per resource change**, resolved by the LAST rule whose `path` (doublestar) matches the unit AND that names the change's action. Built-in defaults: create/update/delete/replace = `resource`, noop = `summary`; report defaults: `group-by` 1, `title` "Terragrunt plan".
- Per-change detail meaning: `summary` = not listed (counts only), `resource` = address + action line, `attribute` = address + before→after diff. A unit renders a `<details>` block iff ≥1 change resolves above `summary`; a no-op unit (no changes) collapses to a "no changes" line.
- `dedicated-comment: true` rules pull matching units into their own comment (own `scope`+`title`); a unit's comment = the LAST dedicated rule matching it, else the main comment (`--scope`). One run emits multiple comments.
- `base:` GitHub shorthand `owner/repo//path@ref`, fetched via the contents API with default auth (`GITHUB_TOKEN`/`GH_TOKEN`, else `gh auth token`); recursive with cycle guard; base rules first, local appended (local wins under last-match-wins).
- Domain types live in `plan`; `ruleset`/`input`/`analyze`/`render` depend on `plan`, never the reverse. `gh`/`ruleset` fetch are the only networked code.
- Counts mirror Terraform (Replace = Add+Destroy+Replace). Severity: Delete=Replace > Update > Create > Read > NoOp.

---

### Task 1: `internal/ruleset` — types, YAML load, per-change resolution

**Files:**
- Create: `internal/ruleset/ruleset.go`
- Test: `internal/ruleset/ruleset_test.go`

**Interfaces:**
- Consumes: `plan.Fidelity`, `plan.Action` (+ its constants).
- Produces:

```go
type Rule struct {
	Path             string `yaml:"path"`
	Title            string `yaml:"title"`
	GroupBy          *int   `yaml:"group-by"`
	DedicatedComment bool   `yaml:"dedicated-comment"`
	Scope            string `yaml:"scope"`
	Create           string `yaml:"create"`
	Update           string `yaml:"update"`
	Delete           string `yaml:"delete"`
	Replace          string `yaml:"replace"`
	Noop             string `yaml:"noop"`
}

type Ruleset struct {
	Base  string `yaml:"base"`
	Rules []Rule `yaml:"rules"`
}

func Parse(data []byte) (Ruleset, error)                        // YAML + validate detail strings
func Load(path string) (Ruleset, error)                        // read file then Parse
func ParseFidelity(s string) (plan.Fidelity, error)            // summary|resource|attribute
func (rs Ruleset) Detail(unitPath string, a plan.Action) plan.Fidelity
func (rs Ruleset) Assign(unitPath string) (scope string, dedicated bool) // last dedicated match
func (rs Ruleset) DedicatedScopes() []string                   // ordered unique dedicated scopes
func (rs Ruleset) Title(scope string, dedicated bool) string
func (rs Ruleset) GroupBy(scope string, dedicated bool) int
```

- Resolution rules: `Detail` maps `a` to its yaml key (`create/update/delete/replace/noop`; `Read` → treated as `noop`), scans `rs.Rules` in order, and for each rule whose `Path` matches `unitPath` (doublestar) and whose field for that action is non-empty, records it (last wins). Falls back to built-in defaults (create/update/delete/replace→`resource`, noop→`summary`). `Assign` returns the last dedicated rule's `Scope` matching the path. `Title`/`GroupBy`: for a dedicated scope, the last dedicated rule with that scope that sets the field, else fall through to the last non-dedicated rule that sets it, else defaults (`"Terragrunt plan"`, `1`). `Parse` returns an error if any detail field is a non-empty invalid fidelity.

- [ ] **Step 1: Write the failing test**

Create `internal/ruleset/ruleset_test.go`:

```go
package ruleset

import (
	"testing"

	"github.com/dknathalage/gruntcmt/internal/plan"
)

const sample = `
rules:
  - path: "**"
    title: "gruntcmt scenarios"
    group-by: 1
    create: summary
    update: resource
    delete: attribute
    replace: attribute
    noop: summary
  - path: "**/database*"
    create: attribute
  - path: "**/security/**"
    dedicated-comment: true
    scope: security
    title: "Security plan"
    delete: attribute
`

func mustParse(t *testing.T) Ruleset {
	t.Helper()
	rs, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestDetailPerAction(t *testing.T) {
	rs := mustParse(t)
	if got := rs.Detail("prod/networking", plan.ActionCreate); got != plan.FidelitySummary {
		t.Errorf("create default = %v, want summary", got)
	}
	if got := rs.Detail("prod/networking", plan.ActionDelete); got != plan.FidelityAttribute {
		t.Errorf("delete default = %v, want attribute", got)
	}
	// database override raises create to attribute (last match wins)
	if got := rs.Detail("prod/database-primary", plan.ActionCreate); got != plan.FidelityAttribute {
		t.Errorf("db create = %v, want attribute", got)
	}
	// unspecified action falls back to built-in default (resource)
	rsMin, _ := Parse([]byte("rules:\n  - path: \"**\"\n    delete: attribute\n"))
	if got := rsMin.Detail("x", plan.ActionUpdate); got != plan.FidelityResource {
		t.Errorf("update fallback = %v, want resource", got)
	}
}

func TestAssignAndMeta(t *testing.T) {
	rs := mustParse(t)
	if scope, ded := rs.Assign("prod/security/iam"); !ded || scope != "security" {
		t.Errorf("security assign = %q,%v", scope, ded)
	}
	if _, ded := rs.Assign("prod/networking"); ded {
		t.Error("networking should be main (not dedicated)")
	}
	if got := rs.DedicatedScopes(); len(got) != 1 || got[0] != "security" {
		t.Errorf("dedicated scopes = %v", got)
	}
	if got := rs.Title("", false); got != "gruntcmt scenarios" {
		t.Errorf("main title = %q", got)
	}
	if got := rs.Title("security", true); got != "Security plan" {
		t.Errorf("dedicated title = %q", got)
	}
	if got := rs.GroupBy("", false); got != 1 {
		t.Errorf("main group-by = %d", got)
	}
}

func TestParseRejectsBadDetail(t *testing.T) {
	if _, err := Parse([]byte("rules:\n  - path: \"**\"\n    create: bogus\n")); err == nil {
		t.Fatal("expected error for invalid detail")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ruleset/`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Write minimal implementation**

Create `internal/ruleset/ruleset.go`:

```go
package ruleset

import (
	"fmt"
	"os"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/dknathalage/gruntcmt/internal/plan"
	"gopkg.in/yaml.v3"
)

type Rule struct {
	Path             string `yaml:"path"`
	Title            string `yaml:"title"`
	GroupBy          *int   `yaml:"group-by"`
	DedicatedComment bool   `yaml:"dedicated-comment"`
	Scope            string `yaml:"scope"`
	Create           string `yaml:"create"`
	Update           string `yaml:"update"`
	Delete           string `yaml:"delete"`
	Replace          string `yaml:"replace"`
	Noop             string `yaml:"noop"`
}

type Ruleset struct {
	Base  string `yaml:"base"`
	Rules []Rule `yaml:"rules"`
}

func ParseFidelity(s string) (plan.Fidelity, error) {
	switch s {
	case "summary":
		return plan.FidelitySummary, nil
	case "resource":
		return plan.FidelityResource, nil
	case "attribute":
		return plan.FidelityAttribute, nil
	default:
		return 0, fmt.Errorf("invalid detail %q (want summary|resource|attribute)", s)
	}
}

func Parse(data []byte) (Ruleset, error) {
	var rs Ruleset
	if err := yaml.Unmarshal(data, &rs); err != nil {
		return rs, err
	}
	for i, r := range rs.Rules {
		for _, v := range []string{r.Create, r.Update, r.Delete, r.Replace, r.Noop} {
			if v != "" {
				if _, err := ParseFidelity(v); err != nil {
					return rs, fmt.Errorf("rule %d (%s): %w", i, r.Path, err)
				}
			}
		}
	}
	return rs, nil
}

func Load(path string) (Ruleset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Ruleset{}, err
	}
	rs, err := Parse(data)
	if err != nil {
		return rs, fmt.Errorf("%s: %w", path, err)
	}
	return rs, nil
}

func actionField(r Rule, a plan.Action) string {
	switch a {
	case plan.ActionCreate:
		return r.Create
	case plan.ActionUpdate:
		return r.Update
	case plan.ActionDelete:
		return r.Delete
	case plan.ActionReplace:
		return r.Replace
	default: // NoOp, Read
		return r.Noop
	}
}

func defaultDetail(a plan.Action) plan.Fidelity {
	if a == plan.ActionNoOp || a == plan.ActionRead {
		return plan.FidelitySummary
	}
	return plan.FidelityResource
}

func match(pattern, path string) bool {
	ok, _ := doublestar.Match(pattern, path)
	return ok
}

func (rs Ruleset) Detail(unitPath string, a plan.Action) plan.Fidelity {
	detail := defaultDetail(a)
	for _, r := range rs.Rules {
		if !match(r.Path, unitPath) {
			continue
		}
		if v := actionField(r, a); v != "" {
			if f, err := ParseFidelity(v); err == nil {
				detail = f
			}
		}
	}
	return detail
}

func (rs Ruleset) Assign(unitPath string) (string, bool) {
	scope, dedicated := "", false
	for _, r := range rs.Rules {
		if r.DedicatedComment && match(r.Path, unitPath) {
			scope, dedicated = r.Scope, true
		}
	}
	return scope, dedicated
}

func (rs Ruleset) DedicatedScopes() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rs.Rules {
		if r.DedicatedComment && r.Scope != "" && !seen[r.Scope] {
			seen[r.Scope] = true
			out = append(out, r.Scope)
		}
	}
	return out
}

func (rs Ruleset) Title(scope string, dedicated bool) string {
	title := "Terragrunt plan"
	for _, r := range rs.Rules {
		if r.Title == "" {
			continue
		}
		if dedicated {
			if r.DedicatedComment && r.Scope == scope {
				title = r.Title
			}
		} else if !r.DedicatedComment {
			title = r.Title
		}
	}
	return title
}

func (rs Ruleset) GroupBy(scope string, dedicated bool) int {
	gb := 1
	for _, r := range rs.Rules {
		if r.GroupBy == nil {
			continue
		}
		if dedicated {
			if r.DedicatedComment && r.Scope == scope {
				gb = *r.GroupBy
			}
		} else if !r.DedicatedComment {
			gb = *r.GroupBy
		}
	}
	return gb
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ruleset/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ruleset/
git commit -m "feat(ruleset): types, YAML load, and per-change detail resolution"
```

---

### Task 2: `internal/ruleset` — GitHub base fetch

**Files:**
- Create: `internal/ruleset/fetch.go`
- Test: `internal/ruleset/fetch_test.go`

**Interfaces:**
- Produces:

```go
type Fetcher struct {
	HTTP   *http.Client
	APIURL string // default https://api.github.com
	Token  string
}

// parseRef splits "owner/repo//path/to/file.yaml@ref".
func parseRef(ref string) (owner, repo, path, gitref string, err error)

// Fetch returns the raw bytes of the referenced file via the contents API.
func (f *Fetcher) Fetch(ctx context.Context, ref string) ([]byte, error)
```

- `parseRef`: split on `//` into `owner/repo` and the remainder; split the remainder on `@` into `path` and optional `gitref`. Error if owner/repo/path missing. `Fetch`: `GET {APIURL}/repos/{owner}/{repo}/contents/{path}?ref={gitref}` (omit `?ref` when empty) with `Accept: application/vnd.github.raw` and `Authorization: Bearer {Token}` (only when Token != ""); non-2xx → error including status + ref.

- [ ] **Step 1: Write the failing test**

Create `internal/ruleset/fetch_test.go`:

```go
package ruleset

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseRef(t *testing.T) {
	owner, repo, path, ref, err := parseRef("org/shared//rules/base.yaml@v1")
	if err != nil || owner != "org" || repo != "shared" || path != "rules/base.yaml" || ref != "v1" {
		t.Fatalf("got %q %q %q %q err=%v", owner, repo, path, ref, err)
	}
	if _, _, _, _, err := parseRef("no-slash-slash"); err == nil {
		t.Fatal("expected error for missing //")
	}
}

func TestFetchRawContents(t *testing.T) {
	var gotPath, gotAccept, gotRef string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccept = r.Header.Get("Accept")
		gotRef = r.URL.Query().Get("ref")
		w.Write([]byte("rules:\n  - path: \"**\"\n"))
	}))
	defer srv.Close()

	f := &Fetcher{HTTP: http.DefaultClient, APIURL: strings.TrimRight(srv.URL, "/"), Token: "tok"}
	data, err := f.Fetch(context.Background(), "org/shared//rules/base.yaml@v1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "rules:") {
		t.Errorf("body = %q", data)
	}
	if gotPath != "/repos/org/shared/contents/rules/base.yaml" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAccept != "application/vnd.github.raw" {
		t.Errorf("accept = %q", gotAccept)
	}
	if gotRef != "v1" {
		t.Errorf("ref = %q", gotRef)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ruleset/ -run 'TestParseRef|TestFetch'`
Expected: FAIL — undefined `parseRef`/`Fetcher`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/ruleset/fetch.go`:

```go
package ruleset

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Fetcher struct {
	HTTP   *http.Client
	APIURL string
	Token  string
}

func parseRef(ref string) (owner, repo, path, gitref string, err error) {
	i := strings.Index(ref, "//")
	if i < 0 {
		return "", "", "", "", fmt.Errorf("invalid base ref %q (want owner/repo//path[@ref])", ref)
	}
	repoPart, rest := ref[:i], ref[i+2:]
	rp := strings.SplitN(repoPart, "/", 2)
	if len(rp) != 2 || rp[0] == "" || rp[1] == "" {
		return "", "", "", "", fmt.Errorf("invalid base ref %q (want owner/repo//path)", ref)
	}
	owner, repo = rp[0], rp[1]
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		path, gitref = rest[:at], rest[at+1:]
	} else {
		path = rest
	}
	if path == "" {
		return "", "", "", "", fmt.Errorf("invalid base ref %q (empty path)", ref)
	}
	return owner, repo, path, gitref, nil
}

func (f *Fetcher) Fetch(ctx context.Context, ref string) ([]byte, error) {
	owner, repo, path, gitref, err := parseRef(ref)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/repos/%s/%s/contents/%s", f.APIURL, owner, repo, path)
	if gitref != "" {
		u += "?ref=" + url.QueryEscape(gitref)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if f.Token != "" {
		req.Header.Set("Authorization", "Bearer "+f.Token)
	}
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("fetch base %q: %s: %s", ref, resp.Status, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ruleset/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ruleset/
git commit -m "feat(ruleset): fetch base ruleset from GitHub via contents API"
```

---

### Task 3: `internal/ruleset` — base merge, recursion, cycle guard, token

**Files:**
- Create: `internal/ruleset/resolve.go`
- Test: `internal/ruleset/resolve_test.go`

**Interfaces:**
- Consumes: `Fetcher`, `Parse`.
- Produces:

```go
// Resolve fetches and merges the base chain: base rules first, local appended, so
// local wins under last-match-wins. Recursive with a cycle guard.
func Resolve(ctx context.Context, rs Ruleset, f *Fetcher) (Ruleset, error)

// DefaultToken resolves a GitHub token: $GITHUB_TOKEN, $GH_TOKEN, then `gh auth token`.
func DefaultToken() string
```

- `Resolve` walks `rs.Base`: if empty, return `rs`. Else `f.Fetch` → `Parse` → recurse (tracking visited refs to detect cycles; max depth 10). Return `Ruleset{Rules: append(resolvedBase.Rules, rs.Rules...)}` (drop the base field on the result). Cycle or depth exceeded → error. `DefaultToken`: env first, else `exec.Command("gh","auth","token")` trimmed (ignore errors → "").

- [ ] **Step 1: Write the failing test**

Create `internal/ruleset/resolve_test.go`:

```go
package ruleset

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolveMergesBaseThenLocal(t *testing.T) {
	// base defines create=summary; local overrides create=attribute (local wins)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("rules:\n  - path: \"**\"\n    create: summary\n    delete: resource\n"))
	}))
	defer srv.Close()

	local, _ := Parse([]byte("base: org/shared//base.yaml@v1\nrules:\n  - path: \"**\"\n    create: attribute\n"))
	f := &Fetcher{HTTP: http.DefaultClient, APIURL: strings.TrimRight(srv.URL, "/")}
	merged, err := Resolve(context.Background(), local, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Rules) != 2 {
		t.Fatalf("merged rules = %d, want 2 (base + local)", len(merged.Rules))
	}
	// base delete=resource still applies; local create=attribute wins over base summary
	if got := merged.Detail("x", plan_ActionCreate); got.String() != "attribute" {
		t.Errorf("create = %v, want attribute (local wins)", got)
	}
	if got := merged.Detail("x", plan_ActionDelete); got.String() != "resource" {
		t.Errorf("delete = %v, want resource (from base)", got)
	}
}

func TestResolveDetectsCycle(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// base points back at itself
		fmt.Fprintf(w, "base: org/shared//loop.yaml@v1\nrules: []\n")
	}))
	defer srv.Close()
	srvURL = srv.URL
	_ = srvURL
	local, _ := Parse([]byte("base: org/shared//loop.yaml@v1\nrules: []\n"))
	f := &Fetcher{HTTP: http.DefaultClient, APIURL: strings.TrimRight(srv.URL, "/")}
	if _, err := Resolve(context.Background(), local, f); err == nil {
		t.Fatal("expected cycle error")
	}
}
```

Note: this test uses `plan_ActionCreate`/`plan_ActionDelete` and `.String()` — add a small test helper file `internal/ruleset/helper_test.go` aliasing `plan.ActionCreate`/`plan.ActionDelete` and ensure `plan.Fidelity` has a `String()` method (add in Task 4 if missing). If `Fidelity.String()` does not exist yet, compare against the `plan.Fidelity` constants instead:

```go
// helper_test.go
package ruleset

import "github.com/dknathalage/gruntcmt/internal/plan"

var plan_ActionCreate = plan.ActionCreate
var plan_ActionDelete = plan.ActionDelete
```

And replace `got.String() != "attribute"` with `got != plan.FidelityAttribute` and `got.String() != "resource"` with `got != plan.FidelityResource` (import plan in the test). Use the constant form to avoid depending on a String() method.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ruleset/ -run TestResolve`
Expected: FAIL — undefined `Resolve`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/ruleset/resolve.go`:

```go
package ruleset

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const maxBaseDepth = 10

func Resolve(ctx context.Context, rs Ruleset, f *Fetcher) (Ruleset, error) {
	return resolve(ctx, rs, f, map[string]bool{}, 0)
}

func resolve(ctx context.Context, rs Ruleset, f *Fetcher, seen map[string]bool, depth int) (Ruleset, error) {
	if rs.Base == "" {
		return Ruleset{Rules: rs.Rules}, nil
	}
	if depth >= maxBaseDepth {
		return Ruleset{}, fmt.Errorf("base chain too deep (>%d)", maxBaseDepth)
	}
	if seen[rs.Base] {
		return Ruleset{}, fmt.Errorf("base cycle detected at %q", rs.Base)
	}
	seen[rs.Base] = true
	data, err := f.Fetch(ctx, rs.Base)
	if err != nil {
		return Ruleset{}, err
	}
	baseRS, err := Parse(data)
	if err != nil {
		return Ruleset{}, fmt.Errorf("base %q: %w", rs.Base, err)
	}
	resolvedBase, err := resolve(ctx, baseRS, f, seen, depth+1)
	if err != nil {
		return Ruleset{}, err
	}
	return Ruleset{Rules: append(resolvedBase.Rules, rs.Rules...)}, nil
}

func DefaultToken() string {
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ruleset/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ruleset/
git commit -m "feat(ruleset): resolve base chain (merge, recursion, cycle guard) + token"
```

---

### Task 4: `plan` — per-change `Detail` field

**Files:**
- Modify: `internal/plan/plan.go` (add field to `ResourceChange`)
- Test: `internal/plan/plan_test.go` (extend)

**Interfaces:**
- Produces: `ResourceChange.Detail plan.Fidelity` (zero value `FidelitySummary`; set by analyze). Domain-only change; no logic.

- [ ] **Step 1: Write the failing test**

Append to `internal/plan/plan_test.go`:

```go
func TestResourceChangeHasDetailField(t *testing.T) {
	c := ResourceChange{Detail: FidelityAttribute}
	if c.Detail != FidelityAttribute {
		t.Fatal("Detail field missing or wrong")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/plan/ -run TestResourceChangeHasDetailField`
Expected: FAIL — unknown field `Detail`.

- [ ] **Step 3: Write minimal implementation**

In `internal/plan/plan.go`, add `Detail Fidelity` to `ResourceChange`:

```go
type ResourceChange struct {
	Address    string
	Action     Action
	Attributes []AttributeChange
	Unchanged  int
	Detail     Fidelity // per-change render fidelity, resolved by analyze
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/plan/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/plan/
git commit -m "feat(plan): add per-change Detail field to ResourceChange"
```

---

### Task 5: `internal/analyze` — partition into per-comment Reports + per-change detail

**Files:**
- Modify: `internal/analyze/analyze.go`
- Test: `internal/analyze/analyze_test.go`

**Interfaces:**
- Consumes: `plan.Unit`, `plan.LoadError`, `ruleset.Ruleset`.
- Produces:

```go
// Report gains Title and GroupBy (per comment).
type Report struct {
	Scope            string
	Title            string
	GroupBy          int
	TerraformVersion string
	Commit           string
	Groups           []Group
	LoadErrors       []plan.LoadError
	Totals           plan.Counts
	Severity         int
}

// Analyze partitions units into one Report per comment (main first, then dedicated
// in DedicatedScopes order), stamps each change's Detail, and groups per comment.
func Analyze(units []plan.Unit, loadErrs []plan.LoadError, rs ruleset.Ruleset, mainScope string) []Report
```

- For each unit: `scope, dedicated := rs.Assign(u.Name)`; bucket key = `scope` if dedicated else `mainScope`. Stamp every `u.Changes[i].Detail = rs.Detail(u.Name, u.Changes[i].Action)`. Build one Report per bucket: `Title=rs.Title(key,dedicated)`, `GroupBy=rs.GroupBy(key,dedicated)`, group units by `groupKey(name, GroupBy)`, counts/severity/sort as before (severity desc, name asc; stable). LoadErrors attach to the main report only. Output order: main report first (even if it has zero units but has load errors; skip if entirely empty), then dedicated reports in `rs.DedicatedScopes()` order (skip empty ones).

- [ ] **Step 1: Write the failing test**

Replace the body of `internal/analyze/analyze_test.go` `import` + helpers to use ruleset, and add:

```go
package analyze

import (
	"testing"

	"github.com/dknathalage/gruntcmt/internal/plan"
	"github.com/dknathalage/gruntcmt/internal/ruleset"
)

func unit(name string, acts ...plan.Action) plan.Unit {
	u := plan.Unit{Name: name}
	for _, a := range acts {
		u.Changes = append(u.Changes, plan.ResourceChange{Action: a})
	}
	u.Counts = plan.Count(u.Changes) // if a Count helper exists; else set manually
	return u
}

func TestAnalyzeSplitsDedicatedComment(t *testing.T) {
	rs, _ := ruleset.Parse([]byte(`
rules:
  - path: "**"
    group-by: 1
    delete: attribute
  - path: "**/security/**"
    dedicated-comment: true
    scope: security
    delete: attribute
`))
	units := []plan.Unit{
		unit("prod/networking", plan.ActionCreate),
		unit("prod/security/iam", plan.ActionDelete),
	}
	reports := Analyze(units, nil, rs, "infra")
	if len(reports) != 2 {
		t.Fatalf("reports = %d, want 2 (main + security)", len(reports))
	}
	if reports[0].Scope != "infra" {
		t.Errorf("main scope = %q", reports[0].Scope)
	}
	if reports[1].Scope != "security" {
		t.Errorf("dedicated scope = %q", reports[1].Scope)
	}
}

func TestAnalyzeStampsPerChangeDetail(t *testing.T) {
	rs, _ := ruleset.Parse([]byte(`
rules:
  - path: "**"
    create: summary
    delete: attribute
`))
	u := unit("prod/db", plan.ActionCreate, plan.ActionDelete)
	reports := Analyze([]plan.Unit{u}, nil, rs, "infra")
	got := reports[0].Groups[0].Units[0].Changes
	if got[0].Detail != plan.FidelitySummary || got[1].Detail != plan.FidelityAttribute {
		t.Fatalf("details = %v,%v", got[0].Detail, got[1].Detail)
	}
}
```

Note: if `plan.Count` does not exist, set `u.Counts` explicitly in the helper. Confirm the existing counts helper name in `internal/plan/plan.go` (`countChanges` is unexported); if so, compute counts in the test helper by hand or export a `plan.Count`. Add `func Count(changes []ResourceChange) Counts { return countChanges(changes) }` to `internal/plan/plan.go` in this task if needed (small, tested via analyze).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/analyze/`
Expected: FAIL — `Analyze` signature mismatch / undefined `ruleset`.

- [ ] **Step 3: Write minimal implementation**

Rewrite `internal/analyze/analyze.go` (`Analyze` now returns `[]Report`, keyed by comment). Keep `groupKey`, `unitSeverity`, `addCounts` helpers. Core:

```go
func Analyze(units []plan.Unit, loadErrs []plan.LoadError, rs ruleset.Ruleset, mainScope string) []Report {
	type bucket struct {
		scope     string
		dedicated bool
		units     []plan.Unit
	}
	order := []string{mainScope}
	buckets := map[string]*bucket{mainScope: {scope: mainScope, dedicated: false}}
	for _, s := range rs.DedicatedScopes() {
		order = append(order, s)
		buckets[s] = &bucket{scope: s, dedicated: true}
	}
	for _, u := range units {
		for i := range u.Changes {
			u.Changes[i].Detail = rs.Detail(u.Name, u.Changes[i].Action)
		}
		scope, dedicated := rs.Assign(u.Name)
		key := mainScope
		if dedicated {
			key = scope
		}
		b := buckets[key]
		if b == nil { // dedicated scope with no rule listed (shouldn't happen) -> main
			b = buckets[mainScope]
		}
		b.units = append(b.units, u)
	}

	var reports []Report
	for _, key := range order {
		b := buckets[key]
		isMain := key == mainScope
		var le []plan.LoadError
		if isMain {
			le = loadErrs
		}
		if len(b.units) == 0 && len(le) == 0 {
			continue
		}
		reports = append(reports, buildReport(b.scope, rs.Title(b.scope, b.dedicated), rs.GroupBy(b.scope, b.dedicated), b.units, le))
	}
	return reports
}
```

Add `buildReport(scope, title string, groupBy int, units []plan.Unit, loadErrs []plan.LoadError) Report` containing the previous grouping/counting/severity/sort logic (adapted from the old single-Report `Analyze`, using the passed `groupBy` and setting `Title`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/analyze/`
Expected: PASS. (Update older analyze tests that called the single-Report `Analyze` to the new signature.)

- [ ] **Step 5: Commit**

```bash
git add internal/analyze/ internal/plan/
git commit -m "feat(analyze): per-comment Reports + per-change detail stamping"
```

---

### Task 6: `internal/render` — per-change detail rendering

**Files:**
- Modify: `internal/render/render.go`
- Test: `internal/render/render_test.go`
- Regenerate: `internal/render/testdata/*.golden`

**Interfaces:**
- Consumes: `analyze.Report`, `plan.*`.
- Produces: `func Render(r analyze.Report) string` (drops the `config.Settings` param). `Marker(scope)` stays.

- Behavior: emoji built-in (🔴/🟡/🟢/➖ — no override). Title/group-by come from `r.Title`/pre-grouped `r.Groups`. Headline includes unit count. Per unit: `renderable := changes with Detail != FidelitySummary`. If `len(renderable) > 0` → unit `<details>` listing each renderable change (resource → address+action glyph; attribute → address + attr lines). Else if the unit has no changes at all (no-op) → a folded `<details><summary><code>NAME</code> — no changes</summary>...</details>`. Else (has changes, all summary) → render nothing for the unit. A group `<details>` renders iff ≥1 of its units rendered something. Unchanged attributes are always omitted (no hidden-count line).

- [ ] **Step 1: Update the golden harness + rewrite sampleReport**

Modify `internal/render/render_test.go`: `Render` now takes only a `Report`; update `sampleReport` to set per-change `Detail` on each change, and drop the `config.Settings` arg from all calls. Add a case that mixes fidelities in one unit (a create at `summary` + a delete at `attribute`) and asserts the create is absent and the delete present. Keep `TestRenderMarkerFirstLine`.

```go
func TestRenderPerChangeDetailMix(t *testing.T) {
	u := plan.Unit{Name: "prod/db", TerraformVersion: "1.9.5",
		Counts:  plan.Counts{Add: 1, Destroy: 1},
		Changes: []plan.ResourceChange{
			{Address: "aws_x.new", Action: plan.ActionCreate, Detail: plan.FidelitySummary},
			{Address: "aws_y.old", Action: plan.ActionDelete, Detail: plan.FidelityAttribute,
				Attributes: []plan.AttributeChange{{Path: "name", Before: "n", Kind: plan.AttrRemove}}},
		}}
	r := analyze.Report{Scope: "s", Title: "T", TerraformVersion: "1.9.5", GroupBy: 1,
		Groups:   []analyze.Group{{Key: "prod", Units: []plan.Unit{u}, Counts: u.Counts, Severity: 5}},
		Totals:   u.Counts, Severity: 5}
	out := Render(r)
	if strings.Contains(out, "aws_x.new") {
		t.Error("summary-detail create should not be listed")
	}
	if !strings.Contains(out, "- aws_y.old") {
		t.Errorf("attribute-detail delete missing:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/`
Expected: FAIL — `Render` arity / behavior.

- [ ] **Step 3: Write minimal implementation**

In `internal/render/render.go`: change `Render(r analyze.Report, s config.Settings)` → `Render(r analyze.Report)`; remove `config` import and all `s.Render.*` usage; replace emoji lookups with built-in constants; in the per-unit loop compute `renderable` by `c.Detail != plan.FidelitySummary` and skip summary changes; for attribute changes call the existing `renderAttr`; drop the `HideUnchanged` count line. Keep the no-op "no changes" one-liner for units with zero changes. Group renders only if it produced any unit body.

- [ ] **Step 4: Regenerate goldens + verify**

Run: `go test ./internal/render/ -update && go test ./internal/render/`
Expected: goldens rewritten (no emoji-override/hide-unchanged cases; per-change detail). Open each `.golden` and confirm marker line 1, headline with unit count, table, and that summary-detail changes are absent from diff bodies.

- [ ] **Step 5: Commit**

```bash
git add internal/render/
git commit -m "feat(render): per-change detail rendering; built-in emoji; drop config dep"
```

---

### Task 7: `cmd/gruntcmt` — ruleset flags, load+resolve, multi-report output; remove config

**Files:**
- Modify: `cmd/gruntcmt/main.go`
- Test: `cmd/gruntcmt/main_test.go`
- Delete: `internal/config/` (whole package)

**Interfaces:**
- Consumes: `ruleset.*`, `analyze.Analyze` (new sig), `render.Render` (new sig), `gh.Client`.
- Produces: updated `run()` with flags `--ruleset --scope --name --input --commit --out --repo --pr --print-ruleset --version`. Loads `--ruleset` (or `./gruntcmt.yaml` if present, else empty `Ruleset{}`), `ruleset.Resolve` with a `Fetcher{Token: ruleset.DefaultToken()}` only when `Base != ""`, analyzes to `[]Report`, then for `--out stdout` writes each `render.Render(rep)` concatenated (blank line between), for `--out gh` posts each `rep` by `rep.Scope`.

- [ ] **Step 1: Write the failing tests**

Rewrite `cmd/gruntcmt/main_test.go` end-to-end tests to the new flags. Keep `TestRunVersion`, `TestResolveVersion*`. Update/replace:

```go
func TestRunEndToEndBarePlanRuleset(t *testing.T) {
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"aws_s3_bucket.b","change":{"actions":["create"]}}]}`
	var out, errBuf bytes.Buffer
	code := run([]string{"--scope", "net", "--name", "networking/s3"}, strings.NewReader(p), &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if !strings.HasPrefix(out.String(), "<!-- gruntcmt:scope=net -->") {
		t.Errorf("missing marker: %q", out.String())
	}
}

func TestRunRejectsRemovedDetailFlag(t *testing.T) {
	const p = `{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[]}`
	var out, errBuf bytes.Buffer
	if code := run([]string{"--scope", "x", "--detail", "attribute"}, strings.NewReader(p), &out, &errBuf); code == 0 {
		t.Fatal("expected non-zero: --detail was removed")
	}
}

func TestRunMultiReportStdout(t *testing.T) {
	dir := t.TempDir()
	rulesetPath := dir + "/gruntcmt.yaml"
	os.WriteFile(rulesetPath, []byte(`
rules:
  - path: "**"
    delete: resource
  - path: "**/security/**"
    dedicated-comment: true
    scope: security
    delete: resource
`), 0o644)
	in := `{"name":"prod/networking","plan":{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"a","change":{"actions":["create"]}}]}}` + "\n" +
		`{"name":"prod/security/iam","plan":{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"b","change":{"actions":["delete"]}}]}}` + "\n"
	var out, errBuf bytes.Buffer
	code := run([]string{"--scope", "infra", "--ruleset", rulesetPath}, strings.NewReader(in), &out, &errBuf)
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "<!-- gruntcmt:scope=infra -->") ||
		!strings.Contains(out.String(), "<!-- gruntcmt:scope=security -->") {
		t.Errorf("expected both main and dedicated markers:\n%s", out.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/gruntcmt/`
Expected: FAIL — old flags / signatures.

- [ ] **Step 3: Rewrite `run()` and delete `internal/config`**

Rewrite `cmd/gruntcmt/main.go`: drop `--config/--detail/--group-by/--no-config` and all `config.*` usage; add `--ruleset`/`--print-ruleset`. Resolution:

```go
// load ruleset
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
	fmt.Fprintf(stderr, "%+v\n", rs)
	return 0
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
			io.WriteString(stdout, "\n")
		}
		io.WriteString(stdout, render.Render(rep))
	}
	return 0
case "gh":
	return postReports(stderr, reports, *repo, *prNum)
default:
	fmt.Fprintln(stderr, "invalid --out (want stdout|gh)")
	return 1
}
```

Add `apiBaseURL()` (reads `GITHUB_API_URL`, defaults `https://api.github.com`) and refactor the existing `postToGitHub` into `postReports(stderr, reports, repo, pr)` that resolves token/repo/pr once and calls `client.UpsertComment` per report using `render.Marker(rep.Scope)` and `render.Render(rep)`; any failure returns 1. Then `rm -rf internal/config`.

- [ ] **Step 4: Run all tests**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS across all packages; `internal/config` gone.

- [ ] **Step 5: Smoke test**

Run:
```bash
go build -o /tmp/gruntcmt ./cmd/gruntcmt
printf '%s\n%s\n' \
  '{"name":"prod/networking","plan":{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"a","change":{"actions":["create"]}}]}}' \
  '{"name":"prod/security/iam","plan":{"format_version":"1.2","terraform_version":"1.9.5","resource_changes":[{"address":"b","change":{"actions":["delete"]}}]}}' \
  | /tmp/gruntcmt --scope infra --ruleset /dev/stdin <<<'' 2>/dev/null || true
```
Expected (with a real ruleset file): two documents (scope=infra, scope=security). Verify by hand with a temp `gruntcmt.yaml`.

- [ ] **Step 6: Commit**

```bash
git add cmd/gruntcmt/ && git rm -r internal/config
git commit -m "feat(cmd): ruleset flags, base resolve, multi-comment output; remove internal/config"
```

---

### Task 8: Rebuild the numbered terragrunt example + workflow

**Files:**
- Delete: `examples/terragrunt/live/{production,staging}`, `plan-to-ndjson.sh`, `demo-changes.sh`, `.gruntcmt.yaml`, `modules/app`
- Create: `examples/terragrunt/gruntcmt.yaml`, `modules/scenario/main.tf`, `live/01-create/ … 06-summary/terragrunt.hcl`, `plan-scenarios.sh`
- Modify: `.github/workflows/pr-demo.yml`, `examples/README.md`, `examples/terragrunt/README.md`

**Interfaces:** none (example + CI). Folded into one task because the pieces are only meaningful together and are verified as a unit.

- [ ] **Step 1: Scenario module + numbered units**

Create `modules/scenario/main.tf` using the `random` provider, driven by inputs `scenario` (string) and `phase` (`baseline`|`changed`, from `get_env("PHASE","baseline")` in `root.hcl`), such that a `terragrunt apply` at `PHASE=baseline` then `plan` at `PHASE=changed` yields, per scenario dir: `01-create` (a create), `02-update` (in-place update incl. a sensitive + known-after-apply attr), `03-replace` (forces-replacement via a `random_id` keeper), `04-destroy` (a destroy), `05-noop` (no change), `06-summary` (a create shown at summary level). Each `live/NN-*/terragrunt.hcl` includes `root.hcl` (via `find_in_parent_folders("root.hcl")`), sources `${get_parent_terragrunt_dir()}/modules//scenario`, and sets `inputs = { scenario = "<kind>" }`.

- [ ] **Step 2: Ruleset at the example root**

Create `examples/terragrunt/gruntcmt.yaml`:

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
  - path: "**/03-replace"
    replace: attribute
  - path: "**/02-update"
    update: attribute
```

- [ ] **Step 3: `plan-scenarios.sh`**

Create `examples/terragrunt/plan-scenarios.sh`: `export TG_TF_PATH=${TG_TF_PATH:-tofu}` (+ `TERRAGRUNT_TFPATH`), apply all units at `PHASE=baseline`, then plan each unit at `PHASE=changed` and emit one wrapped-NDJSON line per unit (name = path under `live/`, cache-dir pruned from the `find`). Mirror the working per-unit plan+show approach from the prior `plan-to-ndjson.sh`.

- [ ] **Step 4: Verify locally end-to-end**

Run (requires OpenTofu + terragrunt, e.g. `mise install` in the dir):
```bash
cd examples/terragrunt
find . -name .terragrunt-cache -prune -exec rm -rf {} + ; find . \( -name '*.tfstate*' -o -name plan.tfplan \) -delete
./plan-scenarios.sh 2>/dev/null | /tmp/gruntcmt --scope scenarios --ruleset gruntcmt.yaml
```
Expected: one comment; a table row per numbered scenario; deletes/replaces at attribute detail, creates counted-only (summary), update at attribute. Confirm the mix renders, then clean the cache/state artifacts again.

- [ ] **Step 5: Update the PR demo workflow + READMEs**

Modify `.github/workflows/pr-demo.yml` Scenario steps to a single step: `./plan-scenarios.sh > s.ndjson` then `"$GRUNTCMT" --scope scenarios --ruleset gruntcmt.yaml --commit "$GITHUB_SHA" --out gh < s.ndjson` and a job-summary write. Update `examples/README.md` and `examples/terragrunt/README.md` to the numbered-scenario + ruleset model.

- [ ] **Step 6: Commit**

```bash
git add examples/ .github/workflows/pr-demo.yml
git commit -m "docs(examples): numbered terragrunt scenarios driven by a ruleset"
```

---

### Task 9: README + docs for the ruleset model

**Files:**
- Modify: `README.md`

**Interfaces:** none (docs). Final task; CLI surface is stable.

- [ ] **Step 1: Rewrite config docs**

Update `README.md`: replace the "Configuration (`.gruntcmt.yaml`)" and Flags sections with the ruleset model — `gruntcmt.yaml`, `--ruleset`, the `rules` array (path × per-action detail, `title`, `group-by`, `dedicated-comment`, `scope`), per-change detail meaning, multi-comment output, and `base:` (GitHub shorthand + default auth). Remove `--config/--detail/--group-by/--no-config` from the flags table; add `--ruleset/--print-ruleset`. Add a short "Migrating from `overrides`" note (old path→detail maps to a `**` rule with per-action detail).

- [ ] **Step 2: Verify**

Run: `go build ./... && go vet ./...`
Expected: clean (docs-only). Cross-check every flag/YAML key named in the README against `cmd/gruntcmt/main.go` and `internal/ruleset/ruleset.go`.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document the ruleset model (rules, per-action detail, dedicated comments, base)"
```

---

## Self-Review

**Spec coverage:**
- Config file name/flag (`gruntcmt.yaml`/`--ruleset`) → Tasks 1,7. ✓
- Ruleset schema (rules, per-rule fields) → Task 1. ✓
- Per-change detail resolution (path × action, last-match, defaults) → Tasks 1,5,6. ✓
- Dedicated comments / multi-document output → Tasks 5 (partition), 6 (render), 7 (emit). ✓
- `base:` fetch (GitHub shorthand, contents API, default auth) → Tasks 2,3. ✓
- Base merge + recursion + cycle guard → Task 3. ✓
- Defaults → Tasks 1 (detail/title/group-by defaults). ✓
- CLI surface changes (removed/renamed flags, `--print-ruleset`) → Task 7. ✓
- Rendering changes (built-in emoji, no hide-unchanged/fold-noop, per-change) → Task 6. ✓
- Error handling (bad ruleset, fetch failure, cycle, out gh) → Tasks 1,3,7. ✓
- Testing strategy → each task's tests. ✓
- Numbered example → Task 8. ✓
- Migration/docs → Task 9. ✓

**Placeholder scan:** No TBD/TODO. Task 5 and Task 3 flag a possible missing `plan.Count`/`Fidelity.String()` and give the concrete fallback (export `Count`, or compare constants) — resolved inline, not deferred.

**Type consistency:** `ruleset.Ruleset/Rule` fields and methods (`Detail`, `Assign`, `DedicatedScopes`, `Title`, `GroupBy`, `Parse`, `Load`, `Fetcher`, `Fetch`, `Resolve`, `DefaultToken`, `parseRef`) are used identically in Tasks 1–3,5,7. `analyze.Analyze(units, loadErrs, rs, mainScope) []Report` and `analyze.Report{Scope,Title,GroupBy,...}` consumed by Tasks 6,7. `render.Render(Report)` + `render.Marker(scope)` consumed by Task 7. `plan.ResourceChange.Detail` (Task 4) consumed by Tasks 5,6. `run()` signature unchanged.

**Executor note:** Task 5 changes `analyze.Analyze`'s signature and Task 6 changes `render.Render`'s — both break existing tests in those packages; each task says to update the older tests to the new signatures. Task 7 deletes `internal/config`; grep for `internal/config` after Task 7 and expect zero references.
