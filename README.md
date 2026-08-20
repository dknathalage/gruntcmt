# gruntcmt

<!-- x-release-please-start-version -->
Current release: **v0.5.0**
<!-- x-release-please-end-version -->

`gruntcmt` is a small CLI that turns Terraform plan JSON — produced by terragrunt
units — into meaningful GitHub-flavored markdown, and by default upserts it as a
pull-request comment. Point it at a directory of plans and it discovers each unit,
renders a grouped summary, and posts (or updates) the comment in place. It works
the same locally and in CI: the repo, PR, commit, and token are auto-detected from
the environment (GitHub Actions) or from local `git`/`gh`.

```bash
terragrunt run --all plan --json-out-dir out
gruntcmt out
```

## Install

<!-- x-release-please-start-version -->
```bash
go install github.com/dknathalage/gruntcmt/cmd/gruntcmt@v0.5.0
```
<!-- x-release-please-end-version -->

Or `@latest` for the newest release. Ensure your Go bin directory is on `PATH` (e.g. `export PATH="$(go env GOBIN):$(go env GOPATH)/bin:$PATH"`).

## Usage

```
gruntcmt [--config <path>] [--print-config] [--out summary|<path>] <plan-path>...
```

Each positional argument is a plan file or a directory:

- **Directory** — walked recursively for terragrunt's `tfplan.json` files (the name
  terragrunt writes under `--json-out-dir`). Each unit is named by its directory
  relative to the argument: `out/aws/prod/networking/tfplan.json` → unit
  `aws/prod/networking`. Grouping and ruleset matching use that name.
- **File** — a single `terraform show -json` document. The unit is named by the file
  path with a `.json` suffix stripped.

A plan that fails to parse becomes an isolated load-error callout in the output; it
never aborts the run.

### Typical flow

```bash
# Plan all units into a JSON plan directory, then summarize.
terragrunt run --all plan --json-out-dir out
gruntcmt out
```

In CI (GitHub Actions) that comments the PR. Locally, either authenticate with
`gh auth login` so the same command can comment, or print instead:

```bash
gruntcmt --out /dev/stdout out
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | `""` | Path to a `gruntcmt.yaml` ruleset. If omitted, `./gruntcmt.yaml` is used when present, else the built-in default. |
| `--print-config` | `false` | Print the resolved ruleset to stderr and exit. |
| `--out` | `""` (PR comment) | Output destination: empty = comment the PR; `summary` = append to `$GITHUB_STEP_SUMMARY`; any other value = write that file. `--out /dev/stdout` (or `-`) prints to stdout. |
| `--version` | `false` | Print version and exit. |

Everything else is automatic:

| Value | CI source | Local fallback |
|-------|-----------|----------------|
| **scope** (comment marker + label) | basename of the first path argument | same |
| **repo** | `$GITHUB_REPOSITORY` | `git remote get-url origin` |
| **PR** | `$GITHUB_REF` / event payload | `gh pr view --json number` |
| **commit** (footer) | `$GITHUB_SHA` | `git rev-parse HEAD` |
| **token** | `$GITHUB_TOKEN` / `$GH_TOKEN` | `gh auth token` |

`$GITHUB_API_URL` is honored for GitHub Enterprise.

**Exit codes:** `0` on success (even when some units had load errors). Non-zero when
no plan paths are given, no plans are found, the ruleset is invalid, or the selected
output destination fails. Plan content (including destroys) never affects the exit
code.

### Output destinations

- **PR comment (default)** — creates or updates the comment in place via the GitHub
  REST API. Each scope owns one comment, keyed by its `<!-- gruntcmt:scope=... -->`
  marker, so re-runs update rather than duplicate. When a ruleset defines
  `dedicated-comment` rules, each report is posted as its own comment. The comment
  URL is printed to stderr.

  > With multiple comments, posting is sequential and returns non-zero on the first
  > failure, so a mid-run failure can leave earlier comments already updated (not atomic).

  Needs a resolvable repo, PR, and token. When there is no PR context (a plain local
  run), use `--out /dev/stdout` or `--out <file>` instead.

- **`--out summary`** — appends the markdown to `$GITHUB_STEP_SUMMARY` (errors if unset).

- **`--out <file>`** — writes the markdown to that path. Multiple reports are
  concatenated, separated by a blank line.

## Ruleset (`gruntcmt.yaml`)

The ruleset is an optional YAML file — `gruntcmt.yaml` in the working directory, or
supplied with `--config`. It controls per-path detail fidelity, grouping, titles,
and dedicated-comment splitting. There is no upward directory search, no global
config file, and no environment variable that overrides ruleset values. When no file
is found, gruntcmt uses a built-in default (see below).

### Built-in default

With no config file present, gruntcmt applies:

- `group-by: 0` — a single flat group.
- `create`/`update`/`delete`/`replace` → `attribute` (full field diffs).
- `noop`/`read` → `summary`.
- title `"Terragrunt plan"`.

Inspect it (or any resolved config) with:

```bash
gruntcmt --print-config
```

### Rules array

The ruleset contains a `rules` array. Each rule matches on `path` (a doublestar
glob) and sets detail fidelity per change action, plus optional display and routing
settings. Rules are evaluated in order; the **last matching rule** wins for each
field independently.

```yaml
# gruntcmt.yaml
rules:
  - path: "**"           # doublestar glob on the unit's /‑delimited name; ** = match all
    title: "Terraform plan"          # comment/section headline (string)
    group-by: 1                      # leading path segments to group by (int; 0 = flat)
    dedicated-comment: false         # pull matching units into their own comment
    scope: ""                        # scope/marker key for the dedicated comment
    create: resource                 # detail for create actions:  summary|resource|attribute
    update: resource                 # detail for update actions
    delete: resource                 # detail for delete actions
    replace: resource                # detail for replace actions
    noop: summary                    # detail for no-op actions
