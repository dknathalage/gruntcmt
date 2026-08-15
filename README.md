# gruntcmt

`gruntcmt` is a pure Unix filter that turns Terraform plan JSON — produced by
terragrunt units — into meaningful GitHub-flavored markdown. It reads plan JSON
from stdin, writes markdown to stdout, and does nothing else by default: the only
optional network calls are ruleset `base:` fetch and `--out gh` comment posting.
The markdown targets PR comments and the Actions job summary
(`$GITHUB_STEP_SUMMARY`), and embeds a stable HTML comment marker so CI can
update a comment in place without keeping its own state.

## Install

```bash
go install github.com/dknathalage/gruntcmt/cmd/gruntcmt@latest
```

Ensure your Go bin directory is on `PATH` (e.g. `export PATH="$(go env GOBIN):$(go env GOPATH)/bin:$PATH"`).

### Install with mise

`gruntcmt` installs cleanly via [mise](https://mise.jdx.dev)'s Go backend — mise
provides the Go toolchain and builds the binary, so there is no separate
prerequisite. Pin it per-project alongside your Go version:

```toml
# mise.toml
[tools]
go = "1.24.13"
"go:github.com/dknathalage/gruntcmt/cmd/gruntcmt" = "latest"
```

```bash
mise install                                                  # build + install
mise use -g "go:github.com/dknathalage/gruntcmt/cmd/gruntcmt@latest"  # or globally
mise upgrade gruntcmt                                         # update
```

Pin a specific release by replacing `latest` with a tag (e.g. `v0.1.1`).

## Input Contract

stdin carries Terraform plan JSON in one of two forms, auto-detected (override with
`--input=auto|wrapped|plan`):

**Wrapped NDJSON (multi-unit)** — one JSON object per line:
```json
{"name": "production/networking/vpc", "plan": { <terraform show -json output> }}
{"name": "production/networking/nat", "plan": { <terraform show -json output> }}
```

Produce this trivially with a per-unit loop:

```bash
for unit in production/networking/vpc production/networking/nat; do
  ( cd "$unit" && terragrunt show -json plan.tfplan \
      | jq -c --arg n "$unit" '{name:$n, plan:.}' )
done | gruntcmt --scope production --commit "$GITHUB_SHA" > comment.md
```

**Bare plan (single-unit)** — a single `terraform show -json` document (may be
pretty-printed / multi-line). Unit name comes from `--name` (default `plan`):

```bash
terragrunt show -json plan.tfplan | gruntcmt --name production/networking/vpc
```

**Auto-detection rule:** if the first top-level JSON object contains a
`format_version` key (the Terraform plan marker), the input is treated as a single
bare plan; otherwise it is treated as wrapped NDJSON. Per-unit parse failures are
surfaced in the output as a load-error callout and never abort the run.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--scope` | `""` | Scope label used in the marker and headline. |
| `--name` | `plan` | Unit name for a bare (unwrapped) single plan. |
| `--input` | `""` (auto) | Force input mode: `auto`, `wrapped`, or `plan`. |
| `--commit` | `""` | Commit SHA stamped in the footer for currency. |
| `--ruleset` | `""` | Path to a `gruntcmt.yaml` ruleset file; if omitted, `./gruntcmt.yaml` is used when present. |
| `--print-ruleset` | `false` | Print the resolved ruleset (after base merge) to stderr and exit. |
| `--out` | `stdout` | Output destination: `stdout` (markdown) or `gh` (post/update the PR comment). |
| `--repo` | `""` | `owner/name` for `--out gh` (default `$GITHUB_REPOSITORY`). |
| `--pr` | `0` | PR number for `--out gh` (default: auto-detected in GitHub Actions). |
| `--version` | `false` | Print version and exit. |

**Exit codes:** `0` on success (even when some units had load errors). Non-zero when
stdin is unreadable, the ruleset is invalid, stdin contains no parseable plan at all,
or `--out gh` fails. Plan content (including destroys) never affects the exit code.

### Post directly with `--out gh`

By default gruntcmt is a pure filter (`--out stdout`). Passing `--out gh` makes it
create or update the PR comment in place itself, via the GitHub REST API — no `gh`
CLI, no separate posting step:

```bash
cat plans.ndjson | gruntcmt --scope infra --out gh
```

It needs a token in `$GITHUB_TOKEN` (or `$GH_TOKEN`); the repo and PR number default
to the GitHub Actions environment (`$GITHUB_REPOSITORY`, `$GITHUB_REF`) and can be
set with `--repo`/`--pr` elsewhere. `$GITHUB_API_URL` is honored for GitHub
Enterprise. The comment URL is printed to stderr; with `--out gh`, stdout stays
empty. Update-in-place uses the same `<!-- gruntcmt:scope=... -->` marker, so each
scope owns one comment. This is the only part of gruntcmt that touches the network
(other than `base:` fetch on startup), and only when you opt in.

When a ruleset uses `dedicated-comment: true` rules (see below), `--out gh` posts
each report as its own comment, each with its own marker. `--out stdout` concatenates
them separated by blank lines.

> **Note:** with multiple comments, `--out gh` posts them sequentially and returns non-zero on the first failure, so a mid-run failure can leave earlier comments already posted/updated (not atomic).

## Ruleset (`gruntcmt.yaml`)

The ruleset is an optional YAML file — `gruntcmt.yaml` (non-hidden) in the working
directory, or supplied with `--ruleset`. It controls per-path detail fidelity,
grouping, titles, and dedicated comment splitting. There is no upward directory
search, no global config file, and no environment variable that overrides ruleset
values.

### Rules array

The ruleset contains a `rules` array. Each rule matches on `path` (a doublestar
glob) and sets detail fidelity per change action, plus optional display and routing
settings. Rules are evaluated in order; the **last matching rule** wins for each
field independently.

```yaml
# gruntcmt.yaml
base: ""          # optional: owner/repo//path@ref  (see "Base rulesets" below)

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

All fields are optional. A rule with no `path` matches nothing (the field defaults
to `""`). Only fields explicitly set in a rule participate in last-match resolution;
unset fields do not shadow earlier rules.

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
the main comment. With `--out gh`, each report is posted/updated as its own PR
comment, each with its own `<!-- gruntcmt:scope=<scope> -->` marker. With
`--out stdout`, the reports are concatenated, separated by blank lines.

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
- Running `gruntcmt --out gh` posts two PR comments: one for the main scope, one for `scope=security`.

### Base rulesets

`base:` fetches a shared ruleset from GitHub and merges it under the local rules
(local rules are appended after base rules, so local **last-match wins**):

```yaml
base: owner/repo//path/to/gruntcmt.yaml@main
```

Format: `owner/repo//path[@ref]`. The `@ref` part is optional (defaults to the
repo's default branch). The file is fetched via the GitHub Contents API
(`$GITHUB_API_URL` for GitHub Enterprise).

Authentication uses, in order:
1. `$GITHUB_TOKEN`
2. `$GH_TOKEN`
3. Output of `gh auth token` (if the `gh` CLI is available)

Base rulesets can themselves have a `base:`, allowing chains up to 10 levels deep.
Cycles are detected and cause a hard error.

Inspect the final merged ruleset with:

```bash
gruntcmt --ruleset gruntcmt.yaml --print-ruleset < /dev/null
```

## Composition Models

Both are first-class; `gruntcmt` does not know which model is in use.

**One grouped comment** — funnel all units into one invocation. Output is grouped by
path segment depth (`group-by` in the ruleset), producing one comment with one marker:

```bash
for unit in production/networking/vpc staging/networking/vpc; do
  ( cd "$unit" && terragrunt show -json plan.tfplan \
      | jq -c --arg n "$unit" '{name:$n, plan:.}' )
done | gruntcmt --scope all-envs --ruleset gruntcmt.yaml > comment.md
```

**One comment per scope** — independent pipelines each run their own invocation.
The PR ends up with several comments, each identified by its own marker
(`scope=production`, `scope=staging`, …), each updated in place by the pipeline
that owns it. They never collide:

```bash
# production pipeline
cat production.ndjson | gruntcmt --scope production --ruleset gruntcmt.yaml > prod-comment.md

# staging pipeline (independent CI job)
cat staging.ndjson | gruntcmt --scope staging --ruleset gruntcmt.yaml > staging-comment.md
```

## GitHub Actions

### Reusable action (easiest)

This repo ships a composite action. Give it a wrapped-NDJSON file and a scope; it
installs `gruntcmt`, writes the job summary, and creates/updates the PR comment in
place:

```yaml
permissions:
  contents: read
  pull-requests: write

steps:
  - uses: actions/checkout@v4
  # ... produce plans.ndjson: one {"name","plan"} line per unit ...
  - uses: dknathalage/gruntcmt@v1
    with:
      ndjson: plans.ndjson
      scope: infra
      ruleset: gruntcmt.yaml        # optional; path to your gruntcmt.yaml
      # gruntcmt-version: v0.2.0   # pin the tool (default: latest)
```

See [`examples/`](examples/) for a runnable terragrunt project and a full
[`workflows/terragrunt-plan.yml`](examples/workflows/terragrunt-plan.yml). If you'd
rather wire the steps yourself, the raw recipes follow.

### PR comment — post or update in place

`gruntcmt` always emits `<!-- gruntcmt:scope=<scope> -->` as line 1, serving as a
stable marker. Use `gh api` to post or update the comment using that marker, or pass
`--out gh` to have `gruntcmt` do it directly:

```yaml
- name: Generate plan comment
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    for unit in production/networking/vpc production/networking/nat; do
      ( cd "$unit" && terragrunt show -json plan.tfplan \
          | jq -c --arg n "$unit" '{name:$n, plan:.}' )
    done \
      | gruntcmt --scope production --ruleset gruntcmt.yaml \
                 --commit "$GITHUB_SHA" --out gh
```

Or capture to a file and post manually:

```yaml
- name: Generate plan comment
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    for unit in production/networking/vpc production/networking/nat; do
      ( cd "$unit" && terragrunt show -json plan.tfplan \
          | jq -c --arg n "$unit" '{name:$n, plan:.}' )
    done \
      | gruntcmt --scope production --ruleset gruntcmt.yaml \
                 --commit "$GITHUB_SHA" \
      > /tmp/comment.md

    MARKER='<!-- gruntcmt:scope=production -->'
    PR="${{ github.event.pull_request.number }}"
    ID=$(gh api "repos/$GITHUB_REPOSITORY/issues/$PR/comments" \
          --jq ".[] | select(.body | contains(\"$MARKER\")) | .id" | head -n1)
    if [ -n "$ID" ]; then
      gh api -X PATCH "repos/$GITHUB_REPOSITORY/issues/comments/$ID" \
        -f body="$(cat /tmp/comment.md)"
    else
      gh pr comment "$PR" --body-file /tmp/comment.md
    fi
```

The marker `<!-- gruntcmt:scope=<scope> -->` is what enables update-in-place
detection. Each scope has its own marker, so multiple pipelines posting to the
same PR never collide. Re-runs update the existing comment in place.

### Job summary

The Actions job summary accepts up to 1 MiB per step. Write the output to
`$GITHUB_STEP_SUMMARY` using a ruleset that sets `attribute` detail for the units
you care about:

```yaml
- name: Write job summary
  run: |
    for unit in production/networking/vpc production/networking/nat; do
      ( cd "$unit" && terragrunt show -json plan.tfplan \
          | jq -c --arg n "$unit" '{name:$n, plan:.}' )
    done \
      | gruntcmt --scope production --ruleset gruntcmt.yaml \
                 --commit "$GITHUB_SHA" \
      >> "$GITHUB_STEP_SUMMARY"
```

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

**Not in v1 output:** Resource drift and output changes are parsed into the
analysis model but are not yet rendered in the markdown output; rendering them is
roadmap.

## Migrating from `overrides:`

If you used the old `.gruntcmt.yaml` `overrides:` model, migrate to a `rules` array.
Each `overrides` entry with a `path` + `detail` becomes a rule with per-action detail
fields. The `**` catch-all rule replaces the old global `detail:` setting.

Old:
```yaml
# .gruntcmt.yaml  (OLD — no longer supported)
detail: resource
overrides:
  - path: "**/database/**"
    detail: attribute
  - path: "development/**"
    detail: summary
```

New:
```yaml
# gruntcmt.yaml
rules:
  - path: "**"
    create: resource
    update: resource
    delete: resource
    replace: resource
    noop: summary
  - path: "**/database/**"
    create: attribute
    update: attribute
    delete: attribute
    replace: attribute
  - path: "development/**"
    create: summary
    update: summary
    delete: summary
    replace: summary
```

Also rename the file from `.gruntcmt.yaml` to `gruntcmt.yaml`, and replace any
`--config`/`--detail`/`--group-by`/`--no-config`/`--print-config` flags with
`--ruleset`/`--print-ruleset`.

## Links

- [Scenario mockups](docs/scenarios.html)
- [Design spec](docs/superpowers/specs/2026-08-14-gruntcmt-design.md)
