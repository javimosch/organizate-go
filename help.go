package main

import "fmt"

// printHelp is the human-facing help. It goes to STDOUT when you ask for it
// (so `organizate-go help | less` works) and exits 0 — the machine paths are
// `help-json` and `guide`.
func printHelp() {
	w := func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	w("organizate-go — the agent-first tidy (dry-run by default)")
	w("")
	w("usage: organizate-go <command> [options]")
	w("       organizate-go             (no command = plan, still a dry run)")
	w("")
	w("commands:")
	w("  plan [--all]       what would move (JSON). Default view, no side effects")
	w("  scan [--measure]   every top-level entry + classification (JSON)")
	w("  apply [--apply]    DRY RUN unless --apply is passed — this is the point")
	w("  undo [n] [--all]   DRY RUN unless --apply is passed; restores newest-first")
	w("  keep add|rm|ls     edit the keep list (tool config, never your data)")
	w("  guide [--human]    the embedded operator guide")
	w("  help-json          machine-readable command catalog")
	w("  version [--json]   print the version")
	w("  help               this help (also: -h, --help)")
	w("")
	w("options:")
	w("  --root DIR    what to tidy          (default: $ORGANIZATE_HOME or $HOME)")
	w("  --dest DIR    where things get filed (default: <root>/Organized)")
	w("  --state DIR   journal + keep list    (default: <root>/.organizate)")
	w("  --only CAT    narrow plan/apply to one category")
	w("  --human       render for a person instead of JSON")
	w("  --measure     size hidden directories too (otherwise size: -1)")
	w("  --apply       actually perform the moves (or the undo)")
	w("")
	w("dry-run examples (what an agent does first):")
	w("  organizate-go plan")
	w("  organizate-go plan --all --only debris")
	w("  organizate-go apply                  # prints the plan, changes nothing")
	w("  organizate-go undo                   # prints what would be restored")
	w("")
	w("when you mean it:")
	w("  organizate-go apply --apply --only debris")
	w("  organizate-go undo 5 --apply")
	w("  organizate-go undo --all --apply")
	w("")
	w("safety: moves are rename(2) into <root>/Organized/ or into the trash under")
	w("the state dir — nothing is ever deleted. Every move appends {ts,op,src,dst}")
	w("to <state>/journal.jsonl, which is what undo walks backwards.")
	w("Exit codes: 0 ok, 80-89 input, 90-99 state, 100-109 external, 110-119 internal")
}
