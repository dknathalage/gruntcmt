# gruntcmt CLI simplification — design

Date: 2026-08-20

## Goal

Turn gruntcmt into a small, self-contained CLI that takes terragrunt plan
JSON, renders a Markdown summary, and (by default) upserts it as a pull-request
comment. Strip everything that made it feel like a GitHub Action or that
required manual wiring. `gruntcmt <dir>` should "just work" both locally and in
CI with no flags.

Driving use case:

```sh
terragrunt run --all plan --json-out-dir out
gruntcmt out
```

## Summary of changes

1. **Single config file, no `base`.** One optional `gruntcmt.yaml`; no remote
   base fetch/merge.
2. **Plan files/dirs as arguments, no stdin.** Recursively discover
   `tfplan.json` under a directory; unit path derived from the tree.
3. **Output destinations.** `--out` defaults to a PR comment; `summary` and a
   file path are the alternatives.
4. **Auto-detection that works locally and in CI.** repo, PR, commit, and token
   resolved from env vars (CI) or `git`/`gh` (local), uniformly.
5. **Sensible built-in defaults.** A default ruleset when no config is present.
6. **Pure CLI.** Remove `action.yml` and all GitHub Action packaging/examples.

## CLI surface (after)

```
gruntcmt [--config <path>] [--print-config] [--out summary|<path>] <plan-path>...
```

Flags kept: `--config`, `--print-config`, `--out`, `--version`.

Flags removed: `--ruleset`/`--print-ruleset` (renamed to `--config`/
`--print-config`), `--scope`, `--name`, `--input`, `--commit`, `--repo`,
`--pr`.

`stdin` is no longer read.

## 1. Config: single file, no base

- `ruleset.Ruleset` loses its `Base` field; it becomes `{ Rules []Rule }`.
- Delete `internal/ruleset/resolve.go` (`Resolve`, `resolve`) and
  `internal/ruleset/fetch.go` (`Fetcher`, `parseRef`), plus `resolve_test.go`
  and `fetch_test.go`. The base chain, cycle detection, depth limit, and GitHub
  Contents API fetch all go away.
- `DefaultToken` (currently in resolve.go) is **relocated**, not deleted — token
  resolution is still needed for commenting. It moves to the detection layer
  (see §4) with the same precedence: `$GITHUB_TOKEN` → `$GH_TOKEN` →
  `gh auth token`.
- Config source order in `main.go`:
  1. `--config <path>` if given.
  2. else `./gruntcmt.yaml` if it exists.
  3. else the built-in `ruleset.Default()` (see §5).

## 2. Input: plan files / directories, no stdin

New `internal/input` API:

```go
func Read(paths []string) ([]plan.Unit, []plan.LoadError, error)
```

- Remove the `Mode` enum, the `wrapped` NDJSON struct, auto-detection, and all
  stdin handling. Each plan file is a bare `terraform show -json` document.
- For each `path` argument:
  - **Directory** → walk recursively (`filepath.WalkDir`, any depth), collecting
    every file named `tfplan.json` (terragrunt's fixed name for
    `--json-out-dir`). Unit name = the file's parent directory relative to the
    given directory root: `filepath.Rel(path, filepath.Dir(file))`.
    e.g. `out/aws/prod/networking/tfplan.json` → unit `aws/prod/networking`.
  - **File** → a single plan. Unit name = the path with a `.json` suffix
    stripped; if the basename is `tfplan.json`, use its parent directory instead,
    so the explicit-file and directory forms agree.
- Errors:
  - No path arguments → error (`no plan files given`).
  - A path that does not exist / cannot be stat'd → error.
  - A file that cannot be read or parsed → recorded as a `plan.LoadError` keyed
    by that unit name; processing continues (isolated failure).
  - Zero units and zero load errors discovered across all paths → error
    (`no plans found`).
- Extract pure, unit-testable helpers, e.g. `unitName(root, file string) string`.

## 3. Output: `--out gh | summary | <path>`

- Default (`--out` unset) → **PR comment** (the `gh` path).
- `--out summary` → append the rendered Markdown to the file named by
  `$GITHUB_STEP_SUMMARY`; error if that variable is unset.
- `--out <anything else>` → treat the value as a file path and write the rendered
  Markdown to it (truncate/create). `--out /dev/stdout` is how you print locally.
- Multi-report handling:
  - `gh` posts/updates one comment per report (keyed by `render.Marker(scope)`).
  - stdout/file/summary concatenate reports with a blank line between them via a
    shared helper `renderAll(reports []analyze.Report) string`.

