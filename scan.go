// The scan: one pass over the target root, one classification per entry.
//
// Nothing here moves anything. A scan produces ITEMS with a PROPOSED action;
// only actions.go touches the filesystem. The rules and even the wording are
// ported from the machin build so both tools classify a machine identically.

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Item is one top-level entry and everything we know about it.
type Item struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Kind     string `json:"kind"` // file | dir | link
	Size     int64  `json:"size"` // -1 = not measured yet (hidden dirs)
	MTime    int64  `json:"mtime"`
	Cat      string `json:"cat"`
	Action   string `json:"action"` // file | trash | none
	Dest     string `json:"dest"`
	Selected bool   `json:"selected"` // pre-ticked: low-risk, one-off output
	Status   string `json:"status"`   // "" | filed | trashed | kept (session)
	Reason   string `json:"reason"`   // why this entry is on the list
	Advice   string `json:"advice"`   // what to do about it (big dirs)
}

// Target is where we look and where state lives. Flags win over env, env wins
// over the built-in defaults.
type Target struct {
	Root  string
	Dest  string
	State string
}

func resolveTarget(root, dest, state string) Target {
	if root == "" {
		root = envOr("ORGANIZATE_HOME", envOr("HOME", "."))
	}
	if dest == "" {
		dest = filepath.Join(root, "Organized")
	}
	if state == "" {
		state = filepath.Join(root, ".organizate")
	}
	return Target{Root: root, Dest: dest, State: state}
}

func (t Target) journalPath() string { return filepath.Join(t.State, "journal.jsonl") }
func (t Target) keepPath() string    { return filepath.Join(t.State, "keep.txt") }

func hogMin() int64 { return 300 * 1024 * 1024 } // 300 MB

// --- classification ------------------------------------------------------

var (
	reDebrisLog      = regexp.MustCompile(`^(apply|check)-.*\.log$`)
	reRunPart        = regexp.MustCompile(`^run-part.*\.sh$`)
	reSearchTerms    = regexp.MustCompile(`^search-terms-part[0-9].*`)
	reParallelResult = regexp.MustCompile(`^parallel-results-part[0-9].*`)
	reFixMedia       = regexp.MustCompile(`^fix-media.*`)
	rePveLog         = regexp.MustCompile(`^pve1\.log$`)
	reStashSuffix    = regexp.MustCompile(`-(rescue|trial|backup|sweep)$`)
)

func isDebris(name string) bool {
	if reDebrisLog.MatchString(name) || reRunPart.MatchString(name) ||
		reSearchTerms.MatchString(name) || reParallelResult.MatchString(name) ||
		reFixMedia.MatchString(name) || rePveLog.MatchString(name) {
		return true
	}
	return name == "skip-processed.js" || strings.HasSuffix(name, ".bak")
}

var extRules = []struct {
	exts   []string
	cat    string
	folder string
	why    string
}{
	{[]string{".log"}, "logs", "logs", "log output — filing it keeps it, deleting nothing"},
	{[]string{".sh", ".py", ".js", ".ts", ".rb", ".pl", ".php"}, "scripts", "scripts", "a script sitting loose in home — filed, not deleted"},
	{[]string{".deb", ".exe", ".msi", ".zip", ".tar.xz", ".tar.gz", ".tgz", ".gz", ".xz", ".iso", ".7z", ".AppImage", ".whl", ".run"}, "installers", "installers", "installer or archive — likely already installed or extracted"},
	{[]string{".pdf", ".md", ".txt", ".doc", ".docx", ".odt", ".rtf", ".tex"}, "docs", "docs", "document — belongs with the other documents"},
	{[]string{".json", ".csv", ".tsv", ".xlsx", ".sqlite", ".db"}, "data", "data", "data export or scratch dataset"},
	{[]string{".mp3", ".mp4", ".wav", ".flac", ".jpg", ".jpeg", ".png", ".gif", ".webp", ".mkv", ".mov", ".avi"}, "media", "media", "media file living in home root"},
}

// classifyFile mirrors the machin build: debris first, then extension rules,
// then "loose file in the home root".
func classifyFile(name string) (cat, folder, why string) {
	if isDebris(name) {
		return "debris", "job-debris", "one-off job artifact (run log or part output) — nothing references it"
	}
	for _, r := range extRules {
		for _, e := range r.exts {
			if strings.HasSuffix(name, e) {
				return r.cat, r.folder, r.why
			}
		}
	}
	return "misc", "misc", "loose file sitting in the home root"
}

func stashNames() []string {
	return []string{"to-backup", "tmp", "installer-stash", "ext1", "Downloads",
		"db-backups", "backups", "__pycache__", "data-test"}
}

