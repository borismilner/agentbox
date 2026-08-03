// Package assign is what an assignment IS, apart from where it is stored and
// what runs it: the definition, its typed parameters, the substitution that
// turns a template into a prompt, and the schedule grammar that decides when the
// next one is due.
//
// It is a separate package for the reason monitor.go is separate from x11.go -
// this is the half worth testing. A scheduler needs a clock and a runner needs a
// child process; a due time is arithmetic and a substitution is a string.
package assign

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Assignment is the definition a human and an agent both edit.
type Assignment struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Prompt      string  `json:"prompt"`
	Spec        []Param `json:"spec"`
	// Params are the current values, by key. Kept as a plain map rather than
	// folded into Spec because the values outlive any one version of the spec:
	// an agent that rewrites the knobs must not silently discard what was set.
	Params    map[string]any `json:"params"`
	PanelHTML string         `json:"panelHtml,omitempty"`
	Model     string         `json:"model,omitempty"`
	Mode      string         `json:"mode,omitempty"` // plan | full
	Dir       string         `json:"dir,omitempty"`
	Schedule  string         `json:"schedule,omitempty"`
	Enabled   bool           `json:"enabled"`

	CreatedMS int64 `json:"createdMs,omitempty"`
	UpdatedMS int64 `json:"updatedMs,omitempty"`
	LastRunMS int64 `json:"lastRunMs,omitempty"`
	NextRunMS int64 `json:"nextRunMs,omitempty"`
}

// Param is one typed knob. The set is deliberately small: every type here has an
// obvious control, an obvious validation and an obvious string form for
// substitution. A type that needs an explanation belongs in the custom panel
// instead (docs/08-assignments.md).
type Param struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
	Type  string `json:"type"`
	Help  string `json:"help,omitempty"`

	Default any `json:"default,omitempty"`

	// number and slider
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
	Step *float64 `json:"step,omitempty"`
	Unit string   `json:"unit,omitempty"`

	// enum
	Values []string `json:"values,omitempty"`

	// text
	Multiline bool `json:"multiline,omitempty"`

	// markdown: prose in the panel rather than an input. Body carries it, and
	// the knob has no key and no value.
	Body string `json:"body,omitempty"`
}

// Parameter types.
const (
	TypeText     = "text"
	TypeNumber   = "number"
	TypeSlider   = "slider"
	TypeToggle   = "toggle"
	TypeEnum     = "enum"
	TypePath     = "path"
	TypeMarkdown = "markdown"
)

// Inputs are the types that carry a value. Markdown is the one that does not:
// it is a paragraph in the middle of the panel, which is what makes a generated
// panel read as a form somebody designed rather than a list of fields.
func (p Param) Input() bool { return p.Type != TypeMarkdown }

// Validate reports every problem with a spec at once, because an agent writing
// one wants the whole list back, not the first line of it.
func Validate(spec []Param) []string {
	var probs []string
	seen := map[string]bool{}
	for i, p := range spec {
		where := fmt.Sprintf("param %d", i+1)
		if p.Key != "" {
			where = fmt.Sprintf("param %q", p.Key)
		}
		switch p.Type {
		case TypeMarkdown:
			if strings.TrimSpace(p.Body) == "" {
				probs = append(probs, where+": a markdown block with no body renders as a gap")
			}
			continue
		case TypeText, TypeNumber, TypeSlider, TypeToggle, TypeEnum, TypePath:
		case "":
			probs = append(probs, where+": no type")
			continue
		default:
			probs = append(probs, where+": unknown type "+strconv.Quote(p.Type))
			continue
		}
		if p.Key == "" {
			probs = append(probs, where+": no key, so nothing in the prompt can reach it")
			continue
		}
		if seen[p.Key] {
			probs = append(probs, where+": duplicate key")
		}
		seen[p.Key] = true
		if p.Type == TypeEnum && len(p.Values) == 0 {
			probs = append(probs, where+": an enum with no values has nothing to choose")
		}
		if p.Type == TypeSlider {
			if p.Min == nil || p.Max == nil {
				probs = append(probs, where+": a slider needs min and max")
			} else if *p.Min >= *p.Max {
				probs = append(probs, where+": min is not below max")
			}
		}
	}
	return probs
}

