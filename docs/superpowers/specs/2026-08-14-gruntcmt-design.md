# gruntcmt — Design

**Date:** 2026-08-14
**Status:** Approved for planning

## Summary

`gruntcmt` is a small Go CLI that turns Terraform plan JSON — produced by
terragrunt units — into a single, meaningful GitHub pull-request comment written
to stdout. It is a pure Unix filter: it reads plan JSON from stdin, writes
GitHub-flavored markdown to stdout, and does nothing else. It never invokes
terragrunt/terraform, makes no network calls, and holds no state.

There is no interactive/TUI mode and no direct GitHub posting. Delivering and
updating the comment on the PR is the caller's job (CI + `gh`), aided by a stable
marker gruntcmt embeds in the output.

## Goals

- Read Terraform plan JSON for one or more terragrunt units from stdin.
- Emit one severity-aware, scannable GitHub PR comment as markdown to stdout.
- Surface destructive changes (destroys, replacements) prominently.
- Group hierarchical unit paths (`env/layer/unit`) into readable sections.
- Stay stateless and composition-agnostic: summarize exactly the units handed to
  one invocation; work equally well for one unit, one environment, or a whole PR.
- Embed a stable marker so CI can update one comment in place per scope.

## Non-Goals (v1)

- No TUI / interactive plan exploration.
- No direct GitHub API posting or comment management (marker only; CI posts).
- No invoking terragrunt/terraform, no reading `.tfplan` binaries, no state access.
- No cross-invocation aggregation or persistence.
- No CI-gating exit codes based on plan content (e.g. fail-on-destroy) — noted as
  a possible future flag, out of scope for v1.

## The Experience

One place that planned a couple of units:

```bash
for unit in networking/vpc networking/nat; do
  ( cd "$unit" && terragrunt show -json plan.tfplan \
      | jq -c --arg n "$unit" '{name:$n, plan:.}' )
done | gruntcmt --scope networking > comment.md

gh pr comment "$PR" --edit-last-if-matches '<!-- gruntcmt:scope=networking -->' \
  --body-file comment.md
```

Everything for a PR funneled into one grouped comment:

```bash
cat dev-plans.ndjson prod-plans.ndjson staging-plans.ndjson \
  | gruntcmt --scope infra --group-by 1 > comment.md
```

A single unit, without wrapping:

```bash
terragrunt show -json plan.tfplan | gruntcmt --name networking/vpc > comment.md
```

### Composition models (both first-class)

- **One grouped comment:** funnel all units into one invocation. Output is grouped
  by path segment (environment by default), one comment with one marker.
- **One comment per scope:** independent pipelines each run their own
  `gruntcmt --scope <env>`. The PR ends up with several comments, each identified
  by its own marker (`scope=production`, `scope=staging`, …), each updated in place
  by the pipeline that owns it. They never collide.

gruntcmt does not know or care which model is in use; the composition is entirely
how the caller wires the pipes.

## Input Contract

stdin carries Terraform plan JSON in one of two forms, auto-detected (overridable
with `--input=auto|wrapped|plan`, default `auto`):

1. **Wrapped NDJSON** — one JSON object per line, each `{"name": "<unit path>",
   "plan": { <terraform show -json output> }}`. This is the multi-unit form; the
   caller's per-unit loop produces it trivially. Unit name comes from `name`.

2. **Bare plan** — a single Terraform plan JSON document (possibly
   pretty-printed / multi-line) = one unit. Its name comes from `--name`
   (default: `plan`).

**Auto-detection rule:** decode the first top-level JSON object from stdin. If it
contains a top-level `format_version` key (the Terraform plan marker), treat all
of stdin as a single bare plan. Otherwise treat stdin as wrapped NDJSON and read
records until EOF.

**Unit names are `/`-delimited hierarchical paths.** Grouping and nesting derive
from these paths; there is no separate topology configuration.

**Per-unit error isolation:** a record whose `plan` is malformed, empty, or not a
recognizable Terraform plan is recorded as a load error against that unit and
surfaced in the output (see Error Handling). One bad unit never aborts the run.

## Normalized Model

Parsing collapses Terraform's plan JSON into a small internal model:

```
Report
  Scope        string          // from --scope; also the marker key
  GroupBy      int             // path-segment depth for grouping (default 1)
  Units        []Unit
  LoadErrors   []LoadError     // units that failed to parse

Unit
  Name             string      // e.g. "production/database/primary"
  TerraformVersion string
  Changes          []ResourceChange
  OutputChanges    []OutputChange
  Drift            []ResourceChange   // from resource_drift
  Counts           Counts

ResourceChange
  Address   string             // e.g. aws_db_instance.primary
  Action    Action             // Create|Update|Delete|Replace|NoOp|Read
  // (further detail — changed attribute names, sensitivity — may be added later)

Counts
  Add      int   // creates + replaces  (mirrors `terraform plan` summary)
  Change   int   // updates
  Destroy  int   // deletes  + replaces
  Replace  int   // surfaced explicitly in addition to Add/Destroy
  NoOp     int
```

