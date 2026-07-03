# Changelog

Mainline is in public alpha. Until `v1.0.0`, release notes should optimize for
clear user impact over strict semver guarantees: call out workflow changes,
schema/config changes, migration notes, and known alpha limitations explicitly.

This project follows the spirit of [Keep a Changelog](https://keepachangelog.com/)
and uses semver-style versions once tags are published.

## [Unreleased]

### Added

- _Template: new features, commands, integrations, or user-visible workflow
  additions._

### Changed

- _Template: behavior changes, workflow changes, docs repositioning, or
  compatibility-affecting updates._

### Fixed

- _Template: bug fixes, correctness fixes, reliability improvements, or reduced
  false positives._

### Security

- _Template: vulnerability fixes, hardening, dependency updates, secret-handling
  changes, or disclosure-process updates._

### Migration Notes

- _Template: actions users or maintainers should take after upgrading._

### Known Alpha Limits

- _Template: intentionally accepted limitations, unstable schemas, incomplete
  integrations, or non-blocking risks relevant to this release._

## [0.5.0] - 2026-07-03

### Added

- Pi is now a first-class Mainline hook agent. `mainline hooks list-agents`
  includes `pi`, and `mainline hooks install --agent pi` writes a managed
  repo-local extension at `.pi/extensions/mainline.ts`.
- Pi lifecycle events now map into Mainline hook events so Pi sessions can
  receive automatic session-start and per-prompt Mainline context.
- `mainline init` installs Pi hooks by default for new repositories, and the
  default Mainline skill installation path now includes `--agent pi`.

### Changed

- README.md and README.zh now lead with Git-native intent memory positioning,
  add demo videos, and simplify the top-of-page media hierarchy.
- Hook documentation now describes Cursor, Codex, Claude Code, and Pi as the
  supported hook runtimes.

### Fixed

- Local-dev hook wrappers for Codex, Claude Code, Cursor, and Pi now fail softly
  when `go run` cannot build Mainline, instead of blocking the host agent.
- `mainline hooks status` now reports a repair reason when local-dev mode finds
  a Go version below Mainline's minimum supported version.
- `mainline doctor --setup` now explains that it must run inside a Git
  repository and points fresh installs to `mainline version` for CLI validation.
- Retrieval status reachability property tests now use explicit witnesses for
  each status branch, avoiding low-sample Full PBT flakes.

### Migration Notes

- Existing repositories that want Pi hook support should run
  `mainline hooks install --agent pi` or `mainline hooks install` after
  upgrading. Pi may need to trust, reload, or restart the project session before
  it loads the repo-local extension.

### Known Alpha Limits

- Hooks remain context providers only. They do not auto-start intents,
  auto-append progress, auto-seal work, or make semantic workflow decisions.

## Release Notes Template

Use this checklist when drafting a GitHub Release:

````markdown
## Mainline <version>

One sentence: what changed for users and why this release exists.

### Highlights

- ...

### Install

```bash
curl -fsSL https://raw.githubusercontent.com/mainline-org/mainline/main/install.sh | MAINLINE_VERSION=<version> bash
```

Prebuilt archives and `checksums.txt` are attached to this release.

### Upgrade Notes

- ...

### Validation

- `make ci-release`
- `govulncheck ./...`
- install script smoke test on macOS/Linux

### Known Alpha Limits

- ...
````
