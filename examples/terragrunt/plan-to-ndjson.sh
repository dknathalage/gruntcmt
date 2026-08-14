#!/usr/bin/env bash
# Plan every terragrunt unit under live/ and emit gruntcmt's wrapped-NDJSON on
# stdout: one {"name","plan"} line per unit, where name is the unit's path
# relative to live/ (e.g. "production/database").
#
#   ./plan-to-ndjson.sh | gruntcmt --scope infra --detail resource
#
# Each unit is planned and shown in its own working dir — the same per-unit
# pattern most CI matrices use. Uses OpenTofu by default; set
# TG_TF_PATH=terraform to use Terraform instead.
set -euo pipefail
export TG_TF_PATH="${TG_TF_PATH:-tofu}"
export TERRAGRUNT_TFPATH="${TERRAGRUNT_TFPATH:-$TG_TF_PATH}"  # older terragrunt
export TG_NON_INTERACTIVE=true

here="$(cd "$(dirname "$0")" && pwd)"
live="$here/live"

while IFS= read -r unit; do
  name="${unit#"$live"/}"
  (
    cd "$unit"
    # Plan + show for this unit; logs to stderr, JSON to stdout.
    terragrunt plan -out=plan.tfplan 1>&2
    terragrunt show -json plan.tfplan 2>/dev/null \
      | jq -c --arg n "$name" '{name: $n, plan: .}'
  )
done < <(find "$live" -name .terragrunt-cache -prune -o -name terragrunt.hcl -print \
          | xargs -n1 dirname | sort -u)
