# gruntcmt examples

- **[`terragrunt/`](terragrunt/)** — a real, cloud-free terragrunt project using
  `terraform_data` (no provider, no cloud). Six numbered scenarios drive every
  change type (create, update, replace, destroy, noop) plus a dedicated security
  comment. Plan them locally and pipe the output into gruntcmt with a ruleset.
  Start here.
- **[`workflows/terragrunt-plan.yml`](workflows/terragrunt-plan.yml)** — a consumer
  GitHub Actions workflow that plans the terragrunt units and posts a gruntcmt
  summary via the reusable action (`uses: dknathalage/gruntcmt@v1`).

Quick local run:

```bash
cd examples/terragrunt
mise install                    # OpenTofu + terragrunt
./plan-scenarios.sh | gruntcmt --scope scenarios --ruleset gruntcmt.yaml
```

See [`../docs/scenarios.html`](../docs/scenarios.html) for rendered mockups across
more scenarios.
