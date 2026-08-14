# gruntcmt examples

- **[`terragrunt/`](terragrunt/)** — a real, cloud-free terragrunt project (uses the
  `random` provider). Plan it locally and pipe the output into gruntcmt; a helper
  script also demonstrates destroys/replacements/no-ops. Start here.
- **[`workflows/terragrunt-plan.yml`](workflows/terragrunt-plan.yml)** — a consumer
  GitHub Actions workflow that plans the terragrunt units and posts a gruntcmt
  summary via the reusable action (`uses: dknathalage/gruntcmt@v1`).

Quick local run:

```bash
cd examples/terragrunt
mise install                 # OpenTofu + terragrunt
./plan-to-ndjson.sh | gruntcmt --scope infra --config .gruntcmt.yaml
./demo-changes.sh   | gruntcmt --scope infra --config .gruntcmt.yaml   # destroys/replaces
```

See [`../docs/scenarios.html`](../docs/scenarios.html) for rendered mockups across
more scenarios.
