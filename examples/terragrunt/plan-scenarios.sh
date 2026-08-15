#!/usr/bin/env bash
# Plan every terragrunt scenario unit under live/ and emit gruntcmt's wrapped-NDJSON
# on stdout: one {"name","plan"} line per unit, where name is the path relative to
# live/ (e.g. "01-create").
#
# Usage:
#   ./plan-scenarios.sh 2>/dev/null | gruntcmt --scope scenarios --ruleset gruntcmt.yaml
#
# Applies a baseline state first, then plans at PHASE=changed so each unit
# produces its designed change type (create/update/replace/destroy/noop).
set -euo pipefail

export TG_TF_PATH="${TG_TF_PATH:-tofu}"
export TERRAGRUNT_TFPATH="${TERRAGRUNT_TFPATH:-$TG_TF_PATH}"
export TG_NON_INTERACTIVE=true

here="$(cd "$(dirname "$0")" && pwd)"
live="$here/live"

# Collect unit dirs, pruning .terragrunt-cache so cached copies are not included.
units_list="$(find "$live" -name .terragrunt-cache -prune -o -name terragrunt.hcl -print \
  | xargs -n1 dirname | sort -u)"

# Phase 1: apply baseline state for each unit individually.
echo "==> Applying baseline state..." >&2
while IFS= read -r unit; do
  name="${unit#"$live"/}"
  echo "  -> baseline apply: $name" >&2
  (
    cd "$unit"
    PHASE=baseline terragrunt apply -auto-approve 1>&2
  )
done <<< "$units_list"

# Phase 2: plan each unit at PHASE=changed and emit NDJSON.
echo "==> Planning changed phase..." >&2
export PHASE=changed
while IFS= read -r unit; do
  name="${unit#"$live"/}"
  echo "  -> plan: $name" >&2
  (
    cd "$unit"
    terragrunt plan -out=plan.tfplan 1>&2
    terragrunt show -json plan.tfplan 2>/dev/null \
      | jq -c --arg n "$name" '{name: $n, plan: .}'
  )
done <<< "$units_list"
