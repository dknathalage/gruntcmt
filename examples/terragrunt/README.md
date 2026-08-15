# Terragrunt numbered-scenario example

A real terragrunt project you can plan locally — no cloud account, no credentials.
Every unit uses the built-in `terraform_data` resource (no provider needed), so
`plan`/`apply` produce genuine resource changes that gruntcmt summarizes via a
ruleset.

```
root.hcl                          # shared root config — injects PHASE env var
modules/scenario/main.tf          # terraform_data module driven by scenario + phase
live/
  01-create/                      # create  — appears at PHASE=changed
  02-update/                      # in-place update (input v1 -> v2)
  03-replace/                     # forces-replacement via triggers_replace
  04-destroy/                     # present at baseline, gone at changed
  05-noop/                        # no change
  06-security/                    # create — rendered in a dedicated security comment
gruntcmt.yaml                     # ruleset: group-by 1, per-type fidelity + security rule
plan-scenarios.sh                 # apply baseline + plan changed -> gruntcmt NDJSON
```

## Prerequisites

`gruntcmt`, plus OpenTofu and terragrunt. Get the pinned tools with mise:

```bash
cd examples/terragrunt
mise install          # OpenTofu 1.12.3 + terragrunt 1.0.8
```

## Plan and summarize all scenarios

```bash
./plan-scenarios.sh 2>/dev/null | gruntcmt --scope scenarios --ruleset gruntcmt.yaml
```

`plan-scenarios.sh` applies a baseline state for each unit, then plans at
`PHASE=changed` so each unit produces its designed change type. It emits one
wrapped-NDJSON line per unit (`{"name":"01-create","plan":{…}}`).

Expected output — two documents:

1. **Main comment** (`scope=scenarios`) — a table row per numbered scenario:
   - `01-create`: 1 add (summary, not expanded)
   - `02-update`: resource-level update line
   - `03-replace`: attribute diff with `# forces replacement`
   - `04-destroy`: attribute diff showing deleted values
   - `05-noop`: folded to one line

2. **Dedicated security comment** (`scope=security`) — `06-security` with full
   attribute-level create diff.

## How the ruleset works

`gruntcmt.yaml` demonstrates two rules:

```yaml
rules:
  - path: "**"
    title: "gruntcmt scenarios"
    group-by: 1
    create: summary     # creates counted only (no expansion)
    update: resource    # updates shown at resource level
    delete: attribute   # destroys expanded to full attribute diffs
    replace: attribute  # replacements expanded to full attribute diffs
    noop: summary       # no-ops folded
  - path: "**/06-security"
    dedicated-comment: true
    scope: security
    title: "Security plan"
    create: attribute   # security creates shown with full attribute detail
    delete: attribute
```

The second rule overrides the first for `06-security`, routing it to a separate
dedicated comment with its own scope and attribute fidelity.

## In CI

See `.github/workflows/pr-demo.yml` — it runs `./plan-scenarios.sh` and posts
both comments via `--out gh`, plus writes the job summary.
