# ADB Explorer

Native Go client under `go/` (MyGo via `replace` to `third/mygo`). `third/mygo` is a nested git repo — do not edit it for app features.

## Testing

- After writing code, never write unit tests.
- Strongly prefer E2E tests as the only test mechanism. Use them to verify that a complex feature actually works. At the end of an E2E test, produce a verifiable, repeatable artifact (a PNG of the tester frame, a log of a live `adb` run, or an equivalent output a human can inspect).
- If you must test a system in isolation, first write down every way it can fail, then write the code. Keep an isolated test only when an E2E of the real UI would miss that failure (wire protocol, cancellation races, queue ordering, path/quoting injection, parser edge cases). Do not add tests that restate helpers, default counts, or formatting.

Go UI: drive through `ui.NewTester` and write a PNG under `go/cmd/adbexplorer/testdata/`. Do not add `*_test.go` files that only call a pure helper and compare a string or a length.

## Scope

- Do not commit unless asked.
