package main

import "encoding/json"

// The embedded guide and command catalog (cli-guide-spec, cli-output-spec §4).
//
// Compiled into the binary: an agent that lands on a machine with this binary
// and no network can still learn the tool. Never fetch this at runtime.

func helpJSON() string {
	body := map[string]any{
		"app":     App,
		"version": Version,
		"commands": []map[string]string{
			{"name": "plan", "args": "[--all] [--only CAT] [--human]", "summary": "what would move (JSON). Default view, no side effects"},
			{"name": "scan", "args": "[--measure] [--only CAT] [--human]", "summary": "every top-level entry + classification"},
			{"name": "apply", "args": "[--apply] [--all] [--only CAT] [--human]", "summary": "DRY RUN unless --apply is passed"},
			{"name": "undo", "args": "[n|--all] [--apply] [--human]", "summary": "DRY RUN unless --apply is passed; restores newest-first"},
			{"name": "keep", "args": "ls|add <name>|rm <name>", "summary": "edit the keep list (tool config, never your data)"},
			{"name": "guide", "args": "[--human]", "summary": "the embedded operator guide"},
			{"name": "help-json", "args": "", "summary": "machine-readable command catalog"},
			{"name": "version", "args": "[--json]", "summary": "print the version"},
			{"name": "help", "args": "", "summary": "human help (also -h, --help)"},
		},
		"options": []string{"--root", "--dest", "--state", "--only", "--all", "--apply", "--human", "--measure"},
		// Top-level so agents (and cli-spec-conformance) can read it without
		// knowing the contract shape: the code equals .error.code on failure.
		"exit_codes": map[string]string{
			"0": "ok", "80-89": "input", "90-99": "state",
			"91": "nothing to undo", "100-109": "external", "110-119": "internal",
		},
		"contract": map[string]any{
			"json_by_default":    true,
			"dry_run_by_default": true,
			"stdout":             "data (JSON unless --human)",
			"stderr":             "progress and warnings",
		},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

func guideJSON() string {
	body := map[string]any{
		App:       "An agent-first, dry-run-by-default tidy: the JSON sibling of the organizate TUI.",
		"version": Version,
		// Self-describing: an agent must be able to learn which contracts this
		// binary is held to without fetching anything.
		"conforms_to": []string{"cli-output-spec", "cli-guide-spec"},
		"one_liner": "Scans a root directory, classifies every top-level entry, proposes moves, and refuses " +
			"to touch anything unless you pass --apply — so an agent can plan first, show its work, then act.",
		"model": map[string]string{
			"moves":     "os.Rename into <root>/Organized/<category>/ or <state>/trash/ — never a delete.",
			"journal":   "<state>/journal.jsonl, one {ts,op,src,dst} line per move; undo walks it backwards.",
			"dry_run":   "plan/apply/undo report without --apply; only keep writes (tool config).",
			"contract":  "JSON on stdout, progress on stderr, semantic exit codes, typed errors, embedded guide.",
			"interplay": "same journal format as the machin build — either tool can undo the other's moves.",
		},
		"loop": []string{
			"organizate-go plan                 — what would move, change nothing",
			"organizate-go plan --all           — including the review-by-hand list",
			"organizate-go apply --apply --only debris — do the low-risk slice",
			"organizate-go undo 5 --apply       — put the newest five back",
			"organizate-go keep add pve1.log    — never propose it again",
		},
		"concepts": map[string]string{
			"categories":     "debris, logs, scripts, installers, docs, data, media, misc (files); project, stash, empty, dir, dotdir, link (dirs).",
			"pre-ticked set": "debris and logs are Selected=true; `apply` touches only those unless --all.",
			"hidden files":   "never proposed at all (.bashrc is configuration, not clutter).",
			"size -1":        "a hidden directory that has not been measured; scan --measure fills it in.",
			"exit codes":     "0 ok, 80-89 input, 90-99 state (91 nothing to undo), 100-109 external, 110-119 internal.",
		},
		"commands": map[string][]string{
			"plan": {
				"organizate-go plan [--all] [--only CAT] [--human]",
				"organizate-go apply                    (dry run: prints the same plan)",
			},
			"act": {
				"organizate-go apply --apply [--all] [--only CAT]",
				"organizate-go undo [n] --apply | undo --all --apply",
				"organizate-go keep add|rm|ls <name>",
			},
			"introspection": {
				"organizate-go scan [--measure] [--human]",
				"organizate-go guide [--human]",
				"organizate-go help-json",
				"organizate-go version [--json]",
			},
		},
		"examples": []map[string]any{
			{"goal": "see what a tidy would do (no side effects)", "do": []string{"organizate-go plan"}},
			{"goal": "file only the one-off job debris", "do": []string{
				"organizate-go apply --only debris          # dry run first",
				"organizate-go apply --only debris --apply   # then this",
			}},
			{"goal": "back out everything this tool did", "do": []string{
				"organizate-go undo --all           # preview",
				"organizate-go undo --all --apply   # restore",
			}},
		},
		"gotchas": []string{
			"Without --apply, apply and undo only print what they would do. That is the design, not a bug.",
			"Hidden directories report size -1 until you pass --measure — measuring them is the slow part (~25 s on a 29 GB home).",
			"An existing destination is never overwritten: the move gets a 1- prefix instead.",
			"A move that succeeds but fails to journal reports MOVED but NOT journaled — read errors[], do not retry blindly.",
		},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

func guideMarkdown() string {
	lines := []string{
		"# organizate-go",
		"",
		"An agent-first tidy for one machine: JSON by default, **dry-run by default**.",
		"The JSON sibling of the human-first organizate TUI.",
		"",
		"## The rule",
		"",
		"`plan`, `apply` and `undo` never touch the filesystem. Only `--apply` performs a move;",
		"only `keep` writes (and that is tool configuration, not your data).",
		"",
		"## What tidy means here",
		"",
		"1. clutter — nothing loose in the home root that is not a project or tool",
		"2. debris  — one-off job artifacts (run logs, part outputs) filed together",
		"3. stash   — rescue/trial/backup scratch dirs reviewed, then filed",
		"4. projects — git repos are visible but never auto-moved",
		"5. hogs    — anything over 300 MB surfaced with advice, never auto-touched",
		"",
		"## Safety model",
		"",
		"- moves are rename(2) into <root>/Organized/<category>/ — nothing is deleted",
		"- trash entries go to <state>/trash/ instead, still journaled",
		"- every move appends {ts,op,src,dst} to <state>/journal.jsonl",
		"- undo reverses newest-first and rewrites the journal; it stops rather than half-revert",
		"- the journal format is shared with the machin build, so either tool can undo the other",
		"- hidden files are never proposed; existing destinations are never overwritten",
		"",
		"## Commands",
		"",
		"```",
		"organizate-go plan [--all] [--only CAT]    # what would move (JSON, no side effects)",
		"organizate-go scan [--measure]             # every entry + classification",
		"organizate-go apply [--apply] [--all]      # DRY RUN unless --apply",
		"organizate-go undo [n|--all] [--apply]     # DRY RUN unless --apply",
		"organizate-go keep add|rm|ls <name>        # the keep list",
		"organizate-go guide [--human] | help-json | version [--json] | help",
		"```",
		"",
		"## Exit codes",
		"",
		"0 ok, 80-89 input, 90-99 state (91 = nothing to undo), 100-109 external, 110-119 internal.",
		"apply --apply that failed part-way exits 100 with errors[] in the body; undo that could not",
		"restore exits 90 with the blocker in .message.",
		"",
		"## Specs",
		"",
		"Adopts cli-output-spec and cli-guide-spec (https://cli-specs.intrane.fr/); verified with",
		"cli-spec-conformance check <binary> --specs output,guide -> 19/19, exit 0.",
		"No daemon, no telemetry, no network calls at all - offline by design.",
	}
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}