func isStash(name string) bool {
	if reStashSuffix.MatchString(name) {
		return true
	}
	for _, n := range stashNames() {
		if name == n {
			return true
		}
	}
	return false
}

// hogAdvice: what a human can actually DO about a big directory. Shown, never
// executed — organizate-go does not run cleanup commands on your machine.
func hogAdvice(name string) string {
	switch name {
	case ".cache":
		return "rebuildable: rm -rf ~/.cache/* (apps refill it)"
	case ".npm":
		return "npm cache clean --force"
	case ".nvm":
		return "nvm uninstall <version> for each stale Node"
	case ".bun":
		return "bun pm cache rm"
	case ".docker":
		return "docker builder prune --force"
	case ".pyenv":
		return "pyenv uninstall <version> for unused Pythons"
	case ".ollama":
		return "ollama rm <model> for unused models"
	case ".vscode-server", ".windsurf-server":
		return "remote-server install — deletes safely, it re-downloads on next connect"
	case ".minikube":
		return "minikube delete, then rm -rf ~/.minikube"
	case ".local":
		return "look inside ~/.local/share before touching it"
	case "docker":
		return "docker system df, then docker system prune -a"
	case "osm":
		return "OSM dataset — archive to another disk if you are done with it"
	case "media-rescue":
		return "rescued media — verify a copy exists before filing it away"
	case "installer-stash":
		return "installer drop-zone — deletable once installs are verified"
	case "to-backup":
		return "NOT yet backed up — do not delete, run a backup first"
	case "db-backups", "backups":
		return "live backups — rotate old ones, keep the newest"
	}
	return "big — review it by hand before touching it"
}