// Defaults is the value map a spec starts from.
func Defaults(spec []Param) map[string]any {
	out := map[string]any{}
	for _, p := range spec {
		if !p.Input() || p.Key == "" {
			continue
		}
		if p.Default != nil {
			out[p.Key] = p.Default
			continue
		}
		switch p.Type {
		case TypeToggle:
			out[p.Key] = false
		case TypeNumber, TypeSlider:
			if p.Min != nil {
				out[p.Key] = *p.Min
			} else {
				out[p.Key] = float64(0)
			}
		case TypeEnum:
			if len(p.Values) > 0 {
				out[p.Key] = p.Values[0]
			} else {
				out[p.Key] = ""
			}
		default:
			out[p.Key] = ""
		}
	}
	return out
}

// Merge folds stored values into a spec's defaults, keeping a value whose knob
// still exists and dropping one whose knob does not. This is what an agent
// rewriting the panel must not be able to break: the knobs change, the settings
// he chose survive as far as they still mean anything.
func Merge(spec []Param, have map[string]any) map[string]any {
	out := Defaults(spec)
	for k, v := range have {
		if _, ok := out[k]; ok {
			out[k] = v
		}
	}
	return out
}

// Render substitutes {{key}} in the template. It also reports the placeholders
// that had nothing behind them, so a missing parameter is something the surface
// says at save time rather than something the agent discovers at 3am with a
// literal "{{threshold}}" in its instructions.
//
// Substitution is deliberately literal and non-recursive: a value that itself
// contains {{...}} is data, not another template. An assignment prompt is
// handed to a model, and a template language with corners is a template language
// with an injection in one of them.
func Render(tmpl string, params map[string]any) (out string, missing []string) {
	var b strings.Builder
	miss := map[string]bool{}
	for {
		i := strings.Index(tmpl, "{{")
		if i < 0 {
			break
		}
		j := strings.Index(tmpl[i:], "}}")
		if j < 0 {
			break
		}
		j += i
		key := strings.TrimSpace(tmpl[i+2 : j])
		b.WriteString(tmpl[:i])
		if v, ok := params[key]; ok {
			b.WriteString(valueString(v))
		} else {
			// Left in place, on purpose: a prompt that silently loses a clause is
			// worse than one that visibly carries an unfilled slot.
			b.WriteString(tmpl[i : j+2])
			miss[key] = true
		}
		tmpl = tmpl[j+2:]
	}
	b.WriteString(tmpl)
	for k := range miss {
		missing = append(missing, k)
	}
	sort.Strings(missing)
	return b.String(), missing
}

// Placeholders lists every {{key}} a template refers to, in first-seen order.
// The surface uses it to tell an author which knobs the prompt is asking for.
func Placeholders(tmpl string) []string {
	var out []string
	seen := map[string]bool{}
	for {
		i := strings.Index(tmpl, "{{")
		if i < 0 {
			return out
		}
		j := strings.Index(tmpl[i:], "}}")
		if j < 0 {
			return out
		}
		j += i
		if k := strings.TrimSpace(tmpl[i+2 : j]); k != "" && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
		tmpl = tmpl[j+2:]
	}
}

// valueString is how a parameter reads inside a prompt. Numbers lose a trailing
// ".0" because "85%" is what somebody wrote on the slider and "85.000000%" is
// what a naive format produces; a bool reads as yes/no because that is what it
// means in a sentence, not as Go's true/false.
func valueString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case bool:
		if t {
			return "yes"
		}
		return "no"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}

// Kind is what the panel calls this assignment's trigger.
func Kind(schedule string) string {
	switch {
	case strings.TrimSpace(schedule) == "":
		return "ad-hoc"
	case strings.HasPrefix(strings.TrimSpace(schedule), "every "):
		return "periodic"
	default:
		return "scheduled"
	}
}

