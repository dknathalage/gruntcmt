# gruntcmt

`gruntcmt` is a pure Unix filter that turns Terraform plan JSON — produced by
terragrunt units — into a single, meaningful chunk of GitHub-flavored markdown on
stdout. It reads plan JSON from stdin, writes markdown to stdout, and does nothing
else: no network calls, no state, no invocations of terraform or terragrunt. The
markdown targets two GitHub surfaces — PR comments and the Actions job summary
(`$GITHUB_STEP_SUMMARY`) — with independently chosen fidelity levels, and embeds a
stable HTML comment marker so CI can update a comment in place without keeping its
own state.

## Install

```bash
go install github.com/dknathalage/gruntcmt/cmd/gruntcmt@latest
```

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
done | gruntcmt --scope production --detail resource > comment.md
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

## Composition Models

Both are first-class; `gruntcmt` does not know which model is in use.

**One grouped comment** — funnel all units into one invocation. Output is grouped by
path segment depth (`--group-by`), producing one comment with one marker:

```bash
for unit in production/networking/vpc staging/networking/vpc; do
  ( cd "$unit" && terragrunt show -json plan.tfplan \
      | jq -c --arg n "$unit" '{name:$n, plan:.}' )
done | gruntcmt --scope all-envs --detail resource > comment.md
```

**One comment per scope** — independent pipelines each run their own invocation.
The PR ends up with several comments, each identified by its own marker
(`scope=production`, `scope=staging`, …), each updated in place by the pipeline
that owns it. They never collide:

```bash
# production pipeline
cat production.ndjson | gruntcmt --scope production --detail resource > prod-comment.md

# staging pipeline (independent CI job)
cat staging.ndjson | gruntcmt --scope staging --detail resource > staging-comment.md
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--scope` | `""` | Scope label used in the marker and headline. |
| `--name` | `plan` | Unit name for a bare (unwrapped) single plan. |
| `--group-by` | `1` | Leading path segments to group by (0 = flat). |
| `--detail` | `""` (config or `resource`) | Fidelity: `summary`, `resource`, or `attribute`. Overrides config globally. |
| `--input` | `""` (auto) | Force input mode: `auto`, `wrapped`, or `plan`. |
| `--commit` | `""` | Commit SHA stamped in the footer for currency. |
| `--config` | `""` | Explicit config file; skips discovery. |
| `--no-config` | `false` | Ignore all config files (reproducible CI). |
| `--print-config` | `false` | Print resolved config to stderr and exit. |
| `--version` | `false` | Print version and exit. |

**Exit codes:** `0` on success (even when some units had load errors). Non-zero when
stdin is unreadable, config is invalid, or stdin contains no parseable plan at all.
Plan content (including destroys) never affects the exit code.

## Fidelity Levels (`--detail`)

One uniform knob applied to every resource:

| Level | Output |
|-------|--------|
| `summary` | Headline + counts table only. Smallest; fits tight comment budgets. |
| `resource` | Adds per-unit collapsibles listing resource addresses and actions. **Default.** |
| `attribute` | Adds a reconstructed per-attribute diff (before → after, forces-replacement, sensitive, known-after-apply). |

Per-path overrides in config may raise or lower fidelity per unit. An explicit
`--detail` flag is a global hammer that overrides config and per-path overrides.

## Grouping (`--group-by N`)

`N` is the number of leading `/`-delimited path segments that form a group key;
deeper segments flatten into the unit's leaf label. Grouping is a single level
(flat groups, flat units within).

Given units `production/database/primary`, `production/database/replica`,
`production/networking`, `staging/database/db1`:

| Flag | Groups produced |
|------|----------------|
| `--group-by 0` | No groups — one flat list by full path. |
| `--group-by 1` | `production` (3 units), `staging` (1 unit). Default. |
| `--group-by 2` | `production/database` (primary, replica), `production/networking` (1), `staging/database` (1). |

A unit with fewer path segments than `N` groups on its whole path as a singleton.

## Configuration (`.gruntcmt.yaml`)

Settings resolve from four layers, highest precedence first; layers merge
key-by-key:

1. CLI flags — always win.
2. `--config <path>` — explicit file, skips discovery.
3. Repo config — nearest `.gruntcmt.yaml` found by walking up from the current
   directory to the repo root (`.git`) or filesystem root.
