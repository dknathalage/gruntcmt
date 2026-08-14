# gruntcmt — Design

**Date:** 2026-08-14
**Status:** Approved for planning

## Summary

`gruntcmt` is a small Go CLI that turns Terraform plan JSON — produced by
terragrunt units — into a single, meaningful chunk of GitHub-flavored markdown
written to stdout. It is a pure Unix filter: it reads plan JSON from stdin, writes
markdown to stdout, and does nothing else. It never invokes terragrunt/terraform,
makes no network calls, and holds no state.

The markdown targets two GitHub surfaces: a **PR comment** and the **Actions job
summary** (`$GITHUB_STEP_SUMMARY`). gruntcmt is surface-agnostic — it renders at a
chosen fidelity (`--detail`), and the caller directs each rendering to the
destination it fits. Typically CI runs gruntcmt twice: a lean `--detail resource`
rendering into the PR comment, and a full `--detail attribute` rendering into the
job summary. gruntcmt never truncates; picking a fidelity that fits a surface's
size limit is the operator's decision (see GitHub Limits).

There is no interactive/TUI mode and no direct GitHub posting. Delivering and
updating the comment on the PR is the caller's job (CI + `gh`), aided by a stable
marker gruntcmt embeds in the output.

## Goals

- Read Terraform plan JSON for one or more terragrunt units from stdin.
- Emit one severity-aware, scannable chunk of GitHub markdown to stdout.
- Render at a chosen fidelity — `summary`, `resource`, or `attribute` — applied
  uniformly to every resource.
- Reconstruct real per-attribute diffs (before → after, forces-replacement,
  sensitive, known-after-apply) at `attribute` fidelity.
- Surface destructive changes (destroys, replacements) prominently.
- Group hierarchical unit paths (`env/layer/unit`) at a configurable depth.
- Stay stateless and composition-agnostic: summarize exactly the units handed to
  one invocation; work equally well for one unit, one environment, or a whole PR.
- Embed a stable marker so CI can update one comment in place per scope.
- Read layered configuration (global → repo → `--config` → flags) so teams set
  conventions once, including per-path behavior overrides.

## Non-Goals (v1)

- No TUI / interactive plan exploration.
- No direct GitHub API posting or comment management (marker only; CI posts).
- No invoking terragrunt/terraform, no reading `.tfplan` binaries, no state access.
- No cross-invocation aggregation or persistence.
- **No output budgeting or truncation.** gruntcmt emits exactly the requested
  fidelity; fitting a surface's byte limit is the operator's call via `--detail`.
- No CI-gating exit codes based on plan content (e.g. fail-on-destroy) — a
  possible future flag.
- No "changed since last plan" diffing — that needs prior-plan state gruntcmt
  does not keep. Deferred.

## GitHub Limits (design context, not enforced)

- **PR / issue comment body:** 65,536 characters (hard API cap; over it the post
  fails).
- **Actions job summary:** 1 MiB per step.
- **HTML comments** (`<!-- … -->`): preserved in the raw body, hidden in the
  rendered view — this is what makes the marker work.
- Collapsed `<details>` content still counts toward the byte limit.

gruntcmt does not police these. The guidance is simply: use a leaner `--detail`
for the comment and `--detail attribute` for the roomier job summary.

## The Experience

One place that planned a couple of units, into a PR comment:

```bash
for unit in networking/vpc networking/nat; do
  ( cd "$unit" && terragrunt show -json plan.tfplan \
      | jq -c --arg n "$unit" '{name:$n, plan:.}' )
done | gruntcmt --scope networking --detail resource > comment.md

gh pr comment "$PR" --edit-last-if-matches '<!-- gruntcmt:scope=networking -->' \
  --body-file comment.md
```

Same plans, full fidelity, into the job summary:

```bash
cat networking.ndjson | gruntcmt --scope networking --detail attribute \
  >> "$GITHUB_STEP_SUMMARY"
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

**Unit names are `/`-delimited hierarchical paths.** Grouping derives from these
paths; there is no separate topology configuration.

**Per-unit error isolation:** a record whose `plan` is malformed, empty, or not a
recognizable Terraform plan is recorded as a load error against that unit and
surfaced in the output (see Error Handling). One bad unit never aborts the run.

## Configuration

Settings resolve from four layers, **highest precedence first**; layers merge
key-by-key (a lower layer supplies a value only if no higher layer set it):

1. **CLI flags** — explicit, always win.
2. **`--config <path>`** — an explicit config file; skips discovery.
3. **Repo config** — the nearest `.gruntcmt.yaml` found by walking up from the
   current directory to the repo root (`.git`) or filesystem root.
4. **Global config** — `$XDG_CONFIG_HOME/gruntcmt/config.yaml`
   (default `~/.config/gruntcmt/config.yaml`).
5. Built-in defaults.

`--no-config` ignores all config files (reproducible CI). `--print-config` writes
the fully resolved configuration to stderr and exits, for debugging precedence.

**Format: YAML.** Schema:

```yaml
# .gruntcmt.yaml (repo root)
group-by: 1                # default grouping depth
detail: resource           # default fidelity: summary | resource | attribute
input: auto                # auto | wrapped | plan

