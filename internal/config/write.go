package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Change is a single edit for Write: set Key to Literal inside [Section].
// Literal is the already-formatted TOML value (use BoolLiteral / IntLiteral /
// FloatLiteral / StringLiteral). The settings tab builds these only for the
// keys the user actually changed.
type Change struct {
	Section string
	Key     string
	Literal string
}

// Write applies changes to the TOML file at path in place, preserving
// comments, formatting and untouched keys (BurntSushi/toml has no
// comment-preserving encoder, and the settings tab must not clobber a
// hand-edited file). For each change: replace the value on the existing
// "key =" line within [section]; else insert the key just under the section
// header; else append a new [section] block at EOF. A missing or empty file is
// fine - sections and keys are appended. The write is atomic (temp + rename) so
// the daemon's config.Watch poller never reads a torn file.
func Write(path string, changes []Change) error {
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := strings.TrimSuffix(string(raw), "\n")
	var lines []string
	if text != "" {
		lines = strings.Split(text, "\n")
	}
	for _, ch := range changes {
		lines = applyChange(lines, ch)
	}
	out := strings.Join(lines, "\n")
	if out != "" {
		out += "\n"
	}
	return writeAtomic(path, []byte(out))
}

// applyChange edits one key, re-scanning the line slice each call so a run of
// changes to the same (possibly just-appended) section composes correctly.
func applyChange(lines []string, ch Change) []string {
	header := "[" + ch.Section + "]"
	newLine := ch.Key + " = " + ch.Literal

	secStart := -1
	for i, ln := range lines {
		if isHeader(ln, header) {
			secStart = i
			break
		}
	}
	if secStart == -1 {
		// Append a fresh section block at EOF, with a blank line before it so
		// it does not run into the previous section's last line.
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		return append(lines, header, newLine)
	}

	// The section runs until the next table header (or EOF).
	secEnd := len(lines)
	for i := secStart + 1; i < len(lines); i++ {
		if isAnyHeader(lines[i]) {
			secEnd = i
			break
		}
	}
	for i := secStart + 1; i < secEnd; i++ {
		if isKeyLine(lines[i], ch.Key) {
			lines[i] = replaceValue(lines[i], ch.Literal)
			return lines
		}
	}
	// Key absent: insert directly under the section header so it stays grouped
	// and never lands after a trailing comment/blank line.
	at := secStart + 1
	return append(lines[:at:at], append([]string{newLine}, lines[at:]...)...)
}

// isHeader reports whether line is exactly the given table header (ignoring
// surrounding whitespace and a trailing comment). Header lines carry no quoted
// strings, so stripping at '#' is safe here.
func isHeader(line, header string) bool {
	return strings.TrimSpace(stripComment(line)) == header
}

func isAnyHeader(line string) bool {
	t := strings.TrimSpace(stripComment(line))
	return strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]")
}

// isKeyLine reports whether line assigns key (key, then optional spaces, then
// '='), so "volume" does not match "volume_boost" or a key it is a prefix of.
func isKeyLine(line, key string) bool {
	t := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(t, key) {
		return false
	}
	rest := strings.TrimLeft(t[len(key):], " \t")
	return strings.HasPrefix(rest, "=")
}

// replaceValue swaps the value on an existing assignment line, preserving the
// leading indentation, the key, and any trailing inline comment.
func replaceValue(line, literal string) string {
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	body := line[len(indent):]
	keyPart, valPart, ok := strings.Cut(body, "=")
	if !ok { // not actually an assignment; leave it be
		return line
	}
	return indent + strings.TrimRight(keyPart, " \t") + " = " + literal + trailingComment(valPart)
}

// trailingComment returns the inline comment (with its leading gap) found in
// the value region s, or "" if there is none. A '#' inside a quoted string is
// not a comment.
func trailingComment(s string) string {
	inStr := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inStr:
			if c == quote {
				inStr = false
			}
		case c == '"' || c == '\'':
			inStr, quote = true, c
		case c == '#':
			j := i
			for j > 0 && (s[j-1] == ' ' || s[j-1] == '\t') {
				j--
			}
			gap := s[j:i]
			if gap == "" {
				gap = " "
			}
			return gap + s[i:]
		}
	}
	return ""
}

// stripComment drops a trailing '#' comment from a line that has no quoted
// string (header lines). Used only by the header helpers.
func stripComment(line string) string {
	before, _, _ := strings.Cut(line, "#")
	return before
}

// BoolLiteral, IntLiteral, FloatLiteral and StringLiteral format a value as the
// TOML literal Write should emit. FloatLiteral always carries a decimal point so
// a float field never receives an integer literal (which TOML rejects).
func BoolLiteral(b bool) string { return strconv.FormatBool(b) }

func IntLiteral(n int) string { return strconv.Itoa(n) }

func FloatLiteral(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

func StringLiteral(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// writeAtomic writes data to path via a temp file in the same directory and a
// rename, so a reader (the Watch poller) never sees a partially written file.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".agentbox-config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // harmless no-op once the rename succeeds
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