## 4. Auto-detection (uniform local + CI)

A small detection layer (new `internal/ghctx` package, or unexported helpers in
`main`) resolves everything the `gh` output path needs. Each value prefers the
CI environment variable and falls back to a local `git`/`gh` invocation, so the
behavior is identical in both environments:

| Value  | CI source                          | Local fallback                          |
|--------|------------------------------------|-----------------------------------------|
| repo   | `$GITHUB_REPOSITORY`               | parse `git remote get-url origin` → `owner/name` |
| PR     | `$GITHUB_REF` (`refs/pull/N/…`) or `$GITHUB_EVENT_PATH` payload | `gh pr view --json number -q .number` for the current branch |
| commit | `$GITHUB_SHA`                      | `git rev-parse HEAD`                    |
| token  | `$GITHUB_TOKEN` / `$GH_TOKEN`      | `gh auth token`                         |

- **scope** is auto-derived from the input path: the cleaned basename of the
  first path argument (`gruntcmt envs/prod` → scope `prod`). Falls back to
  `plan` when the basename is empty/`.`/`/`. Distinct directories therefore
  produce distinct, independently-updated comments. Config-defined
  dedicated-comment scopes continue to split out their own comments on top of
  this main scope.
- **commit** is always stamped into the footer (render already supports
  `Report.Commit`; we now always populate it).
- The `gh` output path errors clearly if repo/PR/token cannot be resolved (this
  is the accepted trade-off of defaulting to a PR comment; use `--out /dev/stdout`
  when there is no PR context).
- `apiBaseURL()` still honors `$GITHUB_API_URL` (GitHub Enterprise), unchanged.

The detection helpers shell out; they are only invoked on the `gh` path, so the
core pipeline (input → analyze → render → file/summary/stdout) stays free of
`git`/`gh` and is fully unit-testable.

## 5. Built-in default ruleset

`ruleset.Default()` returns the ruleset used when no config file is found:

- `group-by: 0` — a single flat group (no path-segment grouping).
- One `**` rule with per-action detail:
  - `create`, `update`, `delete`, `replace` → `attribute` (full field diffs).
  - `noop`, `read` → `summary`.
- `title: "Terragrunt plan"`.

## 6. Remove the GitHub Action

- Delete `action.yml`.
- Delete the Action-oriented example workflows: `.github/workflows/pr-demo.yml`
  and `examples/workflows/terragrunt-plan.yml` (and the `examples/workflows/`
  directory if it becomes empty). Keep `.github/workflows/release-please.yml`
  (repo release automation, unrelated to the Action).
- `examples/terragrunt/`: delete `base.yaml`; inline its `**` rule into
  `gruntcmt.yaml` (drop the `base:` line); update `README.md` and
  `plan-scenarios.sh` to the `terragrunt run --all plan --json-out-dir out` →
  `gruntcmt out` flow (no piping, no `--ruleset`/`--scope`).
- `README.md`: remove Action usage/marketplace framing; document the CLI:
  install, `gruntcmt <dir>`, config file, `--out`, and how detection works in CI
  vs locally.

## Testing

- `internal/input`: table tests over temp directories — nested `tfplan.json`
  discovery at multiple depths, unit-name derivation, single-file form, bad/
  unparseable plan → isolated `LoadError`, empty/no-plans → error.
- `internal/ruleset`: `Default()` yields attribute detail for destructive/
  create/update and summary for noop; `Parse`/`Load` unchanged; delete base
  tests.
- `cmd/gruntcmt`: end-to-end `run` writing to a temp file via `--out <path>`
  and via `--out /dev/stdout`; `--out summary` appending to a temp
  `$GITHUB_STEP_SUMMARY`; scope derived from the input dir; `--print-config`;
  no-args error; unknown-flag/removed-flag rejection. The `gh` default path is
  not exercised end-to-end in unit tests (needs network); its detection helpers
  are tested where pure (env-var parsing, ref parsing, `owner/name` parsing).
- `go build ./...` and `go test ./...` must pass.

## Out of scope / non-goals

- No config discovery walking up parent directories, no global config, no env
  overrides of ruleset values (unchanged from today).
- No new output formats beyond gh/summary/file.
- No retention of the wrapped-NDJSON input format.

## Breaking changes

- The GitHub Action interface is removed entirely.
- CLI flags `--ruleset`, `--scope`, `--name`, `--input`, `--commit`, `--repo`,
  `--pr` are removed; input is positional paths, not stdin.
- Ruleset `base:` is no longer supported.