render:
  title: "Terragrunt plan"
  emoji:                   # override severity glyphs to house style
    destroy: "🔴"
    change:  "🟡"
    add:     "🟢"
    noop:    "➖"
  hide-unchanged: true     # collapse unchanged attributes to a count (attribute detail)
  fold-noop: true          # fold no-op units/groups

overrides:                 # per-path behavior; last match wins
  - path: "**/database/**" # glob on the unit path
    detail: attribute
  - path: "production/**"
    detail: attribute
  - path: "development/**"
    detail: summary
```

**Overrides** let behavior vary by unit path. Each entry has a `path` glob
(`*`/`**` on the `/`-delimited unit path) and may set `detail` and the per-render
toggles (`hide-unchanged`, `fold-noop`). Matching is evaluated per unit; when
several entries match, **the last one in file order wins**. Plan-wide settings —
`group-by`, `scope`, `input` — are global only and cannot be overridden per path.

**Flag vs override interaction:** an explicit `--detail` on the command line is a
global hammer that overrides config *and* per-path overrides (the operator asked
for one fidelity everywhere). Per-path fidelity comes from config, not flags. When
`--detail` is not passed, each unit renders at its overridden-or-default fidelity.

## Normalized Model

Parsing collapses Terraform's plan JSON into a small internal model:

```
Report
  Scope        string          // from --scope; also the marker key
  Title        string          // render.title
  GroupBy      int             // path-segment depth for grouping (default 1)
  Units        []Unit
  LoadErrors   []LoadError     // units that failed to parse

Unit
  Name             string      // e.g. "production/database/primary"
  TerraformVersion string
  Detail           Fidelity    // resolved per-unit: Summary|Resource|Attribute
  Changes          []ResourceChange
  OutputChanges    []OutputChange
  Drift            []ResourceChange   // from resource_drift
  Counts           Counts

ResourceChange
  Address    string            // e.g. aws_db_instance.primary
  Action     Action            // Create|Update|Delete|Replace|NoOp|Read
  Attributes []AttributeChange // populated at attribute fidelity

AttributeChange
  Path      string             // e.g. "engine_version", "route[1].cidr_block"
  Before    string             // rendered value, or "" when adding
  After     string             // rendered value, or "" when removing
  Kind      Add|Update|Remove
  ForcesNew bool               // path present in change.replace_paths
  Sensitive bool               // from before_sensitive/after_sensitive → "(sensitive value)"
  Unknown   bool               // from after_unknown        → "(known after apply)"

Counts
  Add      int   // creates + replaces  (mirrors `terraform plan` summary)
  Change   int   // updates
  Destroy  int   // deletes  + replaces
  Replace  int   // surfaced explicitly in addition to Add/Destroy
  NoOp     int
```

**Action derivation** from `change.actions`:

- `["create"]` → Create · `["update"]` → Update · `["delete"]` → Delete
- `["delete","create"]` / `["create","delete"]` → Replace
- `["no-op"]` → NoOp · `["read"]` → Read

Counts mirror Terraform's own summary math: a Replace counts as both one Add and
one Destroy, and additionally in `Replace` so it can be labeled.

**Attribute diff reconstruction** (attribute fidelity only): diff `change.before`
against `change.after`, producing an `AttributeChange` per changed key. Keys
present in `after_unknown` render `(known after apply)`; keys flagged in
`before_sensitive`/`after_sensitive` render `(sensitive value)`; keys whose path
appears in `replace_paths` set `ForcesNew`. When `render.hide-unchanged` is true,
unchanged keys are omitted and summarized as a count.

## Analysis & Severity

**Severity ranking** (high → low): Destroy = Replace (destructive) > Update >
Create > Read > NoOp.

- **Unit severity** = its most severe change.
- **Group severity** = its most severe unit.
- **Report severity** = its most severe group.

Sorting: groups sort by severity descending (so `production` with destroys floats
to the top); within a group, units sort by severity descending. No-op units and
groups sort last and render folded.

**Headline emoji/label** from report severity (glyphs overridable via
`render.emoji`): any Destroy/Replace → 🔴; else any Update → 🟡; else any
Create → 🟢; else ➖ (no-op).

## Grouping (`--group-by N`)

`N` is the number of leading path segments that form a group key; deeper segments
flatten into the unit's leaf label. Grouping is a **single level** (flat groups,
flat units within) — deliberately not a recursive tree, to stay scannable.

Given units `production/database/primary`, `production/database/replica`,
`production/networking`, `staging/database/db1`:

- `--group-by 0` → no groups; one flat list by full path.
- `--group-by 1` (default) → groups `production` (3 units), `staging` (1).
- `--group-by 2` → groups `production/database` (primary, replica),
  `production/networking` (1), `staging/database` (1).

A unit with fewer than `N` segments groups on its whole path as a singleton
(no crash, no padding). Summary-table rows correspond exactly to these groups.

## Fidelity Levels (`--detail`)

One uniform knob applied to every resource; the same report renders at any level:

- **summary** — headline + counts table only. Smallest.
- **resource** — adds per-unit collapsibles listing resource addresses + actions
  (`+ create`, `~ update`, `- destroy`, `-/+ replace`).
- **attribute** — adds a reconstructed per-attribute diff under each changed
  resource (before → after, `# forces replacement`, `(sensitive value)`,
  `(known after apply)`).

