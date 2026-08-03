package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const saveVersion = 1

// SessionsDir is where saved conversations live, mirroring agentbox's state dir
// (XDG_STATE_HOME/agentbox[-instance]/sessions, else ~/.local/state/...), so the
// instance that wrote them owns them (NFR12).
func SessionsDir() string {
	name := "agentbox"
	if inst := os.Getenv("AGENTBOX_INSTANCE"); inst != "" {
		name = "agentbox-" + inst
	}
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, name, "sessions")
}

// savedSegment mirrors Segment, so a saved file is small and stable across
// renderer changes.
type savedSegment struct {
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	ToolID    string `json:"tool_id,omitempty"`
	ToolInput string `json:"tool_input,omitempty"`
	Result    string `json:"result,omitempty"`
	HasResult bool   `json:"has_result,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type savedTurn struct {
	Role     Role           `json:"role"`
	Model    string         `json:"model,omitempty"`
	CostUSD  float64        `json:"cost_usd,omitempty"`
	Err      string         `json:"err,omitempty"`
	At       string         `json:"at,omitempty"`       // RFC3339; the clock the turn showed
	ThinkMS  int64          `json:"think_ms,omitempty"` // how long the model worked for it
	Segments []savedSegment `json:"segments"`
}

type savedConversation struct {
	Version int    `json:"version"`
	SavedAt string `json:"saved_at,omitempty"`
	// What it takes to carry on rather than only to read: the Claude session id
	// --resume needs, and the directory and mode the child ran in.
	SessionID string      `json:"session_id,omitempty"`
	Cwd       string      `json:"cwd,omitempty"`
	Mode      string      `json:"mode,omitempty"`
	Title     string      `json:"title,omitempty"`
	Turns     []savedTurn `json:"turns"`
}

// Meta is what a conversation needs to be reopened rather than just read.
type Meta struct {
	SessionID string // Claude's own id, for --resume
	Cwd       string
	Mode      string
	Title     string
}

// Saved describes one conversation on disk, for a surface that offers to reopen
// one.
type Saved struct {
	Path    string
	SavedAt time.Time
	Turns   int
	Meta    Meta
	// Preview is the first thing the human said in it, which is a better name for
	// a conversation than its timestamp.
	Preview string
}

var segKindNames = map[SegKind]string{
	SegText:       "text",
	SegThinking:   "thinking",
	SegToolUse:    "tool_use",
	SegToolResult: "tool_result",
}

func segKindName(k SegKind) string {
	if s, ok := segKindNames[k]; ok {
		return s
	}
	return "text"
}

func segKindFrom(s string) SegKind {
	for k, name := range segKindNames {
		if name == s {
			return k
		}
	}
	return SegText
}

// Marshal serialises a conversation to JSON (without the parsed Documents).
func Marshal(turns []Turn) ([]byte, error) {
	return marshalAt(turns, Meta{}, time.Now())
}

func marshalAt(turns []Turn, m Meta, now time.Time) ([]byte, error) {
	sc := savedConversation{
		Version: saveVersion, SavedAt: now.UTC().Format(time.RFC3339),
		SessionID: m.SessionID, Cwd: m.Cwd, Mode: m.Mode, Title: m.Title,
	}
	for _, t := range turns {
		st := savedTurn{Role: t.Role, Model: t.Model, CostUSD: t.CostUSD, Err: t.Err}
		if !t.At.IsZero() {
			st.At = t.At.UTC().Format(time.RFC3339)
		}
		if t.Think > 0 {
			st.ThinkMS = t.Think.Milliseconds()
		}
		for _, s := range t.Segments {
			st.Segments = append(st.Segments, savedSegment{
				Kind: segKindName(s.Kind), Text: s.Text,
				ToolName: s.ToolName, ToolID: s.ToolID, ToolInput: s.ToolInput,
				Result: s.Result, HasResult: s.HasResult, IsError: s.IsError,
			})
		}
		sc.Turns = append(sc.Turns, st)
	}
	return json.MarshalIndent(sc, "", "  ")
}

// Unmarshal rebuilds a conversation from JSON. Segments carry their markdown
// source, so a reloaded turn renders exactly as a live one does.
func Unmarshal(data []byte) ([]Turn, error) {
	turns, _, err := UnmarshalWithMeta(data)
	return turns, err
}

// UnmarshalWithMeta also returns what it takes to carry the conversation on: the
// Claude session id, the directory and the mode.
func UnmarshalWithMeta(data []byte) ([]Turn, Meta, error) {
	var sc savedConversation
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, Meta{}, err
	}
	meta := Meta{SessionID: sc.SessionID, Cwd: sc.Cwd, Mode: sc.Mode, Title: sc.Title}
	turns := make([]Turn, 0, len(sc.Turns))
	for _, st := range sc.Turns {
		t := Turn{Role: st.Role, Model: st.Model, CostUSD: st.CostUSD, Err: st.Err}
		if st.At != "" {
			if at, err := time.Parse(time.RFC3339, st.At); err == nil {
				t.At = at.Local()
			}
		}
		if st.ThinkMS > 0 {
			t.Think = time.Duration(st.ThinkMS) * time.Millisecond
		}
		for _, s := range st.Segments {
			seg := Segment{
				Kind: segKindFrom(s.Kind), Text: s.Text,
				ToolName: s.ToolName, ToolID: s.ToolID, ToolInput: s.ToolInput,
				Result: s.Result, HasResult: s.HasResult, IsError: s.IsError,
			}
			t.Segments = append(t.Segments, seg)
		}
		turns = append(turns, t)
	}
	return turns, meta, nil
}

// ToMarkdown renders a conversation as a human-readable markdown export.
func ToMarkdown(turns []Turn) string {
	var b strings.Builder
	b.WriteString("# Claude session\n")
	for _, t := range turns {
		fmt.Fprintf(&b, "\n## %s\n\n", roleHeading(t.Role))
		for _, s := range t.Segments {
			switch s.Kind {
			case SegToolUse, SegToolResult:
				name := s.ToolName
				if name == "" {
					name = "tool"
				}
				fmt.Fprintf(&b, "**%s** `%s`\n", name, s.ToolInput)
				if s.HasResult && s.Result != "" {
					fmt.Fprintf(&b, "\n> %s\n", strings.ReplaceAll(s.Result, "\n", "\n> "))
				}
				b.WriteString("\n")
			case SegThinking:
				fmt.Fprintf(&b, "_%s_\n\n", s.Text)
			default:
				b.WriteString(s.Text)
				b.WriteString("\n\n")
			}
		}
	}
	return b.String()
}

func roleHeading(r Role) string {
	switch r {
	case RoleUser:
		return "You"
	case RoleAssistant:
		return "Claude"
	default:
		return "System"
	}
}

// Save writes a conversation as JSON (for reload) plus a markdown sibling (for
// reading), under dir, named by timestamp. It returns the JSON path.
func Save(dir string, turns []Turn) (string, error) {
	return saveAt(dir, turns, Meta{}, time.Now())
}

// SaveAs is Save with what it takes to carry the conversation on: the Claude
// session id, its directory and its mode.
func SaveAs(dir string, turns []Turn, m Meta) (string, error) {
	return saveAt(dir, turns, m, time.Now())
}

// SaveInto overwrites one file rather than making a new one, so a session that is
// saved repeatedly (on every reply, say) does not leave a directory full of
// snapshots of itself. The name is the caller's stable id for the conversation.
func SaveInto(dir, name string, turns []Turn, m Meta) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := marshalAt(turns, m, time.Now())
	if err != nil {
		return "", err
	}
	jsonPath := filepath.Join(dir, name+".json")
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return "", err
	}
	mdPath := filepath.Join(dir, name+".md")
	if err := os.WriteFile(mdPath, []byte(ToMarkdown(turns)), 0o644); err != nil {
		return jsonPath, err
	}
	return jsonPath, nil
}

func saveAt(dir string, turns []Turn, m Meta, now time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stamp := now.Format("20060102-150405")
	jsonPath := filepath.Join(dir, "session-"+stamp+".json")
	data, err := marshalAt(turns, m, now)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return "", err
	}
	mdPath := filepath.Join(dir, "session-"+stamp+".md")
	if err := os.WriteFile(mdPath, []byte(ToMarkdown(turns)), 0o644); err != nil {
		return jsonPath, err // JSON is the source of truth; the .md is a bonus
	}
	return jsonPath, nil
}

// Read loads one saved conversation by path.
func Read(path string) ([]Turn, Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, Meta{}, err
	}
	return UnmarshalWithMeta(data)
}

// List describes the saved conversations in dir, newest first, at most max of
// them. A conversation is named by what was first asked in it rather than by its
// timestamp, because that is how a human recognises one.
func List(dir string, max int) []Saved {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Saved
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var sc savedConversation
		if err := json.Unmarshal(data, &sc); err != nil {
			continue
		}
		sv := Saved{
			Path:  path,
			Turns: len(sc.Turns),
			Meta:  Meta{SessionID: sc.SessionID, Cwd: sc.Cwd, Mode: sc.Mode, Title: sc.Title},
		}
		if at, err := time.Parse(time.RFC3339, sc.SavedAt); err == nil {
			sv.SavedAt = at.Local()
		} else if info, err := e.Info(); err == nil {
			sv.SavedAt = info.ModTime()
		}
		sv.Preview = firstPrompt(sc.Turns)
		out = append(out, sv)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SavedAt.After(out[j].SavedAt) })
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

// firstPrompt is the first thing the human said, trimmed to a line.
func firstPrompt(turns []savedTurn) string {
	for _, t := range turns {
		if t.Role != RoleUser {
			continue
		}
		for _, s := range t.Segments {
			if txt := strings.TrimSpace(s.Text); txt != "" {
				return truncate(txt, 80)
			}
		}
	}
	return ""
}

// LoadLatest reopens the most recently saved conversation in dir, returning its
// turns and path. A missing dir or no saved sessions is reported as an error.
func LoadLatest(dir string) ([]Turn, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, "", err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	if len(files) == 0 {
		return nil, "", fmt.Errorf("session: no saved conversations in %s", dir)
	}
	sort.Strings(files) // timestamp names sort chronologically
	path := filepath.Join(dir, files[len(files)-1])
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	turns, err := Unmarshal(data)
	return turns, path, err
}
