// Output: JSON by default (that is what "agent-first" means here), with a
// --human rendering for the same data. Everything printable goes to stdout;
// progress and warnings go to stderr.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func emitJSON(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		die(ExitInternal, "encode_failed", err.Error(), "this is a bug, report it")
	}
	fmt.Println(string(b))
}

// --- shared text helpers -------------------------------------------------

func humanSize(n int64) string {
	if n < 0 {
		return "?"
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%d KB", (n+512)/1024)
	}
	if n < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
}

func padr(s string, n int) string {
	if len(s) > n {
		s = s[:n]
	}
	for len(s) < n {
		s += " "
	}
	return s
}

func padl(s string, n int) string {
	if len(s) > n {
		s = s[len(s)-n:]
	}
	for len(s) < n {
		s = " " + s
	}
	return s
}

// shortPath is honest about prefixes: ~ only when the path really is under
// $HOME, <root> when we are scanning a target that is not your home.
func shortPath(p, root string) string {
	if p == "" {
		return "-"
	}
	if home := os.Getenv("HOME"); home != "" && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	if strings.HasPrefix(p, root+"/") {
		return "<root>" + p[len(root):]
	}
	return p
}

// --- scan ----------------------------------------------------------------

type scanReport struct {
	App           string `json:"app"`
	Version       string `json:"version"`
	Root          string `json:"root"`
	Dest          string `json:"dest"`
	Count         int    `json:"count"`
	Proposed      int    `json:"proposed"`
	ProposedBytes int64  `json:"proposed_bytes"`
	Items         []Item `json:"items"`
}

func scanReportOut(items []Item, t Target, human bool) {
	r := scanReport{
		App: App, Version: Version, Root: t.Root, Dest: t.Dest,
		Count: len(items), Items: items,
	}
	if r.Items == nil {
		r.Items = []Item{}
	}
	for _, it := range items {
		if it.Status == "" && it.Action != "none" {
			r.Proposed++
			r.ProposedBytes += it.Size
		}
	}
	if human {
		var sb strings.Builder
		fmt.Fprintf(&sb, "root   %s\n", t.Root)
		fmt.Fprintf(&sb, "dest   %s\n\n", t.Dest)
		sb.WriteString("  " + padr("cat", 12) + padr("entry", 36) + padl("size", 9) + "  action\n")
		sb.WriteString("  " + strings.Repeat("-", 70) + "\n")
		for _, it := range items {
			mark := " "
			if it.Selected {
				mark = "*"
			}
			act := it.Action
			if act == "file" {
				act = "-> " + shortPath(it.Dest, t.Root)
			}
			sb.WriteString(fmt.Sprintf("  %s %-11s%-36s%9s  %s\n",
				mark, it.Cat, padr(it.Name, 35), humanSize(it.Size), act))
		}
		fmt.Fprintf(&sb, "\n%d proposed move(s), %s involved\n", r.Proposed, humanSize(r.ProposedBytes))
		fmt.Print(sb.String())
		return
	}
	emitJSON(r)
}

// --- plan ----------------------------------------------------------------

type planReport struct {
	App         string `json:"app"`
	Version     string `json:"version"`
	Root        string `json:"root"`
	Dest        string `json:"dest"`
	DryRun      bool   `json:"dry_run"` // always true: plan never executes
	Count       int    `json:"count"`
	Bytes       int64  `json:"bytes"`
	Review      int    `json:"review"`
	ReviewBytes int64  `json:"review_bytes"`
	Moves       []Move `json:"moves"`
}

func planReportOut(moves []Move, by int64, review int, reviewBytes int64, t Target, human, everything bool) {
	r := planReport{
		App: App, Version: Version, Root: t.Root, Dest: t.Dest, DryRun: true,
		Count: len(moves), Bytes: by, Review: review, ReviewBytes: reviewBytes,
		Moves: moves,
	}
	if human {
		fmt.Println(planHuman(moves, by, review, reviewBytes, everything))
		return
	}
	emitJSON(r)
}

// planHuman is the shared prose for a dry-run plan and a dry-run apply.
func planHuman(moves []Move, by int64, review int, reviewBytes int64, everything bool) string {
	var sb strings.Builder
	if len(moves) == 0 {
		sb.WriteString("  nothing auto-proposed — see plan --all for the review list\n")
	}
	for _, m := range moves {
		sb.WriteString(fmt.Sprintf("  %-7s%-12s%s  ->  %s\n",
			m.Op, m.Cat, m.Src, m.Dst))
	}
	sb.WriteString("\n")
	verb := "performed by `apply --apply`"
	if everything {
		verb = "performed by `apply --apply --all`"
	}
	sb.WriteString(fmt.Sprintf("%d move(s) %s, %s involved\n", len(moves), verb, humanSize(by)))
	if review > 0 && !everything {
		sb.WriteString(fmt.Sprintf("%d more (%s) proposed but NOT auto-applied — pass --all or tick them in the TUI\n",
			review, humanSize(reviewBytes)))
	}
	return sb.String()
}

// --- apply ---------------------------------------------------------------

type applyEnvelope struct {
	App     string `json:"app"`
	Version string `json:"version"`
	Root    string `json:"root"`
	ApplyResult
}

func applyReportOut(res ApplyResult, t Target, human bool) {
	if human {
		if res.DryRun {
			fmt.Println(planHuman(res.Moves, res.Bytes, res.Review, res.ReviewBytes, len(res.Moves) > 0 && res.Review == 0))
			fmt.Println(res.Message)
			return
		}
		fmt.Printf("applied %d move(s), %d failed — %s\n", res.Applied, res.Failed, humanSize(res.Bytes))
		for _, e := range res.Errors {
			fmt.Printf("  ! %s\n", e)
		}
		fmt.Printf("journal: %s\n", res.Journal)
		return
	}
	emitJSON(applyEnvelope{App: App, Version: Version, Root: t.Root, ApplyResult: res})
}

// --- undo ----------------------------------------------------------------

type undoEnvelope struct {
	App     string `json:"app"`
	Version string `json:"version"`
	Root    string `json:"root"`
	UndoResult
}

func undoReportOut(res UndoResult, t Target, human bool) {
	if human {
		if res.DryRun {
			for _, m := range res.Moves {
				fmt.Printf("  restore  %s  ->  %s\n", m.From, m.To)
			}
			fmt.Printf("\n%s\n", res.Message)
			return
		}
		fmt.Printf("%s — %d journal entry(ies) remain\n", res.Message, res.Remaining)
		return
	}
	emitJSON(undoEnvelope{App: App, Version: Version, Root: t.Root, UndoResult: res})
}

// --- keep ----------------------------------------------------------------

type keepReport struct {
	App     string   `json:"app"`
	Version string   `json:"version"`
	Action  string   `json:"action"` // list | add | rm
	Name    string   `json:"name"`
	Path    string   `json:"path"`
	Kept    []string `json:"kept"`
}

func keepReportOut(names []string, t Target, action, name string, human bool) {
	if names == nil {
		names = []string{}
	}
	if human {
		switch action {
		case "list":
			for _, n := range names {
				fmt.Println(n)
			}
		case "add":
			fmt.Printf("kept %s\n", name)
		case "rm":
			fmt.Printf("unkept %s\n", name)
		}
		return
	}
	emitJSON(keepReport{
		App: App, Version: Version, Action: action, Name: name,
		Path: t.keepPath(), Kept: names,
	})
}
