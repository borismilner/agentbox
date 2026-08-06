// Package editor opens a cited file at a line in the human's own editor (FR65).
//
// Three things shape it.
//
// An invocation is an argv TEMPLATE, never a shell string. The argument order
// differs per editor and is fussy enough that guessing is worse than a table:
// `goland --line 1367 file.go` is rejected outright with "unrecognized option",
// the project directory has to come first, and VS Code wants the position glued
// to the path instead of carried as flags. A template with placeholders covers
// both shapes, and argv rather than a command line means a path with a space in
// it stays one argument and nothing here can become an execution. This is the
// same decision speech.command made, for the same reasons.
//
// An unset template is DETECTED rather than refused, because zero config is
// meant to give the full experience (NFR11). $EDITOR is deliberately not
// consulted: it is almost always a terminal editor and the daemon has no
// terminal to give it, so honouring it would produce a click that silently does
// nothing. Anybody who wants one writes the terminal into the template
// themselves - ["kitty", "-e", "nvim", "+{line}", "{file}"].
//
// The child is launched OUTSIDE the daemon's cgroup where systemd allows it.
// agentbox.service is KillMode=control-group and a Toolbox launcher script execs
// the IDE in the foreground, so an editor started as a plain child of the daemon
// is killed by the next `make deploy`. Anything the human opened must outlive
// the thing that opened it.
package editor

