# Changelog

## [0.4.1](https://github.com/dknathalage/gruntcmt/compare/v0.4.0...v0.4.1) (2026-08-15)

### Bug Fixes

* **render:** show `(all)` for the flat/empty group key in the group `<details>` header instead of an empty `<code></code>`.

## [0.4.0](https://github.com/dknathalage/gruntcmt/compare/v0.3.0...v0.4.0) (2026-08-15)

### ⚠ BREAKING CHANGES

* The layered `.gruntcmt.yaml` config (`overrides`, `--config`, `--detail`, `--group-by`) is replaced by a composable `gruntcmt.yaml` ruleset (`--ruleset`).

### Features

* **ruleset:** per-resource-change detail resolved by `rules` (path × action, last-match-wins): `summary` (counted, not listed), `resource` (address line), `attribute` (full before→after diff).
* **ruleset:** `dedicated-comment` rules split output into multiple PR comments, each posted/updated by its own marker under `--out gh`.
* **ruleset:** `base:` imports a ruleset from another GitHub repo (`owner/repo//path@ref`), fetched with default GitHub auth and merged under the local rules.
* **cmd:** `--ruleset` / `--print-ruleset` (YAML); removed the layered-config flags.

## [0.3.0](https://github.com/dknathalage/gruntcmt/compare/v0.2.0...v0.3.0) (2026-08-14)

### Features

* **cmd:** `--out gh` posts/updates the PR comment directly via the GitHub REST API (marker-based upsert); added a reusable composite action and PR demo workflow.

## [0.2.0](https://github.com/dknathalage/gruntcmt/compare/v0.1.1...v0.2.0) (2026-08-14)

### Features

* reusable GitHub Action + a real (cloud-free) terragrunt example.

## [0.1.1](https://github.com/dknathalage/gruntcmt/compare/v0.1.0...v0.1.1) (2026-08-14)

### Bug Fixes

* **cmd:** report the real build version via `runtime/debug.ReadBuildInfo`; documented mise install.

## 0.1.0 (2026-08-14)

### Features

* initial gruntcmt: Terraform plan JSON on stdin → GitHub-markdown summary on stdout (pure Unix filter).
