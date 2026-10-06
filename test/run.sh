#!/usr/bin/env bash
# organizate-go CLI suite: static checks, unit tests, then every contract an
# agent relies on — JSON on stdout, dry-run by default, --apply to act,
# undo round-trip, semantic exit codes, human help.
set -uo pipefail
cd "$(dirname "$0")/.."

GO="${GO:-$(command -v go || true)}"
if [ -z "$GO" ]; then echo "  FAIL: no go toolchain (set GO=/path/to/go)"; exit 1; fi
export PATH="$(dirname "$GO"):$PATH"

WORK=$(mktemp -d /tmp/organizate-go-work.XXXXXX)
FIX=$(mktemp -d /tmp/organizate-go-fixture.XXXXXX)
trap 'rm -rf "$FIX" "$WORK"' EXIT

PASS=0; FAIL=0
ok()   { PASS=$((PASS + 1)); echo "  ok   $1"; }
bad()  { FAIL=$((FAIL + 1)); echo "  FAIL $1"; }
has()  { if grep -q -- "$2" "$3" >/dev/null 2>&1; then ok "$1"; else bad "$1 (missing: $2)"; fi; }
exists(){ if test -e "$2"; then ok "$1"; else bad "$1 (no such path: $2)"; fi; }
missing(){ if test ! -e "$2"; then ok "$1"; else bad "$1 (path still exists: $2)"; fi; }
# exits <label> <want> <cmd...>
exits() {
  local label=$1 want=$2; shift 2
  "$@" >"$WORK/rc.out" 2>"$WORK/rc.err"
  local got=$?
  if [ "$got" = "$want" ]; then ok "$label"; else bad "$label (exit $got, want $want)"; fi
}
# jget <file> <dotted.path> — read one value out of a JSON file
jget() {
  python3 - "$1" "$2" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
for part in sys.argv[2].split("."):
    d = d[int(part)] if part.isdigit() else d[part]
print(d)
PY
}
# jok <label> <file> <path> <want>
jok() {
  local got
  got=$(jget "$2" "$3" 2>/dev/null) || { bad "$1 (not JSON / no $3)"; return; }
  if [ "$got" = "$4" ]; then ok "$1"; else bad "$1 ($3 = $got, want $4)"; fi
}

echo "== static checks"
if go vet ./... >"$WORK/vet.log" 2>&1; then ok "go vet"; else bad "go vet"; cat "$WORK/vet.log"; fi
UNFORMATTED=$(gofmt -l . 2>/dev/null | tr -d '\r' || true)
if [ -z "$UNFORMATTED" ]; then ok "gofmt clean"; else bad "gofmt clean: $UNFORMATTED"; fi
if go test ./... >"$WORK/test.log" 2>&1; then ok "go test (unit)"; else bad "go test (unit)"; tail -20 "$WORK/test.log"; fi

echo "== build"
if go build -o organizate-go . >"$WORK/build.log" 2>&1; then ok "go build"; else bad "go build"; cat "$WORK/build.log"; exit 1; fi
ORG=./organizate-go

echo "== fixture at $FIX"
mkdir -p "$FIX/socials-sweep/data" "$FIX/empty-dir" "$FIX/someproj/.git" "$FIX/to-backup" "$FIX/.config"
echo keepme  > "$FIX/apis.txt"
echo log     > "$FIX/apply-b12.log"
echo bak     > "$FIX/fix-media1.py.bak"
echo doc     > "$FIX/free-space-report.md"
echo inst    > "$FIX/node-v22.14.0-linux-x64.tar.xz"
echo parts   > "$FIX/parallel-results-part1.json"
echo script  > "$FIX/standalone-scraper.js"
echo hidden  > "$FIX/.bashrc"
echo a       > "$FIX/socials-sweep/data/f.txt"
echo x       > "$FIX/someproj/a.txt"
echo y       > "$FIX/to-backup/keep.txt"
echo z       > "$FIX/.config/settings"
find "$FIX" | sort > "$WORK/before.txt"

