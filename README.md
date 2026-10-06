# organizate-go

The **agent-first variation** of *organizate* (the human-first, machin-based
sibling build): same tidy rules, same journal, no TUI — JSON on stdout and
**dry-run by default**. Every command that could touch the filesystem reports
what it *would* do and exits 0; you must pass `--apply` to make it act. That
single rule is why an agent can be handed this binary and drive it without a
human watching.

Built with Go (stdlib only), one static binary, no runtime.

```
$ organizate-go plan
{"app":"organizate-go","version":"1.0.0","root":"/home/you","dest":"…/Organized",
 "dry_run":true,"count":40,"bytes":5300224,"review":35,"review_bytes":7876458496,
 "moves":[{"op":"file","src":"…/apply-vid.log","dst":"…/job-debris/apply-vid.log",
           "cat":"debris","size":261,"reason":"one-off job artifact …"}]}

$ organizate-go apply                 # still a dry run — that is the point
{"dry_run":true,"applied":0,"would_apply":40,"message":"dry run — 40 move(s) would be performed; re-run with --apply"}

$ organizate-go apply --only debris --apply     # now it acts
{"dry_run":false,"applied":40,"failed":0}
```

## The contract (cli-output-spec family)

- **stdout = data.** JSON by default; `--human` renders the same data as prose.
- **stderr = context.** Sizing progress and warnings only.
- **Nothing mutates without `--apply`.** `plan`, `apply` and `undo` are dry
  runs by default; a dry run does not even create `<state>/`. The one
  exception is `keep` — it edits the tool's own keep list, never your data.
- **Typed errors**: `{"ok":false,"error":{code,type,message,recoverable,suggestions}}`
  on stdout, and the exit status equals `.error.code`.
- **Embedded guide**: `organizate-go guide` (JSON) / `--human` (markdown),
  `help-json` (command catalog). Compiled in — no network needed.

| exit | meaning | examples |
|------|---------|----------|
| 0 | ok (incl. a dry run) | `apply` without `--apply` is a success |
| 80-89 | input | `80` bad flag/argument · `85` unknown command |
| 90-99 | state | `91` nothing to undo · `90` undo stopped mid-way |
| 100-109 | external | `100` a move/journal I/O failure (recoverable) |
| 110-119 | internal | `110` JSON encode failure (a bug) |

## Commands

```bash
organizate-go                       # = plan (the default, still a dry run)
organizate-go plan [--all]          # the safe set; --all adds the review list
organizate-go scan [--measure]      # every entry + classification
organizate-go apply [--all] [--only CAT]         # DRY RUN
organizate-go apply [--all] [--only CAT] --apply # act
organizate-go undo [n|--all]                    # DRY RUN
organizate-go undo [n|--all] --apply            # restore newest-first
organizate-go keep ls|add NAME|rm NAME  # tool config, never your data
organizate-go guide [--human] · help-json · version [--json] · help
```

Options: `--root DIR` (default `$ORGANIZATE_HOME` or `$HOME`), `--dest DIR`
(default `<root>/Organized`), `--state DIR` (default `<root>/.organizate`),
`--only CAT`, `--human`, `--measure`.

## Safety model (identical to the machin build)

1. The only mutating primitive is `os.Rename` — into `<root>/Organized/<cat>/`
   or into `<state>/trash/`. There is no `rm` in this codebase.
2. Every move appends `{"ts","op","src","dst"}` to `<state>/journal.jsonl`
   *before* you can lose track of it.
3. `undo` reverses newest-first and rewrites the journal; it stops at the
   first entry it cannot restore rather than half-reversing.
4. An existing destination is never overwritten — the move gets a `1-` prefix.
5. Hidden files are never proposed (`.bashrc` is configuration, not clutter);
   hidden directories report `size: -1` until `scan --measure`.
6. Symlinks are followed for `stat` but a symlinked directory is never walked
   or moved; walks are capped at 1 000 000 entries so a runaway tree cannot
   hang.

### Journal interop

The journal line format is byte-compatible with the machin build, so the two
tools can undo each other's work: run `organizate-go apply --apply`, then
`organizate undo --all` (or vice versa) and the tree comes back exactly.

## What "tidy" means here

Same five questions as the machin build, same classification rules (debris
regexes, extension table, stash names, hog advice) — learned from one real
machine, with that machine's folder names generalized for publication
(`socials-sweep`, `media-rescue`, `installer-stash`, `data-test`):

| category | example | destination |
|----------|---------|-------------|
| `debris` | `apply-*.log`, `*.bak`, `run-part*.sh` | `Organized/job-debris/` (pre-ticked = safe set) |
| `logs`, `scripts`, `docs`, `data`, `media`, `installers` | by extension | `Organized/<cat>/` (review list) |
| `stash` | `*-rescue`, `to-backup`, `tmp`… | `Organized/stash/` (review list) |
| `project` | a dir with `.git` | `<root>/Projects/` (opt-in, never auto-moved) |
| `empty` | empty dir | trash (review list) |
| `dotdir`, `link` | `.config`, symlinks | never moved; hogs get advice only |

The safe set (`apply` without `--all`) = pre-ticked `debris` + `logs` files.
`--all` adds everything else that has a proposed action.

## Build & test

```bash
go build -o organizate-go .        # stdlib only, no deps
./test/run.sh                      # 49 assertions (GO=/path/to/go if needed)
go test ./...                      # 14 unit tests: classification, dry-run
                                   # guarantees, round-trip, collision safety
```

The round-trip test (`apply --apply → undo --apply → compare`) is the
contract: the tree must come back byte-identical apart from the tool's own
`Organized/` and `.organizate/` scaffolding.

## Driving it from an agent (a worked loop)

```bash
organizate-go plan --json >/dev/null   # 1. look (JSON is the default)
organizate-go plan --human              #    …or read it as prose
organizate-go apply --only debris       # 2. still dry — check would_apply
organizate-go apply --only debris --apply  # 3. act
organizate-go undo --all                # 4. still dry — check would_undo
organizate-go undo --all --apply        #    …or really undo
```

Rules of thumb: never pass `--apply` on a command you have not first run
without it; `review`/`review_bytes` in a plan are the entries that need a
human (`--all` opts them in); `91` from `undo` just means the journal is
empty.

## License

MIT — see [LICENSE](LICENSE).
