package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanSafeSetVersusAll(t *testing.T) {
	target := newFixture(t)
	items, err := scan(target, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	safe, _, _, _ := planMoves(items, "", false)
	all, _, review, reviewBytes := planMoves(items, "", true)
	_ = reviewBytes

	if len(safe) != 4 {
		t.Errorf("safe set = %d moves, want 4 (the four debris files)", len(safe))
	}
	safeHasDir := false
	for _, m := range safe {
		if m.Op == "file" && strings.HasSuffix(m.Src, "to-backup") {
			safeHasDir = true
		}
		if m.Reason == "" {
			t.Error("every move must carry a human-readable reason")
		}
	}
	if safeHasDir {
		t.Error("the stash dir must NOT be in the safe set — it is a review-by-hand item")
	}
	if len(all) <= len(safe) {
		t.Errorf("plan --all = %d moves, safe = %d; --all must include more", len(all), len(safe))
	}
	// --all must contain the review-by-hand items that the safe set refuses.
	for _, name := range []string{"to-backup", "socials-sweep", "someproj", "empty-dir"} {
		foundAll := false
		for _, m := range all {
			if strings.HasSuffix(m.Src, name) {
				foundAll = true
			}
		}
		if !foundAll {
			t.Errorf("plan --all is missing %s", name)
		}
	}
	// The review counter is what a dry run reports as "not auto-applied".
	_, _, safeReview, _ := planMoves(items, "", false)
	if safeReview != len(all)-len(safe) {
		t.Errorf("review = %d, want %d", safeReview, len(all)-len(safe))
	}
	_ = review
	_ = all
	// A keep-listed file drops out of every plan.
	if _, err := keepAdd(target, "free-space-report.md"); err != nil {
		t.Fatal(err)
	}
	items2, err := scan(target, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	all2, _, _, _ := planMoves(items2, "", true)
	for _, m := range all2 {
		if strings.HasSuffix(m.Src, "free-space-report.md") {
			t.Error("keep list must remove an entry from the plan")
		}
	}
}

// TestApplyDryRunTouchesNothing is the contract this variant exists for.
func TestApplyDryRunTouchesNothing(t *testing.T) {
	target := newFixture(t)
	before := snapshot(t, target.Root, target.Dest, target.State, filepath.Join(target.Root, "Projects"))

	items, err := scan(target, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := applyAll(target, items, "", true, false)

	if !res.DryRun || res.Applied != 0 || res.WouldApply == 0 {
		t.Fatalf("dry run result wrong: %+v", res)
	}
	if exists(target.State) {
		t.Error("a dry run must not even create the state directory")
	}
	after := snapshot(t, target.Root, target.Dest, target.State, filepath.Join(target.Root, "Projects"))
	if len(before) != len(after) {
		t.Errorf("dry run changed the tree: %d -> %d paths", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Errorf("dry run moved something: %s -> %s", before[i], after[i])
		}
	}
}

func TestApplyThenUndoRoundTrip(t *testing.T) {
	target := newFixture(t)
	before := snapshot(t, target.Root, target.Dest, target.State, filepath.Join(target.Root, "Projects"))

	items, err := scan(target, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := applyAll(target, items, "", true, true)
	if !res.OK || res.Applied == 0 || res.Failed != 0 {
		t.Fatalf("apply --apply failed: %+v", res)
	}
	if res.DryRun {
		t.Fatal("dry_run must be false after an executed apply")
	}
	if exists(filepath.Join(target.Root, "apply-b12.log")) {
		t.Error("apply did not move the debris")
	}
	if !exists(filepath.Join(target.Dest, "job-debris", "apply-b12.log")) {
		t.Error("debris missing at destination")
	}
	if !exists(filepath.Join(target.Root, "Projects", "someproj", "a.txt")) {
		t.Error("project was not filed into Projects/")
	}
	entries, err := journalRead(target)
	if err != nil || len(entries) != res.Applied {
		t.Fatalf("journal = %d entries, applied = %d (err %v)", len(entries), res.Applied, err)
	}

	// Dry-run undo first: reports, changes nothing.
	dry, err := undoLast(target, 1<<30, false)
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun || dry.WouldUndo != res.Applied || dry.Undone != 0 {
		t.Fatalf("undo dry run wrong: %+v", dry)
	}
	if !exists(filepath.Join(target.Dest, "job-debris", "apply-b12.log")) {
		t.Error("dry-run undo moved something")
	}

	// Real undo: everything comes back exactly.
	undo, err := undoLast(target, 1<<30, true)
	if err != nil {
		t.Fatal(err)
	}
	if !undo.OK || undo.Undone != res.Applied || undo.Remaining != 0 {
		t.Fatalf("undo --apply wrong: %+v", undo)
	}
	after := snapshot(t, target.Root, target.Dest, target.State, filepath.Join(target.Root, "Projects"))
	if len(before) != len(after) {
		t.Fatalf("round-trip changed the tree: %d -> %d", len(before), len(after))
	}
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("round-trip mismatch at %d: %s vs %s", i, before[i], after[i])
		}
	}
	rest, err := journalRead(target)
	if err != nil || len(rest) != 0 {
		t.Fatalf("journal not empty after full undo: %d entries (err %v)", len(rest), err)
	}
}

func TestUndoIsDryRunByDefaultToo(t *testing.T) {
	target := newFixture(t)
	items, _ := scan(target, false, nil)
	if res := applyAll(target, items, "", true, true); !res.OK {
		t.Fatalf("setup apply failed: %+v", res)
	}
	res, err := undoLast(target, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || res.Undone != 0 || res.WouldUndo != 1 {
		t.Fatalf("undo without --apply must not restore: %+v", res)
	}
	entries, _ := journalRead(target)
	if len(entries) == 0 {
		t.Error("journal must be untouched by a dry-run undo")
	}
}

func TestUndoStopsRatherThanHalfRevert(t *testing.T) {
	target := newFixture(t)
	items, _ := scan(target, false, nil)
	res := applyAll(target, items, "", true, true)
	if !res.OK {
		t.Fatalf("setup failed: %+v", res)
	}
	entries, _ := journalRead(target)
	if len(entries) < 2 {
		t.Skip("need at least two journal entries")
	}
	// Put a file back at the source of the OLDEST entry: undo restores the
	// newest ones until it hits the blocked one, then stops — refusing to
	// clobber what is there and leaving the journal consistent.
	oldest := entries[0]
	if err := os.MkdirAll(filepath.Dir(oldest.Src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldest.Src, []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	undo, err := undoLast(target, len(entries), true)
	if err != nil {
		t.Fatal(err)
	}
	if undo.OK {
		t.Fatal("undo should have refused the occupied source")
	}
	if undo.Undone != len(entries)-1 {
		t.Errorf("undone = %d, want %d (everything but the blocked one)", undo.Undone, len(entries)-1)
	}
	rest, _ := journalRead(target)
	if len(rest) != 1 {
		t.Errorf("journal should keep exactly the blocked entry, has %d", len(rest))
	}
	if b, _ := os.ReadFile(oldest.Src); string(b) != "occupied" {
		t.Errorf("the occupying file was clobbered: %q", b)
	}
}

func TestDestCollisionIsNeverOverwritten(t *testing.T) {
	target := newFixture(t)
	// Occupy one destination before anything moves.
	if err := os.MkdirAll(filepath.Join(target.Dest, "job-debris"), 0o755); err != nil {
		t.Fatal(err)
	}
	occupied := filepath.Join(target.Dest, "job-debris", "apply-b12.log")
	if err := os.WriteFile(occupied, []byte("DO NOT LOSE ME"), 0o644); err != nil {
		t.Fatal(err)
	}
	items, _ := scan(target, false, nil)
	res := applyAll(target, items, "debris", true, true)
	if !res.OK {
		t.Fatalf("apply failed: %+v", res)
	}
	b, err := os.ReadFile(occupied)
	if err != nil {
		t.Fatal("the occupied destination disappeared")
	}
	if string(b) != "DO NOT LOSE ME" {
		t.Fatalf("occupied destination was overwritten: %q", b)
	}
	clash := filepath.Join(target.Dest, "job-debris", "1-apply-b12.log")
	if !exists(clash) {
		t.Errorf("move should have landed at %s", clash)
	}
}

func TestTrashGoesUnderStateNotIntoTheVoid(t *testing.T) {
	target := newFixture(t)
	items, _ := scan(target, false, nil)
	if res := applyAll(target, items, "empty", true, true); !res.OK {
		t.Fatalf("apply failed: %+v", res)
	}
	if exists(filepath.Join(target.Root, "empty-dir")) {
		t.Error("empty dir still in place")
	}
	entries, _ := journalRead(target)
	if len(entries) != 1 || entries[0].Op != "trash" {
		t.Fatalf("journal entries = %+v, want one trash op", entries)
	}
	if !strings.HasPrefix(entries[0].Dst, target.State) {
		t.Errorf("trash destination %q must live under the state dir", entries[0].Dst)
	}
	if undo, err := undoLast(target, 1, true); err != nil || !undo.OK {
		t.Fatalf("undo of trash failed: %+v %v", undo, err)
	}
	if !exists(filepath.Join(target.Root, "empty-dir")) {
		t.Error("trashed dir not restored")
	}
}

func TestKeepRoundTrip(t *testing.T) {
	target := newFixture(t)
	added, err := keepAdd(target, "pve1.log")
	if err != nil || !added {
		t.Fatalf("keepAdd = %v, %v", added, err)
	}
	added2, _ := keepAdd(target, "pve1.log")
	if added2 {
		t.Error("second keepAdd should report nothing new")
	}
	names, err := keepList(target)
	if err != nil || len(names) != 1 || names[0] != "pve1.log" {
		t.Fatalf("keepList = %v, %v", names, err)
	}
	items, err := scan(target, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	it, ok := find(items, "pve1.log")
	if !ok || it.Action != "none" || it.Status != "kept" {
		t.Fatalf("kept file still proposed: %+v", it)
	}
	removed, err := keepRemove(target, "pve1.log")
	if err != nil || !removed {
		t.Fatalf("keepRemove = %v, %v", removed, err)
	}
}

func TestScanFailsCleanlyOnMissingRoot(t *testing.T) {
	target := resolveTarget(filepath.Join(t.TempDir(), "does-not-exist"), "", "")
	if _, err := scan(target, false, nil); err == nil {
		t.Fatal("scan of a missing root must return an error, not an empty plan")
	}
}