echo "== plan is the default and it is a dry run"
$ORG plan --root "$FIX" >"$WORK/plan.json" 2>"$WORK/plan.err"
rc=$?
if [ "$rc" = "0" ]; then ok "plan exits 0"; else bad "plan exits 0 (got $rc)"; fi
jok "plan json parses"        "$WORK/plan.json" "dry_run" "True"
jok "plan proposes 3 safe"    "$WORK/plan.json" "count"   "3"
jok "plan app name"           "$WORK/plan.json" "app"     "organizate-go"
$ORG --root "$FIX" >"$WORK/default.json" 2>/dev/null
jok "no command = plan"       "$WORK/default.json" "dry_run" "True"

echo "== apply without --apply changes nothing"
$ORG apply --root "$FIX" --all >"$WORK/dry.json" 2>/dev/null
rc=$?
if [ "$rc" = "0" ]; then ok "dry apply exits 0"; else bad "dry apply exits 0 (got $rc)"; fi
jok "apply reports dry_run"   "$WORK/dry.json" "dry_run" "True"
jok "apply applied nothing"   "$WORK/dry.json" "applied" "0"
WANT=$(jget "$WORK/dry.json" "would_apply")
if [ "$WANT" -gt "4" ]; then ok "would_apply counts the review list with --all ($WANT)"; else bad "would_apply = $WANT, want > 4"; fi
find "$FIX" | sort > "$WORK/afterdry.txt"
if diff -q "$WORK/before.txt" "$WORK/afterdry.txt" >/dev/null; then ok "dry run left the tree identical"; else bad "dry run changed the tree"; fi
missing "dry run created no state dir" "$FIX/.organizate"

echo "== apply --apply --only debris (the low-risk slice)"
$ORG apply --root "$FIX" --only debris --apply >"$WORK/apply.json" 2>/dev/null
rc=$?
if [ "$rc" = "0" ]; then ok "apply --apply exits 0"; else bad "apply --apply exits 0 (got $rc)"; fi
jok "dry_run false"           "$WORK/apply.json" "dry_run" "False"
APPLIED=$(jget "$WORK/apply.json" "applied")
if [ "$APPLIED" -ge "3" ]; then ok "applied $APPLIED debris moves"; else bad "applied = $APPLIED, want >= 3"; fi
exists "debris filed"           "$FIX/Organized/job-debris/apply-b12.log"
missing "stash NOT auto-moved"  "$FIX/Organized/stash/to-backup"
exists "stash still in place"   "$FIX/to-backup"
exists "project untouched"      "$FIX/someproj"
exists "hidden file untouched"  "$FIX/.bashrc"
exists "journal written"        "$FIX/.organizate/journal.jsonl"

echo "== undo is a dry run too"
$ORG undo --root "$FIX" --all >"$WORK/undry.json" 2>/dev/null
rc=$?
if [ "$rc" = "0" ]; then ok "dry undo exits 0"; else bad "dry undo exits 0 (got $rc)"; fi
jok "undo dry_run"            "$WORK/undry.json" "dry_run" "True"
jok "undo restored nothing"   "$WORK/undry.json" "undone"  "0"
missing "dry undo did not restore" "$FIX/apply-b12.log"
exists "dry undo kept the journal" "$FIX/.organizate/journal.jsonl"

