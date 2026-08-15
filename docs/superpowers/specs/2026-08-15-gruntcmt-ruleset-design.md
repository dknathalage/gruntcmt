# gruntcmt Ruleset — Design

**Date:** 2026-08-15
**Status:** Draft for review
**Supersedes:** the layered-config model in `2026-08-14-gruntcmt-design.md` (Configuration section)

## Summary

Replace gruntcmt's layered YAML config (global → repo → `--config` → flags, with
env/flag overrides) with a single, composable **ruleset**. A ruleset assigns
rendering behavior **per resource change** — keyed on the unit path *and* the
change's action — and can **split output into multiple PR comments**. A ruleset
may declare a `base:` pointing at a ruleset in another GitHub repo; the base is
fetched and the local ruleset layers on top to form one merged ruleset.

This is a breaking change to configuration and the CLI surface → **v0.4.0**.

## Motivation

- Detail should be controllable **per action** (show every delete in full, just
  count creates) and **per path**, not one fidelity per unit.
- Teams want to **share one ruleset** across many repos and extend it locally.
- The layered global/repo/env/flag precedence added surface without clear value;
  a single file + explicit `base:` is easier to reason about.

## Config File

- **Name:** `gruntcmt.yaml` (non-hidden — it is a first-class, shareable artifact).
- **Flag:** `--ruleset <path>` (renamed from `--config`). If omitted, gruntcmt
  looks for `gruntcmt.yaml` in the current directory only (no upward discovery, no
  global config).
- With no ruleset found, gruntcmt uses built-in defaults (see Defaults).

## Ruleset Schema

```yaml
# optional: a base ruleset in another repo, layered UNDER the local rules
base: owner/repo//path/to/gruntcmt.yaml@ref

rules:
  - path: "**"              # doublestar glob on the unit path; "**" = every unit
    title: "Terragrunt plan" # comment title (report-global; see Resolution)
    group-by: 1             # grouping depth (report-global)
    dedicated-comment: false # keep matching units in the main comment
    scope: ""               # marker scope for a dedicated comment (see below)
    # per-action detail: summary | resource | attribute
    create: summary
    update: resource
    delete: attribute
    replace: attribute
    noop: summary

  - path: "**/database*"    # database units: everything in full
    create: attribute
    update: attribute
    delete: attribute
    replace: attribute

  - path: "**/security/**"  # split security units into their own comment
    dedicated-comment: true
    scope: security
    title: "Security plan"
    delete: attribute
    replace: attribute
```

`rules` is an **ordered array**. Each field is optional except `path`.

### Rule fields

| Field | Type | Meaning |
|---|---|---|
| `path` | string glob | doublestar match against the unit path. `**` = all. |
| `create`/`update`/`delete`/`replace`/`noop` | `summary`\|`resource`\|`attribute` | per-action detail for changes of that action in matching units. |
| `title` | string | comment title (report-global; taken from the matching default/dedicated rule). |
| `group-by` | int | grouping depth (report-global). |
| `dedicated-comment` | bool | if true, matching units render in their own comment. |
| `scope` | string | marker scope for this rule's dedicated comment. |

## Detail Resolution (per resource change)

For each resource change `(unitPath, action)`:

1. If `--detail` is **not** a flag anymore, there is no global hammer. Resolution
   is purely from the ruleset.
2. Walk `rules` in order; a rule *applies* to the change when its `path` matches
   `unitPath` **and** it specifies a detail for `action`. The **last** applying
   rule wins.
3. If no rule specifies that action, fall back to the built-in default for that
   action (see Defaults). The `**` rule normally specifies all five actions, so
   this fallback is rarely reached.

**Per-change detail meaning:**

- `summary` — the change is **not listed individually**; it still counts in the
  totals and the summary table.
- `resource` — an address + action line (`+` / `~` / `-` / `-/+`).
- `attribute` — address + reconstructed before → after attribute diff
  (forces-replacement / sensitive / known-after-apply shown; unchanged attributes
  omitted).

A unit's `<details>` block renders when **any** of its changes resolve above
`summary`. A unit whose changes are all `summary`/no-op collapses to a single
"no changes" line.

## Dedicated Comments (multi-document output)

A single gruntcmt run can now produce **multiple comments**:

- **Assignment:** for each unit, the **last** rule matching its `path` that has
  `dedicated-comment: true` sends the unit to that rule's dedicated comment. Units
  matched by no dedicated rule go to the **main** comment.
- **Identity:** the main comment uses the CLI `--scope` for its marker/title
  (title from the applicable `**`/default rule). Each dedicated comment uses its
  rule's `scope` (marker `<!-- gruntcmt:scope=<scope> -->`) and `title`.
- **Rendering:** each comment is a full, independent document (marker, headline,
  table, details) over its own set of units, using per-change detail resolution.
- **Output:**
  - `--out gh` posts/updates **each** comment by its own marker (a re-run updates
    each in place; distinct scopes never collide).
  - `--out stdout` prints the documents back-to-back in a stable order (main
    first, then dedicated comments by scope).

Empty comments (a dedicated rule that matched no units, or a main comment with no
units) are skipped.

## Base Ruleset (`base:`) — fetch & merge

