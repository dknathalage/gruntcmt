#!/usr/bin/env bash
# Plan every terragrunt scenario unit under live/ into a JSON plan directory (./out).
# Rendering is left to the caller so the same script works locally and in CI:
#   ./plan-scenarios.sh && gruntcmt --config gruntcmt.yaml --out /dev/stdout out   # local
#   ./plan-scenarios.sh && gruntcmt --config gruntcmt.yaml out                     # CI: comment PR
#
# Applies a baseline state first, then plans at PHASE=changed so each unit
# produces its designed change type (create/update/replace/destroy/noop).
# `terragrunt run --all plan --json-out-dir out` writes one out/<unit>/tfplan.json
# per unit; `gruntcmt out` discovers them by walking the tree.
set -euo pipefail

export TG_TF_PATH="${TG_TF_PATH:-tofu}"
export TERRAGRUNT_TFPATH="${TERRAGRUNT_TFPATH:-$TG_TF_PATH}"
export TG_NON_INTERACTIVE=true

here="$(cd "$(dirname "$0")" && pwd)"
live="$here/live"
outdir="$here/out"

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

# Phase 2: plan every unit at PHASE=changed into a JSON plan directory.
echo "==> Planning changed phase into $outdir ..." >&2
export PHASE=changed
( cd "$live" && terragrunt run --all plan --out-dir "$outdir" --json-out-dir "$outdir" 1>&2 )

echo "==> Wrote plans to $outdir. Summarize with:" >&2
echo "      gruntcmt --config $here/gruntcmt.yaml --out /dev/stdout $outdir" >&2
