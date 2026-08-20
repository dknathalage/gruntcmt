# gruntcmt examples

- **[`terragrunt/`](terragrunt/)** — a real, cloud-free terragrunt project using
  `terraform_data` (no provider, no cloud). Six numbered scenarios drive every
  change type (create, update, replace, destroy, noop) plus a dedicated security
  comment. Plan them into a JSON plan directory and hand the directory to
  gruntcmt. Start here.

Quick local run:

```bash
cd examples/terragrunt
mise install                    # OpenTofu + terragrunt
./plan-scenarios.sh             # plans into ./out and prints the gruntcmt summary
```

See [`../docs/scenarios.html`](../docs/scenarios.html) for rendered mockups across
more scenarios.