func classifyDir(name, path string) (cat, why, advice string) {
	switch {
	case strings.HasPrefix(name, "."):
		return "dotdir",
			"hidden tool or config directory — hands off unless you know what owns it",
			hogAdvice(name)
	case isDir(filepath.Join(path, ".git")):
		return "project",
			"git repository — a project, not clutter (filing it is opt-in)",
			"if you file it, fix any script that hard-codes its old path"
	case isEmptyDir(path):
		return "empty",
			"empty directory — there is nothing inside to lose",
			"trash it; undo puts it back exactly"
	case isStash(name):
		return "stash",
			"rescue/trial/backup scratch area — worth reviewing, then filing",
			"filing moves it under Organized/stash, out of sight but intact"
	}
	return "dir", "directory in home root — organizate never auto-moves it", hogAdvice(name)
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func isEmptyDir(p string) bool {
	entries, err := os.ReadDir(p)
	return err == nil && len(entries) == 0
}

func isSymlink(p string) bool {
	fi, err := os.Lstat(p)
	return err == nil && fi.Mode()&os.ModeSymlink != 0
}

// --- scan ----------------------------------------------------------------

// skipEntry hides organizate's own directories and, when the destination or
// state dir lives inside the root, those too — scanning your own output is
// how a tidy tool talks itself into a loop.
func skipEntry(name string, t Target) bool {
	if name == ".organizate" {
		return true
	}
	if filepath.Base(t.Dest) == name && filepath.Dir(t.Dest) == t.Root {
		return true
	}
	if filepath.Base(t.State) == name && filepath.Dir(t.State) == t.Root {
		return true
	}
	return false
}

// scan walks the root once. Directories are sized by a worker pool; hidden
// directories are left at size -1 ("not measured") unless measureHidden, which
// is what keeps a cold start at seconds instead of half a minute.
func scan(t Target, measureHidden bool, progress func(done, total int)) ([]Item, error) {
	entries, err := os.ReadDir(t.Root)
	if err != nil {
		return nil, err
	}
	keep, err := readKeep(t)
	if err != nil {
		return nil, err
	}

	items := []Item{}
	var pending []int
	for _, e := range entries {
		name := e.Name()
		if skipEntry(name, t) {
			continue
		}
		p := filepath.Join(t.Root, name)
		it, isDir, err := classifyEntry(p, name, t, keep)
		if err != nil {
			eprintf("%s: %v\n", p, err)
			continue
		}
		items = append(items, it)
		if isDir && (measureHidden || !strings.HasPrefix(name, ".")) {
			pending = append(pending, len(items)-1)
		}
	}
	measure(items, pending, progress)
	return items, nil
}

// classifyEntry follows the machin build's entry rules: stat first (links are
// followed, so a symlink to a file files like a file), a symlink to a
// directory is left alone, and a broken link is simply not a candidate.
func classifyEntry(p, name string, t Target, keep map[string]bool) (Item, bool, error) {
	fi, err := os.Stat(p) // follows symlinks, like the machin build's stat()
	if err != nil {
		return Item{}, false, err
	}
	mt := fi.ModTime().Unix()

	if !fi.IsDir() {
		it := classifyFileItem(p, name, fi.Size(), mt, t, keep)
		return it, false, nil
	}
	if isSymlink(p) {
		return Item{
			Name: name, Path: p, Kind: "link", Size: fi.Size(), MTime: mt,
			Cat: "link", Reason: "symlink — organizate leaves links where they are",
			Action: "none",
		}, false, nil
	}
	return classifyDirItem(p, name, mt, t), true, nil
}

func classifyFileItem(p, name string, size, mt int64, t Target, keep map[string]bool) Item {
	cat, folder, why := classifyFile(name)
	it := Item{
		Name: name, Path: p, Kind: "file", Size: size, MTime: mt,
		Cat: cat, Action: "file",
		Dest:   filepath.Join(t.Dest, folder, name),
		Reason: why,
	}
	switch {
	case strings.HasPrefix(name, "."):
		// A hidden file is configuration, not clutter (.bashrc, .vimrc, …).
		// It never appears in a proposal — not even an opt-in one.
		it.Action, it.Dest = "none", ""
		it.Reason = "hidden file — configuration, organizate never moves it"
	case keep[name]:
		it.Action, it.Dest = "none", ""
		it.Reason = "you kept this one — organizate will not propose it again"
		it.Status = "kept"
	case cat == "debris" || cat == "logs":
		it.Selected = true
	}
	return it
}

func classifyDirItem(p, name string, mt int64, t Target) Item {
	cat, why, advice := classifyDir(name, p)
	it := Item{
		Name: name, Path: p, Kind: "dir", Size: -1, MTime: mt,
		Cat: cat, Action: "none", Reason: why, Advice: advice,
	}
	switch cat {
	case "project":
		it.Action = "file"
		it.Dest = filepath.Join(t.Root, "Projects", name)
	case "stash":
		it.Action = "file"
		it.Dest = filepath.Join(t.Dest, "stash", name)
	case "empty":
		it.Action = "trash"
	}
	return it
}

// --- sizing --------------------------------------------------------------

const sizeBudget = 1_000_000 // entries per directory: a hang guard, not a policy

// dirSize totals a tree with ONE stat per entry. Symlinked directories are
// skipped (a link back up the tree would loop forever) and the per-entry
// budget stops a runaway tree from hanging the CLI.
func dirSize(path string, budget int) (total int64, used int) {
	if budget <= 0 {
		return 0, 0
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, 0
	}
	for _, e := range entries {
		if used >= budget {
			break
		}
		fp := filepath.Join(path, e.Name())
		fi, err := os.Stat(fp) // follows, so a symlinked file counts its target
		used++
		if err != nil {
			continue
		}
		if fi.IsDir() {
			if isSymlink(fp) {
				continue
			}
			t2, u2 := dirSize(fp, budget-used)
			total += t2
			used += u2
			continue
		}
		if fi.Mode().IsRegular() {
			total += fi.Size()
		}
	}
	return total, used
}

// measure sizes the given indices on a worker pool. Workers only see paths
// handed to them through the channel; the item slice is written by the
// collector alone, so there is nothing to race on.
func measure(items []Item, idxs []int, progress func(done, total int)) {
	if len(idxs) == 0 {
		return
	}
	type job struct {
		i    int
		path string
	}
	type result struct {
		i    int
		size int64
	}

	jobs := make(chan job, len(idxs)) // fully buffered: no feeder goroutine
	for _, i := range idxs {
		jobs <- job{i, items[i].Path}
	}
	close(jobs)

	workers := runtime.NumCPU()
	if workers > 8 {
		workers = 8 // I/O bound: more than this buys nothing on a warm cache
	}
	res := make(chan result, workers)
	for w := 0; w < workers; w++ {
		go func() {
			for j := range jobs {
				sz, _ := dirSize(j.path, sizeBudget)
				res <- result{j.i, sz}
			}
		}()
	}
	for done := 0; done < len(idxs); done++ {
		r := <-res
		items[r.i].Size = r.size
		if progress != nil && (done+1)%5 == 0 || (progress != nil && done+1 == len(idxs)) {
			progress(done+1, len(idxs))
		}
	}
}

// pending are the entries still at size -1 (hidden directories).
func pending(items []Item) []int {
	var out []int
	for i, it := range items {
		if it.Size < 0 {
			out = append(out, i)
		}
	}
	return out
}