- **Reference syntax (GitHub shorthand):** `owner/repo//path/to/file.yaml@ref`
  where `//` separates repo from in-repo path and `@ref` is a branch, tag, or SHA
  (defaults to the repo's default branch if omitted).
- **Fetch:** GitHub REST contents API
  `GET {GITHUB_API_URL}/repos/{owner}/{repo}/contents/{path}?ref={ref}` with
  `Accept: application/vnd.github.raw`. `GITHUB_API_URL` (Actions/GHE) honored.
- **Auth — default GitHub authentication, locally and in Actions:** token from
  `GITHUB_TOKEN` or `GH_TOKEN`; if neither is set and the `gh` CLI is available,
  use `gh auth token`. Public repos may fetch unauthenticated. This is a *fetch*
  credential, distinct from the removed config-value overrides.
- **Recursion:** a base may itself declare a `base:`; resolve depth-first with a
  **cycle guard** (by `owner/repo//path@ref`) and a max depth (e.g. 10).
- **Merge:** the fully-resolved base's `rules` come first; the local `rules` are
  appended, so **local rules win** under last-match-wins. Report-global fields
  (`title`, `group-by`) are taken from the local applicable rule if set, else the
  base's. `base` fields do not chain into the local file beyond providing rules.

## Defaults (no ruleset / unspecified action)

Built-in behavior when no `gruntcmt.yaml` is present or an action is unspecified:

- `group-by`: 1
- `title`: "Terragrunt plan"
- per-action detail: `resource` for all actions (matches the current default
  fidelity), `noop`: `summary`.

## CLI Surface (v0.4.0)

```
gruntcmt [flags]   # reads plan JSON on stdin

--ruleset path     Ruleset file (default: ./gruntcmt.yaml if present).
--scope string     Scope/marker for the MAIN comment.
--name string      Unit name for a bare single plan. Default "plan".
--input string     auto | wrapped | plan. Default "auto".
--commit string    Commit SHA stamped in footers.
--out string       stdout | gh. Default "stdout".
--repo string      owner/name for --out gh (default $GITHUB_REPOSITORY).
--pr int           PR number for --out gh (default: auto-detect in Actions).
--print-ruleset    Print the merged ruleset to stderr and exit.
--version          Print version and exit.
```

**Removed:** `--config` (→ `--ruleset`), `--detail`, `--group-by`, `--no-config`,
`--print-config` (→ `--print-ruleset`). No env-var config overrides; no global vs
repo layering.

## Package / Structure Changes

- `internal/config` → **`internal/ruleset`**: types (`Ruleset`, `Rule`), YAML load,
  `base` fetch + merge, and resolution: `DetailFor(unitPath, action) Fidelity`,
  `AssignComment(unitPath) (scope, title, dedicated)`, plus `GroupBy()`/`Title()`.
- New `internal/ruleset/fetch.go` (or reuse `internal/gh`) for the GitHub contents
  fetch + `gh auth token` fallback. Networked; isolated at the edge.
- `internal/analyze`: partition units into comment-sets (main + dedicated) and
  stamp per-change detail using the ruleset. Produces `[]Report` (one per comment)
  instead of one `Report`.
- `internal/render`: render one `Report`; unchanged except per-change detail
  drives the diff body (summary changes omitted; unit collapses when all summary).
- `cmd/gruntcmt`: load+merge ruleset; for each produced `Report`, either write to
  stdout (concatenated) or post via `--out gh` by its marker.

Core packages remain pure except the ruleset `base` fetch (edge, injectable HTTP).

## Error Handling

- Missing/invalid ruleset YAML → stderr diagnostic, non-zero exit.
- `base` fetch failure (network/auth/404) → non-zero exit with a clear message
  (repo, path, ref, status).
- Cyclic/over-deep `base` chain → non-zero exit naming the cycle.
- `--out gh` post failure → non-zero (as today).

## Testing

- `internal/ruleset`: YAML parse; per-action resolution incl. last-match-wins and
  path+action interaction; dedicated-comment assignment; `base` merge order
  (local wins); recursion + cycle detection; token resolution precedence
  (env → `gh auth token`); `base` fetch against an `httptest` server.
- `internal/analyze`: unit partitioning into main + dedicated reports; per-change
  detail stamping.
- `internal/render`: golden per detail-mix (a unit with creates=summary +
  deletes=attribute in one diff body); dedicated vs main documents.
- `cmd`: `--out stdout` emits N documents in stable order; `--out gh` posts each
  marker; `--print-ruleset` shows the merged result; error paths.
- End-to-end: the numbered terragrunt example (below) through the real pipeline.

## Numbered Example (rebuild)

`examples/terragrunt/` becomes numbered scenario units demonstrating the ruleset:

```
examples/terragrunt/
  gruntcmt.yaml                 # the ruleset (base optional; per-action + dedicated)
  root.hcl
  modules/scenario/             # driven by `scenario` + `phase` (get_env)
  live/
    01-create/ 02-update/ 03-replace/ 04-destroy/ 05-noop/ 06-summary/
  plan-scenarios.sh             # apply baseline, plan changed -> one NDJSON
```

The ruleset maps actions to mixed fidelities (creates→summary, updates→resource,
deletes/replaces→attribute) and marks e.g. a `**/database*` or security path as a
`dedicated-comment`, so one run posts the scenarios comment plus a dedicated one —
exercising per-change detail, grouping, and multi-comment output. Verified live on
a PR via `.github/workflows/pr-demo.yml`.

## Migration / Compatibility

Breaking (v0.4.0). Existing `.gruntcmt.yaml` files (`overrides:` + `detail:`) are
**not** auto-migrated; the README documents the mapping from `overrides` (path →
detail) to `rules` (path → per-action detail with `**` default). The `--out gh`
work from v0.3.0 is a prerequisite (dedicated comments reuse its posting path).

## Non-Goals

- No non-GitHub base sources (plain HTTPS/git) in v1 — GitHub shorthand only.
- No per-rule env/flag overrides (explicitly removed).
- No caching of fetched bases beyond a single run.
