// Command organizate-go is the agent-first variation of organizate: a tidy
// planner that is JSON-by-default and DRY-RUN BY DEFAULT.
//
// Contract (cli-output-spec, same family as the machin build):
//
//	data on stdout, context on stderr, typed errors whose body carries the
//	same number as the exit status, semantic exit codes 80-119.
//
// The one rule that makes it safe to hand to an agent: NO COMMAND MUTATES
// ANYTHING WITHOUT --apply. `apply` and `undo` print exactly what they would
// do and exit 0; passing --apply performs the moves and journals them.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const App = "organizate-go"
const Version = "1.0.0"

// Semantic exit codes (cli-output-spec §2).
const (
	ExitOK             = 0
	ExitMissingArg     = 80
	ExitUnknownCommand = 85
	ExitPrecondition   = 90
	ExitNothingToUndo  = 91
	ExitExternal       = 100
	ExitInternal       = 110
)

// valueFlags are the flags that consume the next token, so their values are
// never mistaken for the command or a positional argument.
var valueFlags = map[string]bool{
	"--root": true, "--dest": true, "--state": true, "--only": true,
}

// takesValue reports whether a flag token consumes the next token as its
// value (--root /x or -root /x). A token carrying "=" already has its value
// attached, and bool flags (--all, --apply) never take one.
func takesValue(tok string) bool {
	if strings.Contains(tok, "=") {
		return false
	}
	return valueFlags["--"+strings.TrimLeft(tok, "-")]
}

// positionals returns the non-flag tokens (flag values skipped). The first
// one is the command; the rest are positionals like `undo 5` or `keep add x`.
func positionals(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 0 && a[0] == '-' {
			if takesValue(a) {
				i++
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

func main() {
	args := os.Args[1:]
	pos := positionals(args)

	// Help and version win over everything, including a missing command.
	if hasFlag(args, "--help") || hasFlag(args, "-h") {
		printHelp()
		return
	}
	if hasFlag(args, "--version") || hasFlag(args, "-v") {
		fmt.Printf("%s v%s\n", App, Version)
		return
	}

	cmd := "plan" // dry-run default: no command means "show me the plan"
	if len(pos) > 0 {
		cmd = pos[0]
	}

	switch cmd {
	case "scan":
		cmdScan(args)
	case "plan":
		cmdPlan(args)
	case "apply":
		cmdApply(args)
	case "undo":
		cmdUndo(args, pos[1:])
	case "keep":
		cmdKeep(args, pos[1:])
	case "guide":
		if hasFlag(args, "--human") {
			fmt.Println(guideMarkdown())
		} else {
			fmt.Println(guideJSON())
		}
	case "help-json":
		fmt.Println(helpJSON())
	case "version":
		handleVersion(args)
	case "help":
		printHelp()
	default:
		die(ExitUnknownCommand, "unknown_command",
			fmt.Sprintf("unknown command %q", cmd),
			"organizate-go help-json")
	}
}

// newFlags is the shared FlagSet shape: stderr for parse errors (stdout stays
// clean for data), unknown flags are an input error, not a crash.
func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// addCommon wires the flags every command shares. Defaults are resolved by
// resolveTarget AFTER parsing, so `--root /x` alone still gives
// /x/Organized — a flag default must not depend on another flag.
func addCommon(fs *flag.FlagSet) (root, dest, state, only *string, human *bool) {
	root = fs.String("root", "", "what to tidy (default $ORGANIZATE_HOME or $HOME)")
	dest = fs.String("dest", "", "where things get filed (default <root>/Organized)")
	state = fs.String("state", "", "journal + keep list (default <root>/.organizate)")
	only = fs.String("only", "", "narrow to one category (debris, logs, scripts, ...)")
	human = fs.Bool("human", false, "render for a person instead of JSON")
	return
}

// flagTokens keeps only the flag tokens and the values they consume. This
// filter is load-bearing: Go's flag.Parse halts at the first non-flag token,
// so the command name sitting at args[0] ("plan", "undo", …) would make it
// return WITHOUT PARSEING A SINGLE FLAG — every --root would silently fall
// back to $HOME. Positionals are read via positionals(args) instead.
func flagTokens(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) == 0 || a[0] != '-' {
			continue // drop the command name and positionals
		}
		out = append(out, a)
		if takesValue(a) && i+1 < len(args) {
			i++
			out = append(out, args[i])
		}
	}
	return out
}

func parseFlags(fs *flag.FlagSet, args []string) {
	if err := fs.Parse(flagTokens(args)); err != nil {
		if err == flag.ErrHelp {
			fs.SetOutput(os.Stdout)
			fs.PrintDefaults()
			os.Exit(ExitOK)
		}
		die(ExitMissingArg, "bad_flags", err.Error(), "organizate-go help")
	}
}

func handleVersion(args []string) {
	if hasFlag(args, "--json") {
		out, _ := json.Marshal(map[string]string{"version": Version, "name": App})
		fmt.Println(string(out))
		return
	}
	fmt.Printf("%s v%s\n", App, Version)
}

// die emits a typed error on stdout and exits with the matching code: the
// exit status and .error.code are the same number by construction.
func die(code int, etype, message, suggestion string) {
	body := map[string]any{
		"ok": false,
		"error": map[string]any{
			"code":        code,
			"type":        etype,
			"message":     message,
			"recoverable": code >= 100 && code <= 109,
			"suggestions": []string{suggestion},
		},
	}
	out, _ := json.Marshal(body)
	fmt.Println(string(out))
	os.Exit(code)
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == name {
			return true
		}
	}
	return false
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseIntOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func eprintf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format, a...)
}
