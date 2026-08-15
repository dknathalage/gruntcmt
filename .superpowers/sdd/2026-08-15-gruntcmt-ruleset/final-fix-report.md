# Final Fix Report — gruntcmt v0.4.0 ruleset branch
Date: 2026-08-15

## Changes per Finding

### Finding 1 — `--print-ruleset` prints pointer addresses
**File:** `cmd/gruntcmt/main.go`
Added `gopkg.in/yaml.v3` import. Replaced `fmt.Fprintf(stderr, "%+v\n", rs)` with `yaml.Marshal(rs)` and write resulting bytes to stderr. Output now shows `group-by: 1` not `0x...`.

### Finding 2 — Dedicated scope colliding with mainScope duplicates report
**File:** `internal/analyze/analyze.go`
In `Analyze`, before appending a dedicated scope to `order`/`buckets`, check if that scope key already exists in `buckets`. If it does (collision with mainScope or duplicate dedicated scope), skip adding it. Units assigned to that scope still route into the existing bucket via `rs.Assign` → `key=scope` → `buckets[key]` (which is the main bucket). No duplicate report, no silent comment loss.

### Finding 3 — Dedicated rule with empty scope must be parse error
**File:** `internal/ruleset/ruleset.go`
In `Parse`, added validation before the fidelity check loop: if `r.DedicatedComment && r.Scope == ""`, return error `rule N (path): dedicated-comment requires a non-empty scope`.

### Finding 4 — Null attribute values render as blank
**File:** `internal/plan/attr.go`
In `renderVal`, added check before string-unmarshal: `if string(raw) == "null" { return "(null)" }`. JSON null no longer renders as empty string.

### Finding 5 — Known-after-apply attrs under CREATE use `~` glyph
**File:** `internal/plan/attr.go`
In `diffAttrs`, `isUnknown` case now sets `Kind: AttrAdd` when `!hasB` (no before value, i.e. CREATE), `Kind: AttrUpdate` when `hasB` (UPDATE). Glyph changes from `~` to `+` for create-context unknowns.

### Finding 6 — README: `--out gh` not atomic
**File:** `README.md`
Added one-line blockquote note after the dedicated-comment `--out gh` paragraph: "with multiple comments, `--out gh` posts them sequentially and returns non-zero on the first failure, so a mid-run failure can leave earlier comments already posted/updated (not atomic)."

## Tests Added

- `internal/analyze/analyze_test.go`: `TestAnalyzeDedupesScopeCollision` — ruleset with dedicated rule whose scope equals mainScope `"infra"`; asserts exactly 1 report and both units present.
- `internal/ruleset/ruleset_test.go`: `TestParseRejectsDedicatedWithoutScope` — `dedicated-comment: true` + no `scope` → Parse returns error.
- `internal/plan/attr_test.go`: 
  - `TestRenderValNull` — `renderVal("null")` == `"(null)"`.
  - `TestDiffAttrsNullValues` — null→null unchanged; null→"prod" yields Before=`"(null)"`.
  - `TestDiffAttrsUnknownNoBeforeIsAttrAdd` — unknown attr with no before → `Kind==AttrAdd`.
  - `TestDiffAttrsUnknownWithBeforeIsAttrUpdate` — unknown attr with before → `Kind==AttrUpdate`.

## Golden Files Changed
None. The existing render test fixtures (`sampleReport`, `extrasReport`, `noopUnitReport`) do not use null attribute values or unknown attrs without before values. The `-update` run confirmed no content changes. All goldens timestamps updated but content identical.

## Whole-repo `go test ./...` Output
```
ok  github.com/dknathalage/gruntcmt/cmd/gruntcmt
ok  github.com/dknathalage/gruntcmt/internal/analyze
ok  github.com/dknathalage/gruntcmt/internal/gh
ok  github.com/dknathalage/gruntcmt/internal/input
ok  github.com/dknathalage/gruntcmt/internal/plan
ok  github.com/dknathalage/gruntcmt/internal/render
ok  github.com/dknathalage/gruntcmt/internal/ruleset
```
All green. `go vet ./...` and `gofmt -l cmd/ internal/` both clean.

## `--print-ruleset` Smoke Output
```
$ echo '' | /tmp/gruntcmt --ruleset examples/terragrunt/gruntcmt.yaml --print-ruleset 2>&1 | head -20
base: ""
rules:
    - path: '**'
      title: gruntcmt scenarios
      group-by: 1
      dedicated-comment: false
      scope: ""
      create: summary
      update: resource
      delete: attribute
      replace: attribute
      noop: summary
    - path: '**/06-security'
      title: Security plan
      group-by: null
      dedicated-comment: true
      scope: security
      create: attribute
      update: ""
      delete: attribute
```
YAML output with readable `group-by: 1` (no pointer addresses).

## Concerns
- `group-by: null` appears for rules that don't set group-by (pointer is nil). This is correct YAML representation of the omitted `*int` field — callers fall back to the default (1) via `GroupBy()`. No action needed.
- The `DedicatedScopes()` method already skips rules with empty scope, so the Finding 3 parse error is purely a defensive early catch at parse time; existing runtime behavior was safe.
- No network-dependent smoke test was run (no token in env). The local ruleset file test is sufficient.
