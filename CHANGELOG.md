# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-10-10

First public release.

### Added

- `whereami` prints a summary of the directory it runs in, with six sections:
  project, git, runtime, ports, containers and config.
- Run a single section with `whereami <section>`.
- Project detection for Go, Node.js, TypeScript, Python, Rust, Java (Maven and
  Gradle), PHP, Ruby and .NET, with framework detection for common frameworks
  (for example NestJS, Next.js, Django, FastAPI, Flask, Express, Spring Boot).
- Git state: branch, upstream with ahead/behind counts, staged/modified/untracked
  counts, last commit, remote URL with credentials removed, and stash count.
  Handles non-repositories, detached HEAD, unborn branches and missing remotes.
- Runtime versions for the runtimes relevant to the project, the version each
  project file pins (`go.mod`, `.nvmrc`, `.node-version`, `engines.node`,
  `packageManager`, `.python-version`, `requires-python`, `rust-toolchain`,
  `rust-version`, `composer.json`, `.ruby-version`, `.java-version`,
  `global.json`), and mismatch detection.
- Listening TCP ports owned by processes whose working directory is inside the
  project.
- Docker Compose files in the project and running containers that belong to the
  project, matched by compose project label or working-dir label. Docker is
  queried over the Engine HTTP API on the local socket or named pipe.
- Config inventory: `.env` files (variable counts only), config files,
  Dockerfiles, Makefiles and CI definitions. Warns when an env file is not
  gitignored or is tracked by git.
- Flags: `--json`, `--no-color`, `--no-docker`, `--show-env-keys`, `--path`,
  `--timeout`, `--version`.
- Stable JSON output with `schema_version`, documented in the README.
- Sections run concurrently with a per-section timeout (default 2s). A failing,
  panicking or slow section is reported as unavailable or timed out and does not
  affect the others.
- Respects `NO_COLOR`, `TERM=dumb`, and disables colour and symbols when output
  is not a terminal.
- Builds for Linux, macOS and Windows on amd64 and arm64.

### Security

- Environment files are never read for their values in output. Only variable
  names are listed, and only with `--show-env-keys`.
- Credentials in git remote URLs are stripped before display.
