package webui

import (
	"fmt"
	"slices"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

// The history surface (FR35): how many interruptions arrived, who sent
// them, and how fast they got answered.
//
// The Gio version rendered this through the markdown engine because laying out
// a table and a bar chart by hand in Gio was the expensive part. A webview has
// tables and bars for free, so the Go side stops at the numbers: it queries,
// formats the few values that have a house style (durations, window labels) and
// hands over a shape the surface can paint directly.

// statsWindows are the windows the segmented control offers.
var statsWindows = []string{"24h", "7d", "30d", "all"}

type wireAgentStat struct {
	Agent     string `json:"agent"`
	Hue       string `json:"hue"`
	Total     int    `json:"total"`
	Questions int    `json:"questions"`
	Answered  int    `json:"answered"`
	Median    string `json:"median"`
}

type wireDay struct {
	Day   string `json:"day"`   // YYYY-MM-DD
	Label string `json:"label"` // "Mon 21"
	Count int    `json:"count"`
}

type wireStats struct {
	Window   string `json:"window"` // "7d", echoed so the control can highlight
	Label    string `json:"label"`  // "last 7 days"
	Total    int    `json:"total"`
	Question int    `json:"questions"`
	Answered int    `json:"answered"`
	// AnsweredPct is answered/questions, the number the summary actually means:
	// what share of the things that waited on you got an answer.
	AnsweredPct int    `json:"answeredPct"`
	Median      string `json:"median"`
	PerDay      string `json:"perDay"`

	ByAgent []wireAgentStat `json:"byAgent"`
	ByDay   []wireDay       `json:"byDay"`
	Peak    int             `json:"peak"` // tallest day, so the bars can scale
	Empty   bool            `json:"empty"`
}

// Stats queries one window and encodes it. Errors come back as an empty
// snapshot with the window intact, so the surface shows "nothing yet" rather
// than a blank panel with no controls.
func (u *UI) statsFor(window string) wireStats {
	if !validWindow(window) {
		window = "7d"
	}
	src := u.source()
	if src == nil {
		return wireStats{Window: window, Label: windowLabel(window), Median: "-", PerDay: "-", Empty: true,
			ByAgent: []wireAgentStat{}, ByDay: []wireDay{}}
	}
	st, err := src.Stats(sinceTime(window))
	if err != nil {
		u.log.Error("webui.stats_failed", "component", "webui", "err", err.Error())
		return wireStats{Window: window, Label: windowLabel(window), Median: "-", PerDay: "-", Empty: true,
			ByAgent: []wireAgentStat{}, ByDay: []wireDay{}}
	}
	return encodeStats(st, window, u.themeMode() == "dark")
}

func encodeStats(st proto.Stats, window string, dark bool) wireStats {
	w := wireStats{
		Window:   window,
		Label:    windowLabel(window),
		Total:    st.Total,
		Question: st.Questions,
		Answered: st.Answered,
		Median:   fmtDurMS(st.MedianAnswerMS),
		Empty:    st.Total == 0,
		ByAgent:  make([]wireAgentStat, 0, len(st.ByAgent)),
		ByDay:    make([]wireDay, 0, len(st.ByDay)),
	}
	if st.Questions > 0 {
		w.AnsweredPct = int((float64(st.Answered)/float64(st.Questions))*100 + 0.5)
	}
	for _, a := range st.ByAgent {
		w.ByAgent = append(w.ByAgent, wireAgentStat{
			Agent:     a.Agent,
			Hue:       IdentityHue(a.Agent, "", dark),
			Total:     a.Total,
			Questions: a.Questions,
			Answered:  a.Answered,
			Median:    fmtDurMS(a.MedianAnswerMS),
		})
	}
	for _, d := range st.ByDay {
		if d.Count > w.Peak {
			w.Peak = d.Count
		}
		w.ByDay = append(w.ByDay, wireDay{Day: d.Day, Label: dayLabel(d.Day), Count: d.Count})
	}
	w.PerDay = perDay(st.Total, len(st.ByDay))
	return w
}

// perDay is the rate, which reads better than the total on a long window: 40
// interruptions means nothing until you know it was over a month.
func perDay(total, days int) string {
	if total == 0 || days == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f/day", float64(total)/float64(days))
}

// dayLabel turns 2026-07-21 into "Mon 21". The bar chart is scanned, not read,
// so the weekday matters more than the date.
func dayLabel(day string) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return day
	}
	return t.Format("Mon 2")
}

func validWindow(w string) bool {
	return slices.Contains(statsWindows, w)
}

func sinceTime(window string) time.Time {
	switch window {
	case "24h":
		return time.Now().Add(-24 * time.Hour)
	case "30d":
		return time.Now().Add(-30 * 24 * time.Hour)
	case "all":
		return time.UnixMilli(0)
	default: // 7d
		return time.Now().Add(-7 * 24 * time.Hour)
	}
}

func windowLabel(window string) string {
	switch window {
	case "24h":
		return "last 24h"
	case "30d":
		return "last 30 days"
	case "all":
		return "all time"
	default:
		return "last 7 days"
	}
}

// fmtDurMS matches the Gio surface's house style so the two never disagree
// about what "12s" means while both exist.
func fmtDurMS(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	sec := (ms + 500) / 1000
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	return fmt.Sprintf("%dm%02ds", sec/60, sec%60)
}