import (
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Target is one citation: a position in a file, and the project it belongs to.
// Dir and File are absolute. Line is 1-based; a zero Line or Col means "the
// top", which is what a block with no usable start reduces to.
type Target struct {
	Dir  string
	File string
	Line int
	Col  int
}

// The placeholders a template may use. {column} rather than {col} because a
// template is written by hand in a config file and read months later.
const (
	phDir  = "{dir}"
	phFile = "{file}"
	phLine = "{line}"
	phCol  = "{column}"
)

// Placeholders lists them for the documentation and the settings surface, so
// there is one source for what a template may say.
var Placeholders = []string{phDir, phFile, phLine, phCol}

// candidate is a launcher agentbox knows the argument shape of.
type candidate struct {
	bin  string
	args []string
}

// known is tried in order when no template is configured. The order is a guess
// and cannot be anything else - a machine with GoLand and VS Code on it does not
// say which one the reader wants - so the JetBrains family goes first for one
// concrete reason: with the project already open its launcher routes to the
// existing window rather than opening a second one, which is the behaviour this
// feature is for. Anybody the guess is wrong for sets editor.command.
//
// Only the JetBrains shape is verified live (07-field-requests.md, "Mechanics
// discovered", 2026-07-27, GoLand via Toolbox). The rest are each vendor's
// documented CLI and have not been exercised here.
var known = []candidate{
	{"goland", jetbrains}, {"idea", jetbrains}, {"pycharm", jetbrains},
	{"webstorm", jetbrains}, {"clion", jetbrains}, {"rustrover", jetbrains},
	{"phpstorm", jetbrains}, {"rubymine", jetbrains}, {"datagrip", jetbrains},
	{"code", vscode}, {"code-insiders", vscode}, {"codium", vscode},
	{"zed", []string{"zed", phFile + ":" + phLine + ":" + phCol}},
	{"subl", []string{"subl", phFile + ":" + phLine + ":" + phCol}},
	{"kate", []string{"kate", "--line", phLine, "--column", phCol, phFile}},
	{"gvim", []string{"gvim", "+" + phLine, phFile}},
	{"emacsclient", []string{"emacsclient", "-n", "+" + phLine + ":" + phCol, phFile}},
}

var (
	jetbrains = []string{"", phDir, "--line", phLine, "--column", phCol, phFile}
	vscode    = []string{"", "--goto", phFile + ":" + phLine + ":" + phCol}
)

// fallback is the last resort: the desktop's own handler for the file type,
// which loses the line. Losing the line is worth saying out loud - Resolve
// reports it as the source so a caller can tell the human why they landed at
// the top of the file.
const fallback = "xdg-open"

// ErrNoFile is a template that never names the file. It compiles and runs and
// raises an editor on nothing, which reads as a broken button rather than as a
// broken config, so it is refused at the point the template is read.
var ErrNoFile = errors.New("an editor command must contain " + phFile)

// Resolve picks the argv template to use and says where it came from. The
// source is for the log and for what the human is told: "config", a launcher
// name, or "xdg-open" - the one case where the line is dropped.
func Resolve(configured []string) (tmpl []string, source string, err error) {
	if len(configured) > 0 {
		if !names(configured, phFile) {
			return nil, "config", ErrNoFile
		}
		return configured, "config", nil
	}
	for _, c := range known {
		if _, err := exec.LookPath(c.bin); err != nil {
			continue
		}
		t := append([]string(nil), c.args...)
		t[0] = c.bin // the family templates share one slice; argv[0] is per binary
		return t, c.bin, nil
	}
	if _, err := exec.LookPath(fallback); err != nil {
		return nil, "", errors.New("no editor found; set editor.command in config.toml")
	}
	return []string{fallback, phFile}, fallback, nil
}

func names(tmpl []string, ph string) bool {
	for _, w := range tmpl {
		if strings.Contains(w, ph) {
			return true
		}
	}
	return false
}

// Expand fills a template with one target. A placeholder is substituted INSIDE a
// word, not only as a whole word, because the two families disagree about the
// shape: JetBrains carries the position as its own flags and VS Code glues it to
// the path. A word that was one argument stays one argument either way, so a
// space in a path cannot split it.
func Expand(tmpl []string, t Target) ([]string, error) {
	if len(tmpl) == 0 {
		return nil, errors.New("no editor command")
	}
	if strings.TrimSpace(tmpl[0]) == "" {
		return nil, errors.New("an editor command must start with the program to run")
	}
	if !names(tmpl, phFile) {
		return nil, ErrNoFile
	}
	line, col := t.Line, t.Col
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	r := strings.NewReplacer(
		phDir, t.Dir,
		phFile, t.File,
		phLine, strconv.Itoa(line),
		phCol, strconv.Itoa(col),
	)
	out := make([]string, len(tmpl))
	for i, w := range tmpl {
		out[i] = r.Replace(w)
	}
	return out, nil
}

// Command resolves and expands in one step: the whole path from "the reader
// clicked open" to an argv, with nothing run yet. Split from Start so a caller
// can log the exact command, and so all of it is testable without launching an
// IDE.
func Command(configured []string, t Target) (argv []string, source string, err error) {
	tmpl, source, err := Resolve(configured)
	if err != nil {
		return nil, source, err
	}
	argv, err = Expand(tmpl, t)
	if err != nil {
		return nil, source, err
	}
	if _, err := exec.LookPath(argv[0]); err != nil {
		return nil, source, fmt.Errorf("editor %q is not on PATH", argv[0])
	}
	return argv, source, nil
}

// escape wraps argv so the child lands in a transient scope of its own instead
// of in agentbox.service's control group. Without it the editor is killed with the
// daemon - KillMode=control-group, and a JetBrains Toolbox script execs the IDE
// rather than forking it, so the IDE IS the child. Cold-start only in practice:
// with the IDE already up the launcher is a short-lived client that hands over
// and exits. That is exactly the case that would be missed in testing and hit in
// use, on the deploy right after the reader opened a file.
//
// Returns argv untouched when systemd-run is unavailable, which is the correct
// behaviour outside a systemd user session: there is no cgroup to escape.
func escape(argv []string) []string {
	if _, err := exec.LookPath("systemd-run"); err != nil {
		return argv
	}
	// No --unit: a fixed name would refuse the second launch while the first
	// scope is still alive, which is the ordinary case (one IDE, many files).
	return append([]string{
		"systemd-run", "--user", "--scope", "--quiet", "--collect", "--",
	}, argv...)
}

// Start launches a resolved argv and returns as soon as it is running. It never
// waits: an editor window is open for the rest of the day, and the exit status
// would not be worth the wait anyway - goland exits 0 even when it rejects its
// arguments, so the status cannot tell a launch from a refusal.
//
// The error is only ever a failure to START. Wait is left to the caller through
// the returned command so a caller that wants to notice an immediate crash can,
// without this function deciding how long "immediate" is.
func Start(argv []string) (*exec.Cmd, error) {
	full := escape(argv)
	cmd := exec.Command(full[0], full[1:]...)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}
