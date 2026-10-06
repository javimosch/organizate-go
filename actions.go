// The only code that touches the filesystem.
//
// Contract, and the whole reason this variant exists:
//   - nothing is ever deleted — entries are MOVED (os.Rename) into
//     Organized/ or into a trash folder under the state dir
//   - every move appends {ts,op,src,dst} to a JSONL journal BEFORE you can
//     lose track of it (the journal and the machin build's are interchangeable)
//   - nothing happens at all without --apply: plan/apply/undo report first
//   - undo reverses newest-first and rewrites the journal

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry is one line of journal.jsonl. Field order matches the machin build,
// so `organizate undo` can reverse a `organizate-go apply --apply` and
// vice versa.
type Entry struct {
	TS  int64  `json:"ts"`
	Op  string `json:"op"`
	Src string `json:"src"`
	Dst string `json:"dst"`
}

// Move is one proposed or performed move, as reported to the caller.
type Move struct {
	Op     string `json:"op"`
	Src    string `json:"src"`
	Dst    string `json:"dst"`
	Cat    string `json:"cat"`
	Size   int64  `json:"size"`
	Reason string `json:"reason"`
}

// UndoPlan is what one undo step would do (From = journal dst, To = journal src).
type UndoPlan struct {
	TS   int64  `json:"ts"`
	Op   string `json:"op"`
	From string `json:"from"`
	To   string `json:"to"`
}

// --- keep list -----------------------------------------------------------

// readKeep returns the set of names you told organizate to stop proposing.
// It is tool configuration, never your data.
func readKeep(t Target) (map[string]bool, error) {
	set := map[string]bool{}
	b, err := os.ReadFile(t.keepPath())
	if err != nil {
		if os.IsNotExist(err) {
			return set, nil
		}
		return nil, err
	}
	for _, ln := range strings.Split(string(b), "\n") {
		if s := strings.TrimSpace(ln); s != "" {
			set[s] = true
		}
	}
	return set, nil
}

func keepList(t Target) ([]string, error) {
	set, err := readKeep(t)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	return out, nil
}

func keepAdd(t Target, name string) (added bool, err error) {
	set, err := readKeep(t)
	if err != nil {
		return false, err
	}
	if set[name] {
		return false, nil
	}
	set[name] = true
	return true, writeKeep(t, set)
}

func keepRemove(t Target, name string) (removed bool, err error) {
	set, err := readKeep(t)
	if err != nil {
		return false, err
	}
	if !set[name] {
		return false, nil
	}
	delete(set, name)
	return true, writeKeep(t, set)
}

func writeKeep(t Target, set map[string]bool) error {
	if err := os.MkdirAll(t.State, 0o755); err != nil {
		return err
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	return os.WriteFile(t.keepPath(), []byte(strings.Join(names, "\n")+"\n"), 0o644)
}

// --- journal -------------------------------------------------------------

func journalRead(t Target) ([]Entry, error) {
	b, err := os.ReadFile(t.journalPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			return nil, fmt.Errorf("journal line is not JSON: %w", err)
		}
		out = append(out, e)
	}
	return out, nil
}

