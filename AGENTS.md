# AGENTS.md — EuroKey

Guidance for agents/contributors working in this repo.

## What this is

Windows-only accent keyboard: **dead-key / compose** input for **Deutsch,
Français, Español**, so a US/UK layout can type `ä ö ü ß`, `é è ê ç`, `ñ ¿ ¡`.

Fork of [Tui Gõ](https://github.com/captajn/TuiGoTiengViet): it reuses that
project's Win32 shell (low-level keyboard hook, tray, HUD, settings window) and
replaces the Vietnamese engine with a language-neutral compose engine. The Tui Gõ
engine is itself a port of UniKey/x-unikey — keep the attribution in mind when
redistributing.

## Layout

```
.                 Go package main — Win32 shell (Windows only)
engine/           pure-Go compose engine + tests, no Windows deps
e2e/              separate module (go-rod) end-to-end typing harness
scripts/          check-eurokey.ps1, e2e-eurokey.ps1
```

Shell files: `main.go` (lifecycle, hook install, watchdog), `hook.go`
(`lowLevelKbProc` + `injectOutput`), `features.go` (per-app rules, foreground
app, clipboard injection, startup), `settings.go` / `settings_ui.go` (ini + custom
Win32 settings window), `hud.go`, `tray.go`, `theme.go`, `winapi.go`.

## Build and test

```powershell
go build -ldflags "-H windowsgui -s -w" -o eurokey.exe .
powershell -File scripts/check-eurokey.ps1          # vet + tests + build
go test ./engine/ -bench . -benchtime=200ms -run "^$"   # perf
```

End-to-end typing (needs the app closed + a desktop):

```powershell
powershell -File scripts/e2e-eurokey.ps1 -Lang de
```

`checks.md` records pass/fail and evidence. Keep it updated.

## Engine contract

`engine.Engine` is the only thing the shell knows about:

```go
Filter(ch uint32) Result   // one ASCII char -> what to inject
Backspace() Result
Reset()
SetEnabled(on bool); SetLanguage(lang int)
GetOptions()/SetOptions(Options)
LoadMacro(path string) bool
```

`Result{Out []uint16; Backs int; Consumed bool}`: `Backs` characters are deleted
before `Out` is injected; when `Consumed` is false the OS types the key itself.

## Invariants

- **No diagnostics.** No app-owned logging, crash writers, heartbeat, stderr
  redirection or telemetry. Test files (`*_test.go`) may log.
- **No network in the typing app.** The only optional network user is a
  companion updater, which is not built or shipped here.
- **On-device only.** The clipboard mode reads/replaces the clipboard
  transiently and restores it; nothing is uploaded.
- Keep the engine Windows-free so it stays unit-testable and portable.

## Language tables

Dead keys are per language (`engine/langs.go`); `"` only works in Deutsch, `'`/
`,`/`^` in Français, `~`/`?`/`!` in Español. This is deliberate. If a shared
"all languages" mode is added, keep the per-language tables and union them at
runtime rather than duplicating combinations.

`applyCase` must special-case `ß -> ẞ` (U+1E9E): Go's `unicode.ToUpper` leaves
`ß` unchanged.

## Testing gotchas (learned the hard way)

1. **`SetEnabled` is called on every keydown** by the shell. It must be a no-op
   when the value is unchanged, otherwise it wipes the pending dead key before
   the next character can combine (`"a` never became `ä`). `TestSetEnabledPerKeyKeepsDeadKey`
   guards this.
2. **`settingsProc` recovers panics internally.** A nil `gEngine` makes every UI
   click silently do nothing; `uiOpenSettings` initialises the engine so the UI
   tests cannot pass vacuously.
3. **`rod`/CDP cannot drive the hook.** Browser key events bypass the OS
   keyboard hook, so the e2e harness types with `SendInput` tagged
   `dwExtraInfo = EUROKEY_TEST_MAGIC` (see `features.go`) and only uses rod to
   open the page and read the result.
4. **The app is single-instance.** The e2e harness must be able to set
   `EUROKEY_TEST_MAGIC` at launch, so any running EuroKey must be closed first
   (the test skips with a clear message otherwise).

## Style

- Match the surrounding Win32 code: lazy `NewProc`, `unsafe.Pointer` structs with
  a size assertion in `init`, panics contained at callbacks.
- Keep changes consistent with the existing settings UI (owner-drawn controls,
  no Apply button — every click writes config and saves).