**Action derivation** from `change.actions` in the plan JSON:

- `["create"]` → Create
- `["update"]` → Update
- `["delete"]` → Delete
- `["delete","create"]` or `["create","delete"]` → Replace
- `["no-op"]` → NoOp
- `["read"]` → Read

Counts mirror Terraform's own summary math: a Replace counts as both one Add and
one Destroy, and is additionally counted in `Replace` so it can be labeled.

## Analysis & Severity

**Severity ranking** (high → low): Destroy = Replace (destructive) > Update >
Create > Read > NoOp.

- **Unit severity** = its most severe change.
- **Group severity** = its most severe unit.
- **Report severity** = its most severe group.

Sorting: groups sort by severity descending (so `production` with destroys floats
to the top); within a group, units sort by severity descending. No-op units and
no-op groups sort last and render folded.

**Headline emoji/label** from report severity:

- any Destroy/Replace → 🔴
- else any Update → 🟡
- else any Create → 🟢
- else ➖ (no-op)

## Output Format (GitHub-flavored markdown)

Structure, top to bottom:

1. **Marker comment** — `<!-- gruntcmt:scope=<scope> -->` as the very first line,
   so CI can find and update-in-place. Present even when `--scope` is unset
   (`scope=` empty), though `--scope` is recommended.
2. **Headline** — `### <emoji> Terragrunt plan — <scope/N units> · <D> destroy ·
   <A> add · <C> change`. Destroy count leads because it is what matters most.
3. **Summary table** — one row per group (`--group-by` segments of the unit path):
   Group | Units | Add | Change | Destroy | status cell (⚠️ **destroys** / ✅ /
   ➖ no-op).
4. **Destructive callout** — when any destroys exist: `⚠️ **N destructive
   changes** — review carefully.`
5. **Grouped details** — one collapsible `<details>` per group, sorted by
   severity. Inside, one nested collapsible `<details>` per unit showing a
   ```diff```-fenced list of changes (`+ create`, `~ update`, `- destroy`,
   `-/+ replace`). No-op units/groups render folded with a "no changes" note.
6. **Drift / output-change callouts** — brief collapsible sections when
   `resource_drift` or `output_changes` are present.
7. **Footer** — `<sub>gruntcmt · terraform <version> · …</sub>`.

Large outputs stay scannable because every group and unit is collapsible and
no-ops are folded.

## CLI Surface

```
gruntcmt [flags]   # reads stdin, writes markdown to stdout

--scope string     Scope label + marker key (<!-- gruntcmt:scope=... -->).
--name string      Unit name for a bare (unwrapped) single plan. Default "plan".
--group-by int     Path-segment depth to group by in the summary. Default 1.
--input string     auto | wrapped | plan. Default "auto".
--version          Print version and exit.
```

**Exit codes:** `0` on success (a comment was emitted, even if some units had load
errors). Non-zero when stdin is unreadable or contains no parseable plan at all.
Plan *content* (including destroys) never affects the exit code in v1.

## Package Layout

```
cmd/gruntcmt/main.go     Flag parsing; wire input → analyze → render; write stdout.
internal/plan/           Terraform plan JSON types; parse into normalized Unit.
internal/input/          Read stdin; auto-detect wrapped-NDJSON vs bare plan; []Unit.
internal/analyze/        Build Report: group, count, derive severity, sort.
internal/render/         Render Report → GitHub markdown.
```

Each package has one job and a small interface: `input` yields `[]Unit` +
`[]LoadError`; `analyze` turns those into a sorted `Report`; `render` turns a
`Report` into a markdown string. The core is pure — no I/O below `cmd` and
`input`, no terminal, no network — so it is fully unit-testable.

## Error Handling

- **Unreadable stdin / no valid plans:** write a short diagnostic to stderr, exit
  non-zero, emit no comment.
- **Per-unit parse failure:** collect as `LoadError{Name, Message}`; the run
  continues. The rendered comment includes a `⚠️ N units failed to parse`
  collapsible section listing the offending units and messages, so a broken plan
  is visible on the PR rather than silently dropped.
- **Unknown `change.actions` combinations:** map to a neutral "changed" action and
  keep going rather than failing.

## Testing Strategy

- **Fixtures:** checked-in Terraform plan JSON samples covering create-only,
  update, delete, replace, no-op, drift, output changes, mixed multi-unit, and
  malformed input.
- **Core unit tests:** `plan` (action derivation, count math incl. replace),
  `input` (auto-detection: wrapped vs bare, NDJSON streaming, bad records),
  `analyze` (severity ranking, grouping by depth, sort order).
- **Golden tests:** `render` compares output against checked-in golden markdown
  files for representative reports; update via a `-update` test flag.
- **End-to-end:** feed fixtures to the built binary over stdin, assert on marker
  presence, headline, and table.

## Possible Future Work (explicitly deferred)

- CI-gating exit codes (e.g. `--fail-on destroy`).
- Direct GitHub posting / sticky-comment management.
- Interactive TUI exploration of a plan.
- Cost or policy annotations; per-resource attribute diffs.
