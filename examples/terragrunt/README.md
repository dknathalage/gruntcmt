# Real terragrunt example

A tiny but real terragrunt project you can plan locally — no cloud account, no
credentials. Every unit uses the `random` provider, so `plan`/`apply` produce
genuine resource changes that gruntcmt summarizes.

```
root.hcl                       # shared root config (included by every unit)
modules/app/main.tf            # random_pet / random_id / random_password module
live/
  production/networking/       # instance_count = 3
  production/database/         # engine_version input keys a replaceable resource
  staging/cache/               # instance_count = 2
.gruntcmt.yaml                 # group by env, fold no-ops, attribute detail for production/**
plan-to-ndjson.sh             # plan every unit -> gruntcmt wrapped-NDJSON
demo-changes.sh               # apply, then mutate inputs to show destroys/replaces
```

## Prerequisites

`gruntcmt`, plus OpenTofu (or Terraform) and terragrunt. Get the pinned tools with
mise:

```bash
cd examples/terragrunt
mise install          # OpenTofu 1.12.3 + terragrunt 1.0.8
```

## Plan and summarize (first run: all creates)

```bash
./plan-to-ndjson.sh | gruntcmt --scope infra --config .gruntcmt.yaml
```

`plan-to-ndjson.sh` plans each unit and emits one wrapped-NDJSON line per unit
(`{"name":"production/database","plan":{…}}`). On a clean project every unit is a
create:

```markdown
### 🟢 Terragrunt plan — `infra` · 3 units · 0 destroy · 12 add · 0 change

| Group | Units | Add | Change | Destroy | |
|---|---|---|---|---|---|
| `production` | 2 | 8 | 0 | 0 | ✅ |
| `staging` | 1 | 4 | 0 | 0 | ✅ |
```

## Show destroys and replacements (`demo-changes.sh`)

To see gruntcmt's headline feature — surfacing destructive changes — apply once,
then re-plan against a mutated config. `demo-changes.sh` does this and restores the
inputs afterwards (the "resources" are just random values, so applying is safe):

```bash
./demo-changes.sh | gruntcmt --scope infra --config .gruntcmt.yaml
```

It bumps `production/database`'s `engine_version` (forces a replacement), shrinks
`production/networking` by one instance (a destroy), and leaves `staging/cache`
untouched (a no-op that folds away):

```markdown
### 🔴 Terragrunt plan — `infra` · 3 units · 2 destroy · 1 add · 0 change

| Group | Units | Add | Change | Destroy | |
|---|---|---|---|---|---|
| `production` | 2 | 1 | 0 | 2 | ⚠️ **destroys** |
| `staging` | 1 | 0 | 0 | 0 | ➖ no-op |

⚠️ **2 destructive changes** — review carefully.
```
```diff
-/+ random_id.release
    ~ keepers = {"engine_version":"14.7"} -> {"engine_version":"15.4"}  # forces replacement
- random_pet.instance[2]
    - id = prod-net-intimate-beetle
```

Because `.gruntcmt.yaml` maps `production/**` to attribute fidelity, production
units expand into full before → after diffs while the no-op staging group folds to
a single line.

## In CI

See `../workflows/terragrunt-plan.yml` for a workflow that runs this and posts the
result with the reusable action (`uses: dknathalage/gruntcmt@v1`).
