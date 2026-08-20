# Changelog

## [0.5.1](https://github.com/dknathalage/gruntcmt/compare/v0.5.0...v0.5.1) (2026-08-20)


### Bug Fixes

* remove docs ([269b3db](https://github.com/dknathalage/gruntcmt/commit/269b3db71b5fe86ac7c9898fc7e0ddfc30cf56dc))
* remove docs ([ffbceec](https://github.com/dknathalage/gruntcmt/commit/ffbceec924f69cc51c1e69bdff142a7c7010fe90))

## [0.5.0](https://github.com/dknathalage/gruntcmt/compare/v0.4.2...v0.5.0) (2026-08-20)


### Features

* **ghctx:** add repo/pr/commit/token/scope detection ([394e7c9](https://github.com/dknathalage/gruntcmt/commit/394e7c99569927b799ded3ba1a485343603e79be))
* **input:** add path/directory plan reader ([9e0d05e](https://github.com/dknathalage/gruntcmt/commit/9e0d05eae03337ea68cb0079db6ce2c8e2b799a3))
* pure-CLI cutover — path args, auto-detect, --out, drop base/stdin ([a8126db](https://github.com/dknathalage/gruntcmt/commit/a8126dbba313a4cdf5c2e0d54360d79fe32d2493))
* **ruleset:** add built-in Default ruleset ([de55cf9](https://github.com/dknathalage/gruntcmt/commit/de55cf9e546b3e3b39278407fb5e01ee19e97e6a))

## [0.4.2](https://github.com/dknathalage/gruntcmt/compare/v0.4.1...v0.4.2) (2026-08-15)


### Bug Fixes

* Merge pull request [#5](https://github.com/dknathalage/gruntcmt/issues/5) from dknathalage/fix/stale-action-and-example-workflow ([b18f3f3](https://github.com/dknathalage/gruntcmt/commit/b18f3f3e6a71b6533d7f315792c871a83034ec59))
* repair stale composite action + example workflow, pluralize unit count ([b18f3f3](https://github.com/dknathalage/gruntcmt/commit/b18f3f3e6a71b6533d7f315792c871a83034ec59))
* repair stale composite action and example workflow; pluralize unit count ([622829a](https://github.com/dknathalage/gruntcmt/commit/622829a533305116613239912b78b4049e88a394))

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