Default `resource`. Per-path overrides may raise/lower it per unit unless a
`--detail` flag forces one level globally.

## Output Format (GitHub-flavored markdown)

Top to bottom:

1. **Marker** — `<!-- gruntcmt:scope=<scope> -->` as the first line, so CI can
   update-in-place. Present even when `--scope` is unset.
2. **Headline** — `### <emoji> <title> — <scope/N units> · <D> destroy · <A> add ·
   <C> change`. Destroy leads.
3. **Summary table** — one row per group (per `--group-by`): Group | Units | Add |
   Change | Destroy | status cell (⚠️ **destroys** / ✅ / ➖ no-op). Always shown,
   at every fidelity.
4. **Destructive callout** — when any destroys exist.
5. **Grouped details** (resource/attribute fidelity) — one collapsible `<details>`
   per group (severity-sorted), nesting one `<details>` per unit with a
   ```diff```-fenced body at the unit's resolved fidelity. No-op units/groups fold.
6. **Drift / output-change callouts** — collapsible, when `resource_drift` or
   `output_changes` are present.
7. **Load-error callout** — when any unit failed to parse (see Error Handling).
8. **Footer** — `<sub>gruntcmt · terraform <version> · commit <sha></sub>`. The
   commit/timestamp stamp (optional `--commit`) keeps a re-plan visibly current.

## CLI Surface

```
gruntcmt [flags]   # reads stdin, writes markdown to stdout

--scope string     Scope label + marker key (<!-- gruntcmt:scope=... -->).
--name string      Unit name for a bare (unwrapped) single plan. Default "plan".
--group-by int     Leading path segments to group by. Default 1.
--detail string    summary | resource | attribute. Overrides config globally.
--input string     auto | wrapped | plan. Default "auto".
--commit string    Commit SHA to stamp in the footer (currency).
--config path      Explicit config file; skips discovery.
--no-config        Ignore all config files.
--print-config     Print resolved config to stderr and exit.
--version          Print version and exit.
```

**Exit codes:** `0` on success (a comment was emitted, even if some units had load
errors). Non-zero when stdin is unreadable, config is invalid, or stdin contains
no parseable plan at all. Plan *content* (including destroys) never affects the
exit code in v1.

## Package Layout

```
cmd/gruntcmt/main.go     Flag parsing; resolve config; wire input → analyze → render.
internal/config/         Load + merge global/repo/--config/flags; path-override matching.
internal/plan/           Terraform plan JSON types; parse into normalized Unit + attribute diffs.
internal/input/          Read stdin; auto-detect wrapped-NDJSON vs bare plan; []Unit.
internal/analyze/        Build Report: group, count, derive severity, resolve per-unit detail, sort.
internal/render/         Render Report → GitHub markdown at each fidelity.
```

Each package has one job and a small interface: `config` yields resolved settings;
`input` yields `[]Unit` + `[]LoadError`; `analyze` turns those (plus config) into a
sorted `Report` with per-unit fidelity resolved; `render` turns a `Report` into a
markdown string. The core is pure — no I/O below `cmd`, `config`, and `input`, no
terminal, no network — so it is fully unit-testable.

## Error Handling

- **Unreadable stdin / no valid plans / invalid config:** short diagnostic to
  stderr, non-zero exit, no comment.
- **Per-unit parse failure:** collect as `LoadError{Name, Message}`; the run
  continues. The rendered output includes a `⚠️ N units failed to parse`
  collapsible listing the offending units, so a broken plan is visible on the PR
  rather than silently dropped.
- **Unknown `change.actions` combinations:** map to a neutral "changed" action and
  keep going.

## Testing Strategy

- **Fixtures:** checked-in Terraform plan JSON covering create, update, delete,
  replace, no-op, drift, output changes, sensitive/unknown attributes, mixed
  multi-unit, and malformed input.
- **Core unit tests:** `plan` (action derivation, count math incl. replace,
  attribute-diff reconstruction incl. forces-new/sensitive/unknown), `input`
  (auto-detection, NDJSON streaming, bad records), `analyze` (severity, grouping
  by depth, per-unit detail resolution, sort order), `config` (layer precedence,
  merge, override last-match-wins, `--detail` flag hammer).
- **Golden tests:** `render` compared against checked-in golden markdown per
  representative report **at each fidelity level**; update via a `-update` flag.
- **End-to-end:** feed fixtures to the built binary over stdin with a temp repo
  config; assert on marker, headline, table, and that flags override config.

## Possible Future Work (explicitly deferred)

- CI-gating exit codes (e.g. `--fail-on destroy`).
- Direct GitHub posting / sticky-comment management.
- Interactive TUI exploration of a plan.
- "Changed since last plan" diffing (needs prior-plan state).
- Cost or policy annotations.