4. Global config — `$XDG_CONFIG_HOME/gruntcmt/config.yaml`
   (typically `~/.config/gruntcmt/config.yaml`).

`--no-config` skips all four layers (useful for reproducible CI runs).
`--print-config` prints the fully resolved configuration to stderr and exits.

**YAML schema** (all keys are optional):

```yaml
# .gruntcmt.yaml
group-by: 1                  # leading path segments for grouping (int)
detail: resource             # default fidelity: summary | resource | attribute
input: auto                  # input mode: auto | wrapped | plan

render:
  title: "Terragrunt plan"   # headline title string
  emoji:                     # override severity glyphs (map; replaced wholesale)
    destroy: "🔴"
    change:  "🟡"
    add:     "🟢"
    noop:    "➖"
  hide-unchanged: true       # collapse unchanged attributes to a count (attribute fidelity)
  fold-noop: true            # fold no-op units/groups into collapsed sections

overrides:                   # per-path behavior; last matching entry wins
  - path: "**/database/**"   # glob on the /‑delimited unit path
    detail: attribute
  - path: "production/**"
    detail: attribute
  - path: "development/**"
    detail: summary
```

**Per-path overrides:** each entry matches the unit path with a glob (`*`/`**`).
When multiple entries match, the last one in file order wins. Plan-wide settings
(`group-by`, `input`) are global only and cannot be overridden per path.

**Merge semantics:** `hide-unchanged` and `fold-noop` are additive — once any layer
sets them `true` they stay `true` (no lower layer can unset them). The `emoji` map
is replaced wholesale by the last layer that sets it, not merged key-by-key.
`overrides` are replaced wholesale by the innermost layer that sets them.

## GitHub Actions

### PR comment — post or update in place

`gruntcmt` always emits `<!-- gruntcmt:scope=<scope> -->` as line 1. Use `gh` to
post or update the comment using that marker:

```yaml
- name: Generate plan comment
  env:
    GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    for unit in production/networking/vpc production/networking/nat; do
      ( cd "$unit" && terragrunt show -json plan.tfplan \
          | jq -c --arg n "$unit" '{name:$n, plan:.}' )
    done \
      | gruntcmt --scope production --detail resource --commit "$GITHUB_SHA" \
      > /tmp/comment.md

    gh pr comment "${{ github.event.pull_request.number }}" \
      --edit-last-if-matches '<!-- gruntcmt:scope=production -->' \
      --body-file /tmp/comment.md
```

Each scope has its own marker, so multiple pipelines posting to the same PR never
collide. Re-runs update the existing comment in place.

### Job summary — full attribute diff

The Actions job summary accepts up to 1 MiB per step; use `--detail attribute` here
for the full diff without affecting the PR comment size:

```yaml
- name: Write job summary
  run: |
    for unit in production/networking/vpc production/networking/nat; do
      ( cd "$unit" && terragrunt show -json plan.tfplan \
          | jq -c --arg n "$unit" '{name:$n, plan:.}' )
    done \
      | gruntcmt --scope production --detail attribute --commit "$GITHUB_SHA" \
      >> "$GITHUB_STEP_SUMMARY"
```

**Two-surface pattern:** run `gruntcmt` twice — lean `--detail resource` for the PR
comment, full `--detail attribute` for the job summary. `gruntcmt` never truncates;
picking a fidelity that fits a surface's size limit is the operator's decision.

## Output Format

Output is GitHub-flavored markdown. Top to bottom:

1. **Marker** — `<!-- gruntcmt:scope=<scope> -->` (always present; hidden in
   rendered GitHub views).
2. **Headline** — severity emoji, title, scope, unit count, destroy/add/change
   totals.
3. **Summary table** — one row per group: Group | Units | Add | Change | Destroy |
   status. Always shown at every fidelity.
4. **Destructive callout** — highlighted when any destroys exist.
5. **Grouped details** (`resource`/`attribute`) — one collapsible `<details>` per
   group, nesting one `<details>` per unit with a diff-fenced body.
6. **Drift / output-change callouts** — when `resource_drift` or `output_changes`
   are present.
7. **Load-error callout** — when any unit failed to parse.
8. **Footer** — `gruntcmt · terraform <version> · commit <sha>`.

## Links

- [Scenario mockups](docs/scenarios.html)
- [Design spec](docs/superpowers/specs/2026-08-14-gruntcmt-design.md)