echo "== undo --all --apply restores everything"
$ORG undo --root "$FIX" --all --apply >"$WORK/undo.json" 2>/dev/null
rc=$?
if [ "$rc" = "0" ]; then ok "undo --apply exits 0"; else bad "undo --apply exits 0 (got $rc)"; fi
UNDONE=$(jget "$WORK/undo.json" "undone")
if [ "$UNDONE" = "$APPLIED" ]; then ok "undone ($UNDONE) matches applied ($APPLIED)"; else bad "undone=$UNDONE applied=$APPLIED"; fi
# The tool's own scaffolding (journal dir, empty Organized/) is not user
# data — the unit-test snapshot and the machin suite exclude it too.
find "$FIX" | grep -v -e '/.organizate' -e '/Organized' -e '/Projects' | sort > "$WORK/afterundo.txt"
if diff -q "$WORK/before.txt" "$WORK/afterundo.txt" >/dev/null; then ok "round-trip: every original path is back exactly"; else bad "round-trip changed the tree"; diff "$WORK/before.txt" "$WORK/afterundo.txt" | head; fi
if [ ! -s "$FIX/.organizate/journal.jsonl" ]; then ok "journal empty after undo"; else bad "journal empty after undo"; fi

echo "== keep list"
$ORG keep add free-space-report.md --root "$FIX" >"$WORK/keep.json" 2>/dev/null
jok "keep action"            "$WORK/keep.json" "action" "add"
$ORG plan --root "$FIX" --all >"$WORK/plan2.json" 2>/dev/null
if grep -q 'free-space-report.md' "$WORK/plan2.json"; then bad "kept file still in plan"; else ok "kept file left the plan"; fi

echo "== exit codes"
exits "undo on empty journal exits 91"    91 $ORG undo --root "$FIX" --all --apply
exits "unknown command exits 85"          85 $ORG frobnicate --root "$FIX"
exits "undo with a bad argument exits 80" 80 $ORG undo plenty --root "$FIX"

echo "== human help"
$ORG help >"$WORK/help.txt" 2>"$WORK/help.err"
rc=$?
if [ "$rc" = "0" ]; then ok "help exits 0"; else bad "help exits 0 (got $rc)"; fi
has "help shows usage"          "usage: organizate-go" "$WORK/help.txt"
has "help states dry-run"       "dry-run by default"   "$WORK/help.txt"
has "help shows the --apply rule" "actually perform the moves" "$WORK/help.txt"
has "help shows examples"       "dry-run examples"     "$WORK/help.txt"
if [ -s "$WORK/help.err" ]; then bad "help writes nothing to stderr"; else ok "help writes nothing to stderr"; fi
$ORG --help >"$WORK/help2.txt" 2>&1
if diff -q "$WORK/help.txt" "$WORK/help2.txt" >/dev/null; then ok "--help prints the same help"; else bad "--help prints the same help"; fi
$ORG help-json >"$WORK/helpjson.json" 2>/dev/null
jok "help-json is JSON"        "$WORK/helpjson.json" "app" "organizate-go"
$ORG guide --human >"$WORK/guide.md" 2>/dev/null
has "guide explains the rule"  "dry-run by default" "$WORK/guide.md"

echo "== scan"
$ORG scan --root "$FIX" >"$WORK/scan.json" 2>/dev/null
jok "scan counts entries"      "$WORK/scan.json" "count" "13"
HAS_HIDDEN=$(python3 -c "
import json
d = json.load(open('$WORK/scan.json'))
print(sum(1 for i in d['items'] if i['name'] == '.bashrc' and i['action'] == 'none'))
")
if [ "$HAS_HIDDEN" = "1" ]; then ok "hidden file action=none in scan"; else bad "hidden file action=none in scan"; fi

echo "== spec conformance (skipped when the checker is not installed)"
if command -v cli-spec-conformance >/dev/null 2>&1; then
  if cli-spec-conformance check "$ORG" --specs output,guide >"$WORK/spec.json" 2>&1; then
    ok "cli-spec-conformance output,guide (19/19)"
  else
    bad "cli-spec-conformance output,guide"; tail -5 "$WORK/spec.json"
  fi
else
  echo "  skip cli-spec-conformance (not on PATH; see README §Spec conformance)"
fi

echo
echo "passed $PASS, failed $FAIL"
if [ "$FAIL" -gt 0 ]; then exit 1; fi
