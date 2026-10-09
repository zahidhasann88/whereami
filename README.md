# whereami

A one-screen summary of the project you are standing in: its type, Git state, runtime versions, listening ports, containers and configuration files.

```sh
cd ~/code/shop-web
whereami
```

Read-only, fast (sections run concurrently and each has a time budget), and designed so that a missing tool or a failing section never stops the rest of the summary.

[![CI](https://github.com/zahidhasann88/whereami/actions/workflows/ci.yml/badge.svg)](https://github.com/zahidhasann88/whereami/actions/workflows/ci.yml)

---

## Contents

- [Install](#install)
- [Example](#example)
- [Commands and flags](#commands-and-flags)
- [What each section shows](#what-each-section-shows)
- [JSON output](#json-output)
- [Exit codes](#exit-codes)
- [Environment files and security](#environment-files-and-security)
- [Decisions and limitations](#decisions-and-limitations)
- [Roadmap](#roadmap)
- [Contributing](#contributing)
- [Licence](#licence)

## Install

**With Go 1.22 or newer:**

```sh
go install github.com/zahidhasann88/whereami/cmd/whereami@latest
```

**Release binaries:** download the archive for your platform from the [Releases page](https://github.com/zahidhasann88/whereami/releases). Archives are published for Linux, macOS and Windows on amd64 and arm64. Verify with the `checksums.txt` file attached to each release.

**From source:**

```sh
git clone https://github.com/zahidhasann88/whereami
cd whereami
make build      # writes bin/whereami
make install    # installs into $(go env GOPATH)/bin
```

On Windows PowerShell, build and run without Make:

```powershell
go build -o bin/whereami.exe ./cmd/whereami
.\bin\whereami.exe
```

whereami has two third-party dependencies: [spf13/cobra](https://github.com/spf13/cobra) for the command line and [shirou/gopsutil/v3](https://github.com/shirou/gopsutil) for process and socket information. Git is called as the `git` binary; Docker is queried over its HTTP API. There is no Docker SDK dependency.

## Example

Real output from this tool, run in a demo project (a Next.js app with a real git history, a listener on port 3000, and a `.env` file that is not gitignored). It was captured from a pipe, so it has no colour or symbols. In a terminal the same lines use colour and ✓ / ⚠ markers.

```
whereami 0.1.0 - /tmp/demo-shop

Project
  Path        /tmp/demo-shop
  Root        /tmp/demo-shop (git root)
  Name        shop-web
  Types       Node.js, TypeScript
  Frameworks  Next.js
  Managers    pnpm
  Manifests   package.json

Git
  Branch      main (no upstream)
  State       dirty - 1 staged, 1 modified, 2 untracked
  Last commit 14b28b7 "feat: add api stub" - 51 seconds ago by Jane_Doe
  Remote      https://github.com/acme/shop.git (origin)
  Stashes     1

Runtime
  node      20.20.2            [warn] pinned 18 by .nvmrc - mismatch
                               [ok] pinned >=18 <21 by package.json (engines.node)
  npm       10.8.2
  pnpm      not installed      [warn] pinned 8.6.0 by package.json (packageManager) - not installed

Ports
  :3000   python3    pid 18559   127.0.0.1

Containers
  Compose     docker-compose.yml (project shop; services web, db)

Config
  Env         .env - 3 variables - [warn] NOT gitignored
  Env         .env.example - 1 variable - template - committed
  Compose     docker-compose.yml
  Dockerfile  Dockerfile
  Makefile    Makefile
  CI          .github/workflows/ci.yml
  [warn] .env is not gitignored; add it to .gitignore before committing
```

Notice what is absent: the values in `.env` (including the database password and the secret key), and the token that was embedded in the git remote. Docker is not installed on this machine, so the containers section shows only the Compose file. A container section with no Docker reachable is silent by design.

## Commands and flags

```
whereami [section] [flags]
```

| Command | Output |
| --- | --- |
| `whereami` | Full summary of all six sections |
| `whereami <section>` | Only that section. One of `project`, `git`, `runtime`, `ports`, `containers`, `config` |

| Flag | Default | Meaning |
| --- | --- | --- |
| `--json` | off | Print the [JSON schema](#json-output) instead of text |
| `--path <dir>` | current directory | Inspect another directory |
| `--no-color` | off | Disable colour (colour is also off when output is not a terminal, and when `NO_COLOR` is set) |
| `--no-docker` | off | Do not contact Docker at all |
| `--show-env-keys` | off | List environment variable **names** (never values) |
| `--timeout <duration>` | `2s` | Time budget for each section |
| `--version` | | Print version, commit and build date |
| `-h`, `--help` | | Help |

Environment variables read by whereami:

- `NO_COLOR` (any non-empty value): disables colour, per [no-color.org](https://no-color.org).
- `TERM=dumb`: disables colour and symbols.
- `DOCKER_HOST`: honoured when it is `unix://` or `npipe://`. Other schemes are reported as Docker unavailable.

Output is coloured and uses symbols only when stdout is an interactive terminal. Piped and redirected output is plain ASCII.

## What each section shows

Every section runs concurrently with its own time budget. When a section cannot produce data, the summary says so on one line with a short reason, such as `unavailable - not a git repository`, or `timed out after 2s`. Other sections are unaffected.

### Project

- **Path**: the directory inspected.
- **Root**: the nearest directory, walking up from the current directory, that contains a `.git` entry or a project marker (`go.mod`, `package.json`, `pyproject.toml`, `requirements.txt`, `Pipfile`, `Cargo.toml`, `pom.xml`, `build.gradle(.kts)`, `composer.json`, `Gemfile`, `setup.py`, `*.csproj`, `*.sln`). The root is labelled `git root` or `project marker`. If nothing is found, the directory itself is used.
- **Name**: from `go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml` or `composer.json`, when present.
- **Types**: Go, Node.js, TypeScript (`tsconfig.json` or a `typescript` dependency), Python, Rust, Java (Maven or Gradle), PHP, Ruby, .NET.
- **Frameworks**: found from manifests. Go: Gin, Echo, Fiber, chi. Node: Next.js, NestJS, Express, Fastify, Nuxt, SvelteKit, Angular, Vue. Python: Django, FastAPI, Flask. Rust: Axum, Actix Web, Rocket. Java: Spring Boot. PHP: Laravel, Symfony. Ruby: Rails. .NET: ASP.NET Core.
- **Managers**: from lockfiles and manifests, for example `pnpm`, `yarn`, `npm`, `bun`, `uv`, `poetry`, `pipenv`, `cargo`, `maven`, `gradle`, `composer`, `bundler`.

### Git

- **Branch** with its upstream and ahead/behind counts, or `detached at <hash>`, or `(no commits yet)`.
- **State**: `clean`, or `dirty` with counts of conflicted, staged, modified and untracked files.
- **Last commit**: short hash, subject, relative time, and author.
- **Remote**: `origin` if it exists, otherwise the first remote. Credentials are removed from the URL (`https://user:token@host/…` is shown as `https://host/…`).
- **Stashes**: the number of stash entries.

Read-only git commands run with `GIT_OPTIONAL_LOCKS=0`, so whereami does not take index locks.

### Runtime

Shows the installed version of each runtime that is relevant to the project. The relevant set comes from the detected types and package managers, plus anything a pin file mentions.

Runtimes probed: `go`, `node`, `npm`, `pnpm`, `yarn`, `bun`, `python`, `rustc`, `java`, `php`, `ruby`, `dotnet`.

Pins read from project files:

| Source | Runtime | How it is compared |
| --- | --- | --- |
| `go.mod` `go` directive | go | Installed version must be at least the directive |
| `.nvmrc`, `.node-version` | node | Prefix match for numeric pins (`18` matches 18.x.y) |
| `package.json` `engines.node` | node | Semver-style range |
| `package.json` `packageManager` | pnpm, yarn, bun, npm | Exact or prefix match |
| `.python-version` | python | Prefix match |
| `pyproject.toml` `requires-python` | python | PEP 440-style range |
| `rust-toolchain`, `rust-toolchain.toml` | rustc | Prefix match for numeric channels |
| `Cargo.toml` `rust-version` | rustc | At least this version |
| `composer.json` `require.php` | php | Composer-style range |
| `.ruby-version` | ruby | Prefix match |
| `.java-version` | java | Prefix match |
| `global.json` `sdk.version` | dotnet | Major.minor match |

Each pin shows one of four results: `[ok] pinned … ` (match), `[warn] … - mismatch`, `[warn] … - not installed`, or `[?] … (cannot compare)` when the pin uses syntax whereami does not evaluate (for example `lts/*` or `stable`). A cannot-compare pin is never reported as a mismatch.

### Ports

TCP sockets in the `LISTEN` state, owned by processes whose working directory is inside the project root. Listeners that share a port and process (IPv4 and IPv6, for example) are merged. The line shows port, process name, PID and bind address.

If some listeners belong to processes whose working directory cannot be read (typically other users' processes on Linux and macOS), the count is shown and they are not guessed at. If every listener is unreadable, the section reports itself unavailable.

### Containers

- **Compose**: `docker-compose.*`, `compose.*` (`.yml` or `.yaml`) in the project root, with the compose project name and service names. Service names come from a small line-based reader, not a full YAML parser.
- **Running**: containers from `GET /containers/json` that belong to the project. A container belongs to the project when its `com.docker.compose.project.working_dir` label is inside the root. If that label is absent, it belongs when its `com.docker.compose.project` label matches a project defined by a Compose file in the root.

If Docker is not reachable, the Running line is omitted without any message. If there is also no Compose file, the whole section is hidden.

### Config

- **Env files**: `.env` and `.env.*` in the project root. Shown with the number of variables, and whether git ignores them.
- **Config files**: `config.yaml`, `config.yml`, `config.json`, `config.toml`.
- **Compose, Dockerfile, Makefile**: `docker-compose.*`, `compose.*`, `Dockerfile`, `Dockerfile.*`, `*.Dockerfile`, `Makefile`, `makefile`, `GNUmakefile`.
- **CI**: `.github/workflows/*.yml|yaml`, `.circleci/config.yml`, `.gitlab-ci.yml`, `.travis.yml`, `azure-pipelines.yml`, `bitbucket-pipelines.yml`, `.drone.yml`, `Jenkinsfile`.

Env files named `*.example`, `*.sample`, `*.template` or `*.dist` are treated as templates and are not checked for ignore status.

For every non-template env file, whereami runs `git check-ignore` and `git ls-files`. It warns when the file is:

- `[warn] NOT gitignored`: git would commit it.
- `[warn] tracked by git`: it is already in the index, so ignoring it now is not enough.

Outside a git repository, the status is shown as `not in a git repository` and no warning is raised.

## JSON output

`whereami --json` prints one JSON document to stdout. Its shape is versioned by `schema_version`. Version 1 is described here.

```jsonc
{
  "schema_version": 1,              // integer; bumped only on breaking changes
  "tool": "whereami",
  "version": "0.1.0",               // build version
  "generated_at": "2026-10-09T18:44:38Z",   // RFC 3339, UTC
  "path": "/tmp/demo-shop",         // absolute directory inspected
  "sections": [                     // always all six, in this order, unless one was requested
    { "name": "project", "status": "ok", "data": { … } },
    { "name": "git", "status": "unavailable", "reason": "not a git repository" },
    { "name": "runtime", "status": "timed_out", "reason": "timed out after 2s" },
    …
  ]
}
```

Each section has:

| Field | Type | Present | Meaning |
| --- | --- | --- | --- |
| `name` | string | always | `project`, `git`, `runtime`, `ports`, `containers`, `config` |
| `status` | string | always | `ok`, `unavailable`, `timed_out`, or `skipped` |
| `reason` | string | when not `ok` | Short human-readable explanation |
| `data` | object | when `ok` | Section payload, below |

`skipped` appears for `containers` when Docker is unreachable and no Compose file exists. Text output omits skipped sections; JSON keeps them.

### `project` data

```json
{
  "dir": "/tmp/demo-shop",
  "root": "/tmp/demo-shop",
  "root_source": "git",
  "name": "shop-web",
  "types": ["Node.js", "TypeScript"],
  "frameworks": ["Next.js"],
  "package_managers": ["pnpm"],
  "manifests": ["package.json"]
}
```

`root_source` is `git`, `marker`, or `directory`. `name` is omitted when unknown. The four list fields are always present and may be empty.

### `git` data

```json
{
  "branch": "main",
  "detached": false,
  "unborn": false,
  "head": "14b28b7",
  "upstream": "origin/main",
  "ahead": 2,
  "behind": 0,
  "clean": false,
  "staged": 1,
  "modified": 1,
  "untracked": 2,
  "conflicted": 0,
  "last_commit": {
    "hash": "14b28b7",
    "subject": "feat: add api stub",
    "relative_time": "51 seconds ago",
    "date": "2026-10-09T18:43:46Z",
    "author": "Jane_Doe"
  },
  "remote": { "name": "origin", "url": "https://github.com/acme/shop.git" },
  "stashes": 1
}
```

`branch` is omitted when detached. `head` is omitted when there are no commits. `upstream` is omitted when none is configured, and in that case `ahead` and `behind` are `0` and must be ignored. `last_commit` and `remote` are omitted when absent. `date` is ISO 8601 with the commit's offset.

### `runtime` data

```json
{
  "runtimes": [
    {
      "name": "node",
      "version": "20.20.2",
      "pins": [
        { "source": ".nvmrc", "expr": "18", "status": "mismatch" },
        { "source": "package.json (engines.node)", "expr": ">=18 <21", "status": "match" }
      ]
    },
    { "name": "python", "reason": "not installed" }
  ],
  "warnings": ["node 20.20.2 does not satisfy 18 (.nvmrc)"]
}
```

`version` is omitted when the runtime is not installed or its version could not be read; `reason` explains why. Pin `status` is one of `match`, `mismatch`, `missing` (pinned but not installed), or `unknown` (cannot compare). `pins` is omitted when there are none.

### `ports` data

```json
{
  "listening": [
    { "port": 3000, "proto": "tcp", "addresses": ["127.0.0.1", "::1"], "pid": 18559, "process": "python3", "cwd": "/tmp/demo-shop" }
  ],
  "inaccessible": 0
}
```

`inaccessible` counts listeners whose working directory could not be read.

### `containers` data

```json
{
  "docker_available": true,
  "compose_files": [
    { "path": "docker-compose.yml", "project": "shop", "services": ["web", "db"] }
  ],
  "running": [
    {
      "id": "9f1c2a7b3d4e",
      "name": "shop-web-1",
      "image": "shop-web:latest",
      "state": "running",
      "status": "Up 3 hours",
      "compose_project": "shop",
      "compose_service": "web",
      "ports": ["0.0.0.0:8080->8000/tcp"],
      "matched_by": "working_dir"
    }
  ]
}
```

`matched_by` is `working_dir` or `compose_project`. `compose_project` and `compose_service` are omitted when the labels are absent. `docker_available` is `false` when Docker could not be reached, in which case `running` is empty.

### `config` data

```json
{
  "env_files": [
    { "name": ".env", "variables": 3, "gitignore": "not_ignored" },
    { "name": ".env.example", "variables": 1, "template": true },
    { "name": ".env.local", "variables": 2, "gitignore": "ignored", "keys": ["API_URL", "DEBUG"] }
  ],
  "files": [
    { "path": "docker-compose.yml", "kind": "compose" },
    { "path": ".github/workflows/ci.yml", "kind": "ci" }
  ],
  "warnings": [".env is not gitignored; add it to .gitignore before committing"]
}
```

`gitignore` is one of `ignored`, `not_ignored`, `tracked`, or `unknown`, and is omitted for templates. `keys` appears only with `--show-env-keys`. `kind` is one of `config`, `compose`, `dockerfile`, `makefile`, `ci`.

### Stability promise

Within a `schema_version`, fields are only added, never removed or renamed, and enum values are only added. Consumers should ignore unknown fields. A breaking change increments `schema_version` and is listed in the changelog.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success. Missing data is never an error, so `0` is returned for a directory with no project, no git, no Docker, or no listening ports. |
| `2` | Usage error: unknown flag or section, more than one section, `--path` that does not exist or is not a directory, `--timeout` of zero or less. |
| `3` | Unexpected runtime error, such as a failure to write output. |

## Environment files and security

whereami is built to be safe to run in any directory, including ones that hold secrets.

- **Env values are never printed.** For `.env`-style files, whereami reads each line only far enough to find the name before `=`. The value is discarded immediately. No value is stored in any structure that is rendered, so no value can reach text or JSON output.
- **Names only, and only on request.** Variable names are listed only with `--show-env-keys`. Names are the only thing that flag reveals.
- **Remote credentials are removed** before the URL is shown. This covers `https://user:pass@…` and `https://token@…`, and `ssh://user:pass@…`. SCP-style `git@host:path` URLs have no secret, so they are unchanged.
- **Docker is read-only.** The only request is `GET /containers/json`.
- **Git is read-only.** whereami runs only `rev-parse`, `status`, `log`, `remote`, `stash list`, `ls-files` and `check-ignore`, with `GIT_OPTIONAL_LOCKS=0` and `GIT_TERMINAL_PROMPT=0`.
- **No network access** except the local Docker socket.

The test suite plants recognisable secret strings in `.env` files and git remote URLs, and asserts that they never appear in text or JSON output, with and without `--show-env-keys`. The CI workflow runs the suite on Linux, macOS and Windows.

To report a security issue, see [SECURITY.md](SECURITY.md).

## Decisions and limitations

These were decided for v0.1. They are listed so you can check them against your expectations.

**Detection**

- **Root discovery** walks upward from the current directory and stops at the first directory with a `.git` entry or a project marker. A marker in a subdirectory therefore wins over the git root above it. This makes sense in a monorepo when you run whereami inside one package.
- **Type detection is shallow.** Only the root directory's files are inspected. whereami does not recurse into subprojects. Point `--path` at a subproject to inspect it.
- **Frameworks come from manifests**, using substring or name matching. This is intentionally simple and can give false positives, for example a commented-out dependency in `pyproject.toml`.

**Runtime**

- **Go's `go` directive is a minimum.** The installed toolchain must be at least that version. The `toolchain` line is not used as a pin.
- **Pins are compared by syntax.** Numeric pins are prefix-matched (`18` means 18.x.y). Syntax that is not evaluated, such as `lts/*`, `stable` or hyphen ranges, is reported as `cannot compare` rather than as a mismatch.
- **Version probes** run `<tool> --version` (or the equivalent) with a 1.5 second budget each. Python tries `python3` then `python`. Java reads `java -version` from stderr.
- **`npm` is probed for every Node project**, because npm is the default package manager. `pnpm`, `yarn` and `bun` are probed only when they are pinned or detected from a lockfile.

**Ports**

- **Ownership is by working directory.** A server whose process is running inside the project is attributed to it. A server started from another directory, even one that serves project files, is not.
- **Windows** uses gopsutil's process lookup to read working directories. Protected processes return no directory, and they are counted as inaccessible rather than attributed.
- **Linux and macOS** may not expose other users' processes. Run with sufficient privileges to see them.

**Containers**

- **Compose project names** come from a top-level `name:` in the compose file, or else from the directory name, normalised the way Compose does. A `COMPOSE_PROJECT_NAME` set in `.env` is not read, because doing so would require reading an env value.
- **Docker connection** uses the default socket, `/var/run/docker.sock` on Unix or `\\.\pipe\docker_engine` on Windows, or `DOCKER_HOST` when it is `unix://` or `npipe://`. TCP and TLS endpoints are not supported in v0.1.
- **Only running containers** are listed (the Engine API default). Stopped containers are not shown.

**Config**

- **Env-variable counting** counts lines that look like `KEY=value` (optionally prefixed with `export`). Multi-line quoted values can be miscounted.
- **Gitignore checks use git's own rules** through `git check-ignore --no-index`, so global excludes and `.git/info/exclude` are respected. Checking happens only for files in the project root, not in subdirectories.

**General**

- **Sections run in parallel** with a per-section budget (default 2 seconds). A section that exceeds its budget is reported as `timed out`; its goroutine is abandoned, and the process exits normally.
- **Colour and symbols** are used only on an interactive terminal, and only when `NO_COLOR` is unset and `TERM` is not `dumb`.
- **Text output omits skipped sections**, so the Containers section does not appear at all when Docker is off and no Compose file exists.
- **`generated_at` in JSON** uses UTC.
- **Section interface.** `Section` has `Name()` and `Collect(ctx, env)`. Detection happens inside `Collect`, so there is no separate `Detect` method.

**Testing limits**

- The Docker client is tested against a fake Engine API served on a real unix socket, and by JSON fixtures. It has not been tested against a live Docker daemon in CI. Expect rough edges with unusual daemon setups.
- The Windows named-pipe transport is compiled and vetted for Windows, but it has not been run against a live Docker Desktop daemon.

## Performance

On a typical repository, a full run takes well under a second. In the demo above it took about 0.12 s on the first run. Git calls are the largest cost (about six short subprocesses). Everything else is file reads and one socket listing.

## Roadmap

- **Plugin sections.** The section registry (`internal/sections`) is the seam for external sections. A future version could load plugins that implement the same `Section` interface, either as Go plugins or as executables speaking JSON over stdio. Neither is part of v0.1. The current API is internal and may change until a v1.0 is released.
- **Monorepo mode**: inspect every project under the root.
- **More runtime and framework rules**, driven by fixtures.
- **Live Windows Docker test.** Add a CI job with a real Docker Desktop daemon to exercise the named-pipe transport end to end.
- **Compose `.env` project name** via `COMPOSE_PROJECT_NAME`, if we can read it without ever exposing its value.

Ideas and proposals are welcome as issues.

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) for the development setup, the checks every change must pass, and a step-by-step guide to adding a new section. Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md).

```sh
make check   # gofmt, go vet, go test
make lint    # golangci-lint v1.64.8
```

## Licence

MIT. See [LICENSE](LICENSE).
