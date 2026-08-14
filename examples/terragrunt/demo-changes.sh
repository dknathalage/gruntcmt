#!/usr/bin/env bash
# OPTIONAL demo that produces a *realistic mix* (add / change / replace / destroy /
# no-op) instead of an all-create first plan:
#   1. apply every unit once to establish state (random resources only — no cloud),
#   2. mutate a couple of inputs (restored automatically on exit),
#   3. re-plan and emit gruntcmt NDJSON on stdout.
#
#   ./demo-changes.sh | gruntcmt --scope infra --detail attribute
set -euo pipefail
export TG_TF_PATH="${TG_TF_PATH:-tofu}"
export TERRAGRUNT_TFPATH="${TERRAGRUNT_TFPATH:-$TG_TF_PATH}"  # older terragrunt
export TG_NON_INTERACTIVE=true

here="$(cd "$(dirname "$0")" && pwd)"
live="$here/live"
net="$live/production/networking/terragrunt.hcl"
db="$live/production/database/terragrunt.hcl"

# 1) Establish state.
( cd "$live" && terragrunt run --all -- apply -auto-approve ) 1>&2

# 2) Mutate inputs, restoring the originals on any exit.
cp "$net" "$net.bak"
cp "$db" "$db.bak"
trap 'mv "$net.bak" "$net"; mv "$db.bak" "$db"' EXIT
perl -pi -e 's/instance_count = 3/instance_count = 2/' "$net"          # destroy 1 random_pet
perl -pi -e 's/engine_version = "14\.7"/engine_version = "15.4"/' "$db" # replace random_id.release
# staging/cache is left untouched -> no-op

# 3) Re-plan the changed state and emit NDJSON.
"$here/plan-to-ndjson.sh"