func journalWrite(t Target, entries []Entry) error {
	if err := os.MkdirAll(t.State, 0o755); err != nil {
		return err
	}
	var sb strings.Builder
	for _, e := range entries {
		b, _ := json.Marshal(e)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return os.WriteFile(t.journalPath(), []byte(sb.String()), 0o644)
}

func journalAdd(t Target, op, src, dst string) error {
	entries, err := journalRead(t)
	if err != nil {
		return err
	}
	entries = append(entries, Entry{TS: time.Now().Unix(), Op: op, Src: src, Dst: dst})
	return journalWrite(t, entries)
}

// --- moves ---------------------------------------------------------------

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func trashPath(t Target, name string) string {
	return filepath.Join(t.State, "trash", fmt.Sprintf("%d-%s", time.Now().Unix(), name))
}

// uniqueDst never overwrites. If the destination is taken we prefix a counter
// instead — losing an existing file to a tidy would be the worst bug this
// program could have.
func uniqueDst(p string) string {
	if !exists(p) {
		return p
	}
	dir, base := filepath.Split(p)
	for n := 1; n < 999; n++ {
		cand := filepath.Join(dir, fmt.Sprintf("%d-%s", n, base))
		if !exists(cand) {
			return cand
		}
	}
	return p
}

// applyOne moves items[i]. forceOp overrides the proposal ("trash").
// Returns the destination it used, or an error — never a silent failure.
func applyOne(t Target, items []Item, i int, forceOp string) (string, error) {
	it := items[i]
	op := it.Action
	if forceOp != "" {
		op = forceOp
	}
	if op == "" || op == "none" {
		return "", fmt.Errorf("no action proposed for %s", it.Name)
	}
	if !exists(it.Path) {
		return "", fmt.Errorf("%s: source disappeared", it.Name)
	}
	dst := it.Dest
	if op == "trash" {
		dst = trashPath(t, it.Name)
	}
	if dst == "" {
		return "", fmt.Errorf("%s: no destination", it.Name)
	}
	dst = uniqueDst(dst)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("%s: %w", it.Name, err)
	}
	if err := os.Rename(it.Path, dst); err != nil {
		return "", fmt.Errorf("%s: move failed: %v", it.Name, err)
	}
	if err := journalAdd(t, op, it.Path, dst); err != nil {
		// The file is already at dst: say so loudly rather than pretending
		// the move did not happen.
		return dst, fmt.Errorf("%s: MOVED to %s but NOT journaled: %v", it.Name, dst, err)
	}
	items[i].Status = op + "d" // filed / trashed
	return dst, nil
}

// --- plan ----------------------------------------------------------------

// planMoves splits the proposals into what this run would touch (everything
// when everything is true, only the pre-ticked safe set otherwise) and the
// review-by-hand list that stays untouched.
func planMoves(items []Item, only string, everything bool) (moves []Move, bytes int64, review int, reviewBytes int64) {
	moves = []Move{}
	for _, it := range items {
		if it.Status != "" || it.Action == "none" {
			continue
		}
		selected := everything || it.Selected
		if !selected {
			review++
			reviewBytes += it.Size
			continue
		}
		if only != "" && it.Cat != only {
			continue
		}
		dst := it.Dest
		if it.Action == "trash" {
			dst = "trash"
		}
		moves = append(moves, Move{
			Op: it.Action, Src: it.Path, Dst: dst,
			Cat: it.Cat, Size: it.Size, Reason: it.Reason,
		})
		bytes += it.Size
	}
	return moves, bytes, review, reviewBytes
}

// ApplyResult is the single, stable shape `apply` answers with — dry run and
// executed runs differ only in dry_run/applied/failed, so one parser handles
// both.
type ApplyResult struct {
	OK          bool     `json:"ok"`
	DryRun      bool     `json:"dry_run"`
	Applied     int      `json:"applied"`
	WouldApply  int      `json:"would_apply"`
	Failed      int      `json:"failed"`
	Bytes       int64    `json:"bytes"`
	Review      int      `json:"review"`
	ReviewBytes int64    `json:"review_bytes"`
	Journal     string   `json:"journal"`
	Moves       []Move   `json:"moves"`
	Errors      []string `json:"errors"`
	Message     string   `json:"message"`
}