```

All fields are optional. A rule with no `path` matches nothing. Only fields
explicitly set in a rule participate in last-match resolution; unset fields do not
shadow earlier rules.

### Per-action detail resolution

Detail fidelity is resolved **per resource change** (not per unit), using:

1. Start from the built-in default for the change's action:
   - `create`, `update`, `delete`, `replace` → `resource`
   - `noop` (and read) → `summary`
2. Walk every rule in order. For each rule whose `path` glob matches the **unit's
   path** AND that rule sets the field for this change's action (e.g. `update:`), the
   detail is updated to that value.
3. The **last** such match wins.

Detail level meanings:

| Level | Output |
|-------|--------|
| `summary` | Resource is counted in the summary table but not listed individually. |
| `resource` | Resource address and action appear in the unit's collapsible section. |
| `attribute` | Full before→after attribute diff is included (forces-replacement, sensitive, known-after-apply). |

### Dedicated comments

A rule with `dedicated-comment: true` pulls all units whose path matches into a
separate comment, identified by the rule's `scope` (and its own `title`/`group-by`
settings). Units that do not match any dedicated rule go into the main comment.

One `gruntcmt` invocation can emit multiple reports — one per dedicated scope plus
the main comment. Each is posted/updated as its own PR comment with its own
`<!-- gruntcmt:scope=<scope> -->` marker; to a file/summary they are concatenated,
separated by blank lines.

### Example ruleset

This is adapted from [`examples/terragrunt/gruntcmt.yaml`](examples/terragrunt/gruntcmt.yaml):

```yaml
# gruntcmt.yaml
rules:
  - path: "**"
    title: "Terragrunt plan"
    group-by: 1
    create: summary
    update: resource
    delete: attribute
    replace: attribute
    noop: summary

  - path: "**/security"
    dedicated-comment: true
    scope: security
    title: "Security plan"
    create: attribute
    delete: attribute
```

With this ruleset:
- All units default to the `**` rule: updates shown at resource level, deletes/replaces at attribute level, creates/noops counted only.
- Units under any `security/` directory are pulled into a dedicated comment with scope `security` and full attribute diff for creates and deletes.
- Running `gruntcmt out` posts two PR comments: one for the main scope, one for `scope=security`.

## Output Format

Output is GitHub-flavored markdown. Top to bottom:

1. **Marker** — `<!-- gruntcmt:scope=<scope> -->` (always present; hidden in
   rendered GitHub views).
2. **Headline** — severity emoji, title, scope, unit count, destroy/add/change
   totals.
3. **Summary table** — one row per group: Group | Units | Add | Change | Destroy.
   Always shown at every fidelity.
4. **Destructive callout** — highlighted when any destroys exist.
5. **Grouped details** (`resource`/`attribute`) — one collapsible `<details>` per
   group, nesting one `<details>` per unit with a diff-fenced body.
6. **Load-error callout** — when any unit failed to parse.
7. **Footer** — `gruntcmt · terraform <version> · commit <sha>`.

**Not in output:** Resource drift and output changes are parsed into the analysis
model but are not yet rendered in the markdown output; rendering them is roadmap.

## Links

- [Scenario mockups](docs/scenarios.html)
- [Example project](examples/terragrunt/)
