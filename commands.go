package main

import (
	"fmt"
	"os"
	"regexp"
)

// progressPrinter keeps stdout clean: sizing progress is context, so it goes
// to stderr (cli-output-spec §1).
func progressPrinter(done, total int) {
	eprintf("organizate-go: sizing %d/%d directories…\n", done, total)
}

func cmdScan(args []string) {
	fs := newFlags("scan")
	root, dest, state, only, human := addCommon(fs)
	measure := fs.Bool("measure", false, "size hidden directories too (otherwise they report -1)")
	parseFlags(fs, args)

	t := resolveTarget(*root, *dest, *state)
	items, err := scan(t, *measure, progressPrinter)
	if err != nil {
		die(ExitExternal, "scan_failed", err.Error(), "organizate-go help --root /path/to/dir")
	}
	if *only != "" {
		var out []Item
		for _, it := range items {
			if it.Cat == *only {
				out = append(out, it)
			}
		}
		items = out
	}
	scanReportOut(items, t, *human)
}

func cmdPlan(args []string) {
	fs := newFlags("plan")
	root, dest, state, only, human := addCommon(fs)
	all := fs.Bool("all", false, "every proposal, not just the pre-ticked safe set")
	parseFlags(fs, args)

	t := resolveTarget(*root, *dest, *state)
	items, err := scan(t, false, progressPrinter)
	if err != nil {
		die(ExitExternal, "scan_failed", err.Error(), "organizate-go help")
	}
	moves, by, review, reviewBytes := planMoves(items, *only, *all)
	planReportOut(moves, by, review, reviewBytes, t, *human, *all)
}

func cmdApply(args []string) {
	fs := newFlags("apply")
	root, dest, state, only, human := addCommon(fs)
	all := fs.Bool("all", false, "every proposal, not just the pre-ticked safe set")
	execute := fs.Bool("apply", false, "actually perform the moves (default: dry run)")
	parseFlags(fs, args)

	t := resolveTarget(*root, *dest, *state)
	items, err := scan(t, false, progressPrinter)
	if err != nil {
		die(ExitExternal, "scan_failed", err.Error(), "organizate-go help")
	}
	res := applyAll(t, items, *only, *all, *execute)
	applyReportOut(res, t, *human)
	if res.Failed > 0 {
		exitAfter(ExitExternal)
	}
}

var reDigits = regexp.MustCompile(`^[0-9]+$`)

func cmdUndo(args []string, pos []string) {
	fs := newFlags("undo")
	root, dest, state, _, human := addCommon(fs)
	all := fs.Bool("all", false, "restore every journal entry")
	execute := fs.Bool("apply", false, "actually restore (default: dry run)")
	parseFlags(fs, args)

	n := 1
	for _, p := range pos {
		if !reDigits.MatchString(p) {
			die(ExitMissingArg, "bad_argument",
				fmt.Sprintf("undo takes a number, got %q", p),
				"organizate-go undo 5 --apply")
		}
		n = parseIntOr(p, 1)
	}
	if *all {
		n = 1 << 30
	}

	t := resolveTarget(*root, *dest, *state)
	res, err := undoLast(t, n, *execute)
	if err != nil {
		die(ExitExternal, "journal_unreadable", err.Error(), "inspect "+t.journalPath())
	}
	if res.WouldUndo == 0 {
		die(ExitNothingToUndo, "nothing_to_undo", res.Message, "organizate-go plan")
	}
	undoReportOut(res, t, *human)
	if !res.OK && *execute {
		// Something could not be restored: the result body names it.
		exitAfter(ExitPrecondition)
	}
}

func cmdKeep(args []string, pos []string) {
	root, dest, state := "", "", ""
	fs := newFlags("keep")
	fs.StringVar(&root, "root", "", "what to tidy (default $ORGANIZATE_HOME or $HOME)")
	fs.StringVar(&dest, "dest", "", "where things get filed (default <root>/Organized)")
	fs.StringVar(&state, "state", "", "journal + keep list (default <root>/.organizate)")
	human := fs.Bool("human", false, "render for a person instead of JSON")
	parseFlags(fs, args)
	t := resolveTarget(root, dest, state)

	sub := "ls"
	if len(pos) > 0 {
		sub = pos[0]
	}
	switch sub {
	case "ls":
		names, err := keepList(t)
		if err != nil {
			die(ExitExternal, "keep_unreadable", err.Error(), "check "+t.keepPath())
		}
		keepReportOut(names, t, "list", "", *human)
	case "add":
		if len(pos) < 2 {
			die(ExitMissingArg, "missing_argument", "keep add needs a file name", "organizate-go keep add pve1.log")
		}
		added, err := keepAdd(t, pos[1])
		if err != nil {
			die(ExitExternal, "keep_write_failed", err.Error(), "check "+t.State+" permissions")
		}
		keepReportOut(keepOrEmpty(t, pos[1]), t, "add", pos[1], *human)
		if !added {
			eprintf("already kept: %s\n", pos[1])
		}
	case "rm":
		if len(pos) < 2 {
			die(ExitMissingArg, "missing_argument", "keep rm needs a file name", "organizate-go keep ls")
		}
		removed, err := keepRemove(t, pos[1])
		if err != nil {
			die(ExitExternal, "keep_write_failed", err.Error(), "check "+t.State+" permissions")
		}
		keepReportOut(nil, t, "rm", pos[1], *human)
		if !removed {
			eprintf("was not kept: %s\n", pos[1])
		}
	default:
		die(ExitMissingArg, "bad_argument",
			fmt.Sprintf("keep subcommand must be ls, add or rm — got %q", sub),
			"organizate-go keep ls")
	}
}

// keepOrEmpty reports a single name as the list for `keep add` output.
func keepOrEmpty(t Target, name string) []string {
	return []string{name}
}

// exitAfter prints its JSON body first (via the report helpers) and then
// leaves with a semantic status — the body stays the data, the exit code
// stays the signal a script branches on.
func exitAfter(code int) {
	os.Exit(code)
}