func applyAll(t Target, items []Item, only string, everything, execute bool) ApplyResult {
	moves, by, review, reviewBytes := planMoves(items, only, everything)
	res := ApplyResult{
		OK: true, DryRun: !execute, WouldApply: len(moves),
		Bytes: by, Review: review, ReviewBytes: reviewBytes,
		Journal: t.journalPath(), Moves: moves, Errors: []string{},
	}
	if !execute {
		res.Message = fmt.Sprintf("dry run — %d move(s) would be performed; re-run with --apply", len(moves))
		return res
	}
	// Execute: walk the items again so failures are attributed per item.
	for i, it := range items {
		if it.Status != "" || it.Action == "none" {
			continue
		}
		if !everything && !it.Selected {
			continue
		}
		if only != "" && it.Cat != only {
			continue
		}
		if _, err := applyOne(t, items, i, ""); err != nil {
			res.Failed++
			res.Errors = append(res.Errors, err.Error())
			continue
		}
		res.Applied++
	}
	res.OK = res.Failed == 0
	if res.Applied > 0 {
		res.Message = fmt.Sprintf("applied %d move(s), %d failed", res.Applied, res.Failed)
	} else {
		res.Message = "nothing to apply"
	}
	return res
}

// UndoResult is the stable shape for `undo`.
type UndoResult struct {
	OK        bool       `json:"ok"`
	DryRun    bool       `json:"dry_run"`
	Undone    int        `json:"undone"`
	WouldUndo int        `json:"would_undo"`
	Remaining int        `json:"remaining"`
	Journal   string     `json:"journal"`
	Moves     []UndoPlan `json:"moves"`
	Message   string     `json:"message"`
}

// undoLast reverses the newest n journal entries, newest first. It stops at
// the first entry it cannot restore rather than leaving a half-reversed
// journal behind; a dry run never touches anything.
func undoLast(t Target, n int, execute bool) (UndoResult, error) {
	entries, err := journalRead(t)
	if err != nil {
		return UndoResult{}, err
	}
	res := UndoResult{OK: true, DryRun: !execute, Journal: t.journalPath(), Moves: []UndoPlan{}}
	if len(entries) == 0 {
		res.Message = "nothing to undo — the journal is empty"
		return res, nil
	}
	if n > len(entries) {
		n = len(entries)
	}
	for i := len(entries) - 1; i >= len(entries)-n; i-- {
		e := entries[i]
		res.Moves = append(res.Moves, UndoPlan{TS: e.TS, Op: e.Op, From: e.Dst, To: e.Src})
	}
	res.WouldUndo = len(res.Moves)
	res.Remaining = len(entries) - res.WouldUndo

	if !execute {
		res.Message = fmt.Sprintf("dry run — %d move(s) would be restored; re-run with --apply", res.WouldUndo)
		return res, nil
	}

	// survivors = the entries we will keep; we walk backwards from the end.
	survivors := len(entries) - len(res.Moves)
	stop := -1 // index of the first entry we could not restore, if any
	for i := len(entries) - 1; i >= survivors; i-- {
		if err := restoreOne(entries[i]); err != nil {
			res.Message = err.Error()
			stop = i
			break
		}
		res.Undone++
	}
	if stop >= 0 {
		survivors = stop + 1 // everything up to and including stop stays journaled
		res.OK = false
	}
	res.Remaining = survivors
	if err := journalWrite(t, entries[:survivors]); err != nil {
		return res, fmt.Errorf("restored %d but could not rewrite the journal: %w", res.Undone, err)
	}
	if res.OK {
		res.Message = fmt.Sprintf("restored %d item(s)", res.Undone)
	}
	return res, nil
}

// restoreOne puts a journaled move back exactly where it came from.
func restoreOne(e Entry) error {
	if !exists(e.Dst) {
		return fmt.Errorf("cannot restore %s: it is gone", e.Dst)
	}
	if exists(e.Src) {
		return fmt.Errorf("cannot restore %s: something is already there", e.Src)
	}
	if err := os.MkdirAll(filepath.Dir(e.Src), 0o755); err != nil {
		return fmt.Errorf("cannot restore %s: %v", e.Src, err)
	}
	if err := os.Rename(e.Dst, e.Src); err != nil {
		return fmt.Errorf("cannot restore %s: %v", e.Dst, err)
	}
	return nil
}
