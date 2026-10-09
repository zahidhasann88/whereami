# Contributing to whereami

Thanks for helping out. This guide covers setting up a development environment,
the checks every change must pass, and how to add a new section.

## Development setup

You need Go 1.22 or newer (CI tests 1.22 and 1.23). Git is needed for the git
section tests, which create real temporary repositories.

```sh
git clone https://github.com/zahidhasann88/whereami
cd whereami
make build        # bin/whereami
make check        # gofmt, go vet, go test
make lint         # golangci-lint
```

Install golangci-lint at the version CI uses:

```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
```

## Before you open a pull request

1. `make check` passes.
2. `golangci-lint run ./...` passes.
3. New behaviour has tests. Bug fixes include a test that fails without the fix.
4. If text or JSON output changed on purpose, regenerate the golden files and review the diff:
   `go test ./internal/render -update`. The golden tests are `TestTextGolden`, `TestJSONGolden`
   and their degraded-state variants.
5. If JSON output changed, update the schema in the README and the `schema_version`
   rules below.
6. Output must never contain environment variable values, tokens or credentials.
   Fixtures must not contain real secrets either.

Commits follow [Conventional Commits](https://www.conventionalcommits.org/):
`feat:`, `fix:`, `test:`, `docs:`, `chore:`, `refactor:`, `ci:`.

## Project layout

```
cmd/whereami/          main package: wires build info and calls internal/cli
internal/cli/          flags, argument validation, exit codes, dependency wiring
internal/env/          interfaces for the outside world (Runner, FS, ProcessSource, DockerAPI) and Env
internal/env/envtest/  fakes for those interfaces, used by tests
internal/system/       production implementations (os/exec, os, gopsutil, Docker socket client)
internal/sections/     the Section interface, registry, runner, and every built-in section
internal/render/       text and JSON renderers, golden tests
```

Sections never call `os`, `os/exec` or the network directly. They use the `*env.Env`
they are given, which is what makes them testable without a real system.

## How to add a new section

Suppose you want a `deps` section that reports outdated dependency files.

1. **Create the file** `internal/sections/deps.go`:

   ```go
   package sections

   import (
       "context"

       "github.com/zahidhasann88/whereami/internal/env"
   )

   // Deps reports dependency manifests in the project root.
   type Deps struct{}

   // Name implements Section. It is the name users type on the command line.
   func (Deps) Name() string { return "deps" }

   // DepsInfo is the data of the deps section. Give fields JSON tags; they
   // become part of the public --json schema.
   type DepsInfo struct {
       Lockfiles []string `json:"lockfiles"`
   }

   // Collect implements Section. Return StatusUnavailable with a short Reason
   // when the data cannot be obtained; return StatusSkipped when there is
   // nothing to show. Errors returned here are converted into "unavailable".
   func (Deps) Collect(ctx context.Context, e *env.Env) (Result, error) {
       if err := ctx.Err(); err != nil {
           return Result{}, err
       }
       root, _ := findRoot(e.FS, e.Dir)
       if root == "" {
           root = e.Dir
       }
       var found []string
       for _, name := range []string{"go.sum", "package-lock.json", "Cargo.lock"} {
           if _, err := e.FS.Stat(root + "/" + name); err == nil {
               found = append(found, name)
           }
       }
       return Result{Status: StatusOK, Data: DepsInfo{Lockfiles: nonNil(found)}}, nil
   }
   ```

   Use `filepath.Join` rather than string concatenation in real code.

2. **Register it** in `Builtin()` in `internal/sections/section.go`. Display order
   is the order in that list.

3. **Render it** in `internal/render/text.go`: add a `case sections.DepsInfo:` to the
   type switch in `Text`, a title in `sectionTitle`, and a `writeDeps` function.
   The renderer must handle the empty case.

4. **Test it.** Put fixtures under `internal/sections/testdata/`. Use
   `envtest.Runner` for commands and `envtest.Processes` / `envtest.Docker` for
   their interfaces. Cover at least: the happy path, the not-applicable path
   (`StatusSkipped` or `unavailable` with a reason), and a runner error.

5. **Golden files.** Add the section to `sampleReport()` in `internal/render/render_test.go`
   and run `go test ./internal/render -update`. Review the new golden files.

6. **Document it.** Add a subsection to the README under "What each section shows" and
   describe its JSON `data` shape in the schema section.

7. **Changelog.** Add a line under `[Unreleased]` in `CHANGELOG.md`.

Guidelines:

- Keep detection cheap. A section should read a handful of files, not walk the whole tree.
- Prefer deterministic output: sort lists, avoid map iteration order in output.
- Never store or print values from `.env`-style files. Names only, and only behind
  `--show-env-keys`.
- A section must not exceed its time budget by design. Pass `ctx` to every command
  and honour cancellation.

## Adding a new project type or framework

Detection rules live in `internal/sections/project.go` (`detectProject`). Add
a fixture directory under `internal/sections/testdata/projects/<name>` and a row
in `TestDetectProjectFixtures`.

## Changing the JSON schema

- Adding a field is backwards compatible: keep `schema_version` at its current value.
- Removing a field, renaming it, or changing its meaning requires bumping
  `render.SchemaVersion` and a changelog entry under "Changed" or "Removed".

## Reporting security issues

Do not open a public issue. See [SECURITY.md](SECURITY.md).

## Code of conduct

Participation is governed by the [Code of Conduct](CODE_OF_CONDUCT.md).
