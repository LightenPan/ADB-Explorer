# Repository Guidelines

## Project Structure & Module Organization

SwitchHosts is a Go desktop app using MyGo's native UI. Root Go files bootstrap the window, tray, hosts bridge, capture, and diagnostics. Internal packages live under `internal/`:

- `internal/appui/` contains the interface, dialogs, actions, and icons.
- `internal/store/` owns the data layout, manifest, entries, preferences, transactions, and recovery.
- `internal/hostswrite/` aggregates hosts content, handles privileged writes, and records history.
- `internal/i18n/` contains translations in `languages.json` and `native.go`.

Application and installer assets live in `assets/`; Windows packaging code is in `cmd/package-windows/`. Build artifacts and local data directories are not source files.

## Testing Rules (Required)

1. **Never write unit tests after writing code.**
2. **Strongly prefer E2E tests as the sole testing mechanism.** Verify complex features through the real app's user-facing workflow. End each E2E test with a verifiable, reproducible artifact (for example, a screenshot, log, or report) and document how to regenerate it.
3. **If a system must be isolated for testing, first write down all the ways it could fail, then write the code.** Use that list to define verification; never isolate a system without it.

Headless renders and `SWH_SNAPSHOT_DIR` output are not E2E evidence. Run permission and elevation tests on the affected OS with the real app. Legacy tests may remain, but new work should add E2E coverage, not broader unit tests.

## Build and Development Commands

```bash
go run .                         # Start the app
go build -o SwitchHosts.exe .    # Build a local executable
go vet ./...                     # Run Go's static checks
gofmt -w .                       # Format Go sources
```

Use `go run .` or the built executable for E2E verification.

Windows installer:

```bash
go run github.com/egoist/mygo/cmd/mygo@v0.6.0 build -platform windows/amd64
go run ./cmd/package-windows
```

## Coding Style & Naming Conventions

Use standard Go formatting, tabs, idiomatic naming, and short package-private identifiers. Document exported declarations. Name platform files `_windows.go`, `_darwin.go`, and `_linux.go`. UI state belongs in `appui`; persistence in `store`; system hosts access in `hostswrite`. Do not bypass those boundaries.

## Commit & Pull Request Guidelines

Use Conventional Commits (`feat(scope): ...`, `fix(scope): ...`, `chore(deps-dev): ...`) with an imperative, focused subject.

Pull requests should explain the problem and solution, link related issues, and include the E2E command, artifact, and regeneration steps. Show artifacts for visible UI changes. Call out data-format, migration, or elevation changes explicitly. Do not commit generated `build/` artifacts, binaries, user data, or temporary E2E outputs.
