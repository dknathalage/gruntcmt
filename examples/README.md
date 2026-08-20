# gruntcmt examples

- **[`terragrunt/`](terragrunt/)** — a real, cloud-free terragrunt project using
  `terraform_data` (no provider, no cloud). Six numbered scenarios drive every
  change type (create, update, replace, destroy, noop) plus a dedicated security
  comment. Plan them into a JSON plan directory and hand the directory to
  gruntcmt. Start here.

- **[`workflows/terragrunt-plan.yml`](workflows/terragrunt-plan.yml)** — a
  copy-pasteable GitHub Actions workflow that plans terragrunt units on each PR and
  comments the summary with the gruntcmt CLI.

Quick local run:

```bash
cd examples/terragrunt
mise install                    # OpenTofu + terragrunt (example toolchain)
./plan-scenarios.sh             # plans into ./out
gruntcmt --config gruntcmt.yaml --out /dev/stdout out
```

See [`../docs/scenarios.html`](../docs/scenarios.html) for rendered mockups across
more scenarios.
