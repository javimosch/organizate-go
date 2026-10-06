# AGENTS.md — organizate-go

Guidance for agents extending this project. Go, stdlib only (`go 1.21` in
`go.mod`). The sibling machin project (`../organizate`) is the human-first
build; this one is the agent-first variation. Read its `AGENTS.md` for the
*why* behind the classification rules — they are ported from there verbatim.

## Philosophy (agent-first, dry-run by default)

- **No command mutates anything without `--apply`.** `plan`, `apply` and
  `undo` report and exit 0; passing `--apply` performs the moves. A dry run
  must not even create `<state>/`. The only exception is `keep`, which edits
  the tool's own keep list — configuration, never your data.
- **A move is never a delete.** The only mutating primitive is `os.Rename`.
  If you ever need a delete, the answer is a journaled move to trash instead.
- **stdout is the API.** JSON by default (`--human` renders the same data as
  prose); progress/warnings go to stderr. Never print anything else to stdout.
- **Errors are typed**: `die(code, type, msg, suggestion)` emits
  `{"ok":false,"error":{…}}` on stdout and exits with `.error.code` — the two
  numbers are equal by construction.
- **Explain every proposal in words.** Every `Item` carries `reason` (why it
  is on the list) and, where relevant, `advice` (what a human can do). A row
  without an explanation does not belong in a report.

## Layout & where things go

| file | responsibility |
|------|----------------|
| `main.go` | dispatch, `positionals()`/`flagTokens()`/`takesValue()`, `die`, flag wiring |
| `commands.go` | `cmdScan`/`cmdPlan`/`cmdApply`/`cmdUndo`/`cmdKeep` — one per verb |
| `scan.go` | `Item`/`Target`, classification (port of the machin rules), `scan`, worker-pool sizing |
| `actions.go` | the only code that touches the filesystem: planMoves, apply, journal, undo, keep |
| `report.go` | JSON-by-default emitters + `--human` renderers |
| `help.go` | human help → stdout |
| `guide.go` | embedded `guide`/`help-json` content (never fetched at runtime) |
| `scan_test.go`, `actions_test.go` | unit tests (14) |
| `test/run.sh` | CLI suite: 49 assertions incl. dry-run guarantees and the round-trip |

## The flag-parsing rule (this bit us once)

`cmd*` functions receive the **full** `os.Args[1:]`, whose first token is the
command name. Go's `flag.Parse` halts at the first non-flag token, so passing
raw args would silently parse NOTHING — every `--root` would fall back to
`$HOME`. `parseFlags` therefore filters through `flagTokens(args)`, which
keeps only flag tokens and the values they consume (`takesValue`). Positionals
come from `positionals(args)`. **If you add a flag that takes a value, add it
to `valueFlags` in `main.go`** — both helpers read that one map.

## How to change things

- **Add a classification** — extend `classifyFile` (extension rules, checked
  in order; `isDebris` first) or `classifyDir` (+ `isStash`). Give the new
  category a destination folder and a `why` sentence. Keep the wording in sync
  with `../organizate/src/scan.src` — both tools must classify identically
  (a few machine-specific folder names were generalized in this repo for
  publication: `socials-sweep`, `media-rescue`, `installer-stash`, `data-test`).
- **Add a command** — a `cmdX` in `commands.go`, a branch in `main`, output
  through the `*ReportOut` helpers in `report.go`, `die(...)` on failure, and
  add it to `help.go` + `guide.go`.
- **Add a flag** — register it in the command's FlagSet; if it takes a value,
  also add it to `valueFlags`. Add a suite assertion in `test/run.sh`.
- **Change safety behaviour** — read `actions.go` first and update
  `test/run.sh` in the same change; the round-trip
  (`apply --apply → undo --apply → diff`) is the contract.

## Conventions

- gofmt + `go vet` must stay clean (the suite enforces both).
- ASCII only inside alignment-sensitive strings; em-dashes in prose are fine.
- One declaration per line; comments explain *why*, not what.
- Data on stdout, progress on stderr, exit codes: `0` ok · `80-89` input ·
  `90-99` state · `100-109` external (recoverable) · `110-119` internal.
- Every new action gets: a `reason` sentence, a dry-run path, a `--apply`
  path, a journal entry, and an undo.

## Journal interop

`journal.jsonl` lines are `{"ts","op","src","dst"}` — byte-compatible with the
machin build so either tool can undo the other's work. Field names and order
are part of the contract; do not change them without updating both projects
and both test suites.

## Build & test

```bash
go build -o organizate-go .    # stdlib only
go test ./...                  # unit tests
./test/run.sh                  # full CLI suite → "passed N, failed 0"
```

The suite builds fresh, runs against a temp fixture, and traps its cleanup —
`/tmp` must be clean afterwards.