// ParseSchedule reads the grammar in docs/08-assignments.md. An empty schedule
// is ad-hoc and valid - it is not an error to have no schedule, it is a choice.
func ParseSchedule(s string) (Schedule, error) {
	f := strings.Fields(strings.ToLower(strings.TrimSpace(s)))
	if len(f) == 0 {
		return Schedule{}, nil
	}
	switch f[0] {
	case "every":
		if len(f) != 2 {
			return Schedule{}, fmt.Errorf("every: want one interval like %q, got %q", "every 30m", s)
		}
		d, err := parseEvery(f[1])
		if err != nil {
			return Schedule{}, err
		}
		return Schedule{Every: d}, nil
	case "daily":
		if len(f) != 2 {
			return Schedule{}, fmt.Errorf("daily: want a time like %q, got %q", "daily 09:00", s)
		}
		h, m, err := parseClock(f[1])
		if err != nil {
			return Schedule{}, err
		}
		return Schedule{Hour: h, Min: m, Daily: true}, nil
	case "weekly":
		if len(f) != 3 {
			return Schedule{}, fmt.Errorf("weekly: want a day and a time like %q, got %q", "weekly mon 09:00", s)
		}
		wd, ok := weekday(f[1])
		if !ok {
			return Schedule{}, fmt.Errorf("weekly: %q is not a day (mon..sun)", f[1])
		}
		h, m, err := parseClock(f[2])
		if err != nil {
			return Schedule{}, err
		}
		return Schedule{Hour: h, Min: m, Weekday: wd, Weekly: true}, nil
	}
	return Schedule{}, fmt.Errorf("schedule: %q is not one of every/daily/weekly", f[0])
}

// Schedule is a parsed trigger. The zero value is ad-hoc, which is why every
// caller can treat "no schedule" as "never due" without a special case.
type Schedule struct {
	Every   time.Duration
	Hour    int
	Min     int
	Weekday time.Weekday
	Daily   bool
	Weekly  bool
}

func (s Schedule) AdHoc() bool { return s.Every == 0 && !s.Daily && !s.Weekly }

// Next is when this schedule is next due, strictly after from.
//
// An interval counts from the LAST RUN when there is one, and from now when
// there is not - so an assignment created at 14:03 with "every 1h" first runs at
// 15:03 rather than immediately. Creating something must not be the same
// gesture as running it; that is what the Run button is.
func (s Schedule) Next(from, last time.Time) time.Time {
	switch {
	case s.Every > 0:
		base := from
		if !last.IsZero() && last.After(from.Add(-s.Every)) {
			base = last
		}
		return base.Add(s.Every)
	case s.Daily:
		t := time.Date(from.Year(), from.Month(), from.Day(), s.Hour, s.Min, 0, 0, from.Location())
		if !t.After(from) {
			t = t.AddDate(0, 0, 1)
		}
		return t
	case s.Weekly:
		t := time.Date(from.Year(), from.Month(), from.Day(), s.Hour, s.Min, 0, 0, from.Location())
		for i := range 8 {
			c := t.AddDate(0, 0, i)
			if c.Weekday() == s.Weekday && c.After(from) {
				return c
			}
		}
		return time.Time{}
	}
	return time.Time{}
}

// Missed counts the slots between due and now that will never run, which is what
// the panel reports instead of firing them all (Boris, 2026-08-01: skip and say
// so). It is capped, because a laptop shut for a month should say "a lot", not
// spend a millisecond counting to forty thousand.
func (s Schedule) Missed(due, now time.Time) int {
	if due.IsZero() || !due.Before(now) {
		return 0
	}
	const cap = 999
	switch {
	case s.Every > 0:
		n := int(now.Sub(due) / s.Every)
		return min(n+1, cap)
	case s.Daily:
		return min(int(now.Sub(due)/(24*time.Hour))+1, cap)
	case s.Weekly:
		return min(int(now.Sub(due)/(7*24*time.Hour))+1, cap)
	}
	return 0
}

// parseEvery takes the short forms a person writes on a schedule: 30m, 4h, 1d.
// Below a minute is refused rather than clamped - a schedule of "every 5s" is
// a mistake, and spawning a Claude child five times a minute is an expensive
// one to discover by watching it happen.
func parseEvery(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("every: %q is not a number of days", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("every: %q is not an interval (try 30m, 4h, 1d)", s)
	}
	if d < time.Minute {
		return 0, fmt.Errorf("every: %s is below the one-minute floor", s)
	}
	return d, nil
}

func parseClock(s string) (int, int, error) {
	h, m, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, fmt.Errorf("time: %q is not HH:MM", s)
	}
	hi, err1 := strconv.Atoi(h)
	mi, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hi < 0 || hi > 23 || mi < 0 || mi > 59 {
		return 0, 0, fmt.Errorf("time: %q is not a time of day", s)
	}
	return hi, mi, nil
}

func weekday(s string) (time.Weekday, bool) {
	for d, n := range []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"} {
		if strings.HasPrefix(s, n) {
			return time.Weekday(d), true
		}
	}
	return 0, false
}
