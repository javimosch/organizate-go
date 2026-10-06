package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// --- helpers -------------------------------------------------------------

func writeFixture(t *testing.T, rel, content string) {
	t.Helper()
	p := filepath.Join(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newFixture builds the shape the machin tests use: run debris, a project,
// a stash dir, an empty dir, a hidden file and a plain script.
func newFixture(t *testing.T) Target {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "apply-b12.log"), "log\n")
	writeFixture(t, filepath.Join(root, "pve1.log"), strings.Repeat("x", 4096))
	writeFixture(t, filepath.Join(root, "fix-media1.py.bak"), "bak\n")
	writeFixture(t, filepath.Join(root, "parallel-results-part1.json"), "{}\n")
	writeFixture(t, filepath.Join(root, "standalone-scraper.js"), "js\n")
	writeFixture(t, filepath.Join(root, "free-space-report.md"), "md\n")
	writeFixture(t, filepath.Join(root, "node-v22.14.0-linux-x64.tar.xz"), "inst\n")
	writeFixture(t, filepath.Join(root, ".bashrc"), "export X=1\n")
	writeFixture(t, filepath.Join(root, "to-backup/keep.txt"), "backup\n")
	writeFixture(t, filepath.Join(root, "socials-sweep/data/f.txt"), "a\n")
	writeFixture(t, filepath.Join(root, "someproj/.git/HEAD"), "ref\n")
	writeFixture(t, filepath.Join(root, "someproj/a.txt"), "code\n")
	if err := os.MkdirAll(filepath.Join(root, "empty-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	return resolveTarget(root, "", "")
}

// snapshot lists every path under root except organizate's own output trees,
// so before/after comparisons ignore what the tool itself creates.
func snapshot(t *testing.T, root string, skip ...string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		for _, s := range skip {
			if p == s || strings.HasPrefix(p, s+string(filepath.Separator)) {
				if fi.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func find(items []Item, name string) (Item, bool) {
	for _, it := range items {
		if it.Name == name {
			return it, true
		}
	}
	return Item{}, false
}

// --- classification ------------------------------------------------------

func TestClassifyFile(t *testing.T) {
	cases := []struct {
		name, cat, folder string
		selected          bool
	}{
		{"apply-b12.log", "debris", "job-debris", true},
		{"check-b1920.log", "debris", "job-debris", true},
		{"run-part1-v3.sh", "debris", "job-debris", true},
		{"search-terms-part2.txt", "debris", "job-debris", true},
		{"parallel-results-part3.json", "debris", "job-debris", true},
		{"skip-processed.js", "debris", "job-debris", true},
		{"media-add.py.bak", "debris", "job-debris", true},
		{"pve1.log", "debris", "job-debris", true},
		{"apply-mp3.log", "debris", "job-debris", true},
		{"other.log", "logs", "logs", true},
		{"standalone-scraper.js", "scripts", "scripts", false},
		{"node-v22.14.0-linux-x64.tar.xz", "installers", "installers", false},
		{"wpasupplicant.deb", "installers", "installers", false},
		{"free-space-report.md", "docs", "docs", false},
		{"apis.txt", "docs", "docs", false},
		{"parallel-raw.json", "data", "data", false},
		{"song.mp3", "media", "media", false},
		{"mystery.xyz", "misc", "misc", false},
	}
	for _, c := range cases {
		cat, folder, why := classifyFile(c.name)
		if cat != c.cat || folder != c.folder {
			t.Errorf("classifyFile(%q) = (%q,%q), want (%q,%q)", c.name, cat, folder, c.cat, c.folder)
		}
		if why == "" {
			t.Errorf("classifyFile(%q) has no reason — every proposal must explain itself", c.name)
		}
	}
}

func TestHiddenFileIsNeverProposed(t *testing.T) {
	target := newFixture(t)
	items, err := scan(target, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	it, ok := find(items, ".bashrc")
	if !ok {
		t.Fatal(".bashrc missing from scan")
	}
	if it.Action != "none" || it.Dest != "" || it.Selected {
		t.Errorf("hidden file must never be proposed, got action=%q dest=%q selected=%v", it.Action, it.Dest, it.Selected)
	}
	if it.Reason == "" {
		t.Error("hidden file needs a reason too")
	}
}

func TestDirClassification(t *testing.T) {
	target := newFixture(t)
	items, err := scan(target, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"to-backup":     "stash",
		"socials-sweep": "stash",
		"someproj":      "project",
		"empty-dir":     "empty",
	}
	for name, cat := range want {
		it, ok := find(items, name)
		if !ok {
			t.Fatalf("%s missing from scan", name)
		}
		if it.Cat != cat {
			t.Errorf("%s cat = %q, want %q", name, it.Cat, cat)
		}
	}
	proj, _ := find(items, "someproj")
	if proj.Action != "file" || !strings.HasSuffix(proj.Dest, filepath.Join("Projects", "someproj")) {
		t.Errorf("project action/dest wrong: %q %q", proj.Action, proj.Dest)
	}
	empty, _ := find(items, "empty-dir")
	if empty.Action != "trash" {
		t.Errorf("empty dir action = %q, want trash", empty.Action)
	}
	// Hidden directories are deliberately unmeasured at scan time.
	for _, it := range items {
		if strings.HasPrefix(it.Name, ".") && it.Kind == "dir" && it.Size >= 0 {
			t.Errorf("hidden dir %s should be size -1 until measured", it.Name)
		}
	}
}

func TestSymlinkedDirIsNotFollowed(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "real/file.txt"), "data\n")
	// A symlink back up the tree: a walk that follows it never terminates.
	if err := os.Symlink(root, filepath.Join(root, "real", "loop")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	target := resolveTarget(root, "", "")
	items, err := scan(target, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	it, ok := find(items, "real")
	if !ok {
		t.Fatal("real/ missing")
	}
	if it.Size <= 0 {
		t.Errorf("size = %d, want > 0 (and, more importantly, to return at all)", it.Size)
	}
	// Budgeted walk must also terminate on a cyclic tree.
	if _, used := dirSize(root, sizeBudget); used == 0 {
		t.Error("dirSize visited nothing")
	}
}

func TestDirSizeCountsFilesNotDirs(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "a/one.txt"), strings.Repeat("x", 100))
	writeFixture(t, filepath.Join(root, "a/b/two.txt"), strings.Repeat("y", 200))
	got, used := dirSize(root, sizeBudget)
	if got != 300 {
		t.Errorf("dirSize = %d, want 300", got)
	}
	// used counts every entry visited, directories included: a, a/one.txt, a/b, a/b/two.txt
	if used != 4 {
		t.Errorf("used = %d, want 4", used)
	}
}
