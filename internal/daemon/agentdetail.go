package daemon

import (
	"sort"
	"strings"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

// What opening a row on the Agents board asks for (FR83). The three blocks the
// surface has always rendered - what this session has been doing, what it has
// said and heard, what it raised - were filled by the demo fixture and by nothing
// else, so a real row opened onto the meta list and its locks and stopped there.
//
// It is assembled here rather than in the roster because the answer spans three
// owners that must not learn about each other: the roster holds the activity
// ring, the signal hub holds who HEARD what (the store cannot - see the note on
// signals.received), and the store holds what was posted and what was raised.
//
// Per opened row, never in the roster push. The roster goes out on every change
// and once a second while anything moves; twenty ticks and twenty signals per
// agent in each of those would be paid for a hundred times over by rows nobody
// opened.

// detailSignals bounds each half of the signal block before the merge, so one
// chatty direction cannot crowd out the other.
const detailSignals = 20

// detailItems bounds the items block on a row.
const detailItems = 12

// AgentDetail answers for one session key. An unknown key comes back with
// Found false rather than empty: a session that ended and a session that has done
// nothing are the same picture otherwise, and only one of them is worth saying.
func (d *Daemon) AgentDetail(key string) proto.SyncAgentDetail {
	key = strings.TrimSpace(key)
	out := proto.SyncAgentDetail{Key: key}
	if key == "" {
		return out
	}
	now := time.Now()

	history, ok := d.roster.historyOf(key)
	out.Found = ok
	for _, t := range history {
		out.Timeline = append(out.Timeline, proto.SyncTick{Line: t.line, SinceMS: sinceMS(now, t.at)})
	}

	out.Signals = d.signalTicks(key, now)
	return out
}

// signalTicks merges the two halves of a session's signal life into one list,
// oldest first. Sorted by age rather than concatenated: posting and hearing are
// a conversation, and reading them in two blocks makes the reader do the
// interleaving in their head.
func (d *Daemon) signalTicks(key string, now time.Time) []proto.SyncSignalTick {
	var ticks []proto.SyncSignalTick
	if st := d.st; st != nil {
		posted, err := st.SignalsPostedBy(key, detailSignals)
		if err != nil {
			d.log.Warn("sync.detail_signals_failed", "component", "daemon", "key", key, "err", err.Error())
		}
		for _, sig := range posted {
			ticks = append(ticks, proto.SyncSignalTick{
				Topic: sig.Topic, Dir: "posted",
				SinceMS: sinceMS(now, time.UnixMilli(sig.AtMS)),
				Data:    glimpse(string(sig.Data)),
			})
		}
	}
	for _, r := range d.signals.receivedBy(key) {
		ticks = append(ticks, proto.SyncSignalTick{
			Topic: r.topic, Dir: "received", SinceMS: sinceMS(now, r.at), Data: r.data,
		})
	}
	// Oldest first: the largest age comes first, which is the order every other
	// block on this surface reads in.
	sort.SliceStable(ticks, func(i, j int) bool { return ticks[i].SinceMS > ticks[j].SinceMS })
	return ticks
}

// sinceMS is how long ago something was, never negative. A clock that moved
// backwards would otherwise render as a thing that has not happened yet.
func sinceMS(now, then time.Time) int64 {
	if then.IsZero() || !now.After(then) {
		return 0
	}
	return now.Sub(then).Milliseconds()
}

// historyOf is the roster's half: the lines this session has moved past, oldest
// first, and whether the session is on the board at all.
func (r *roster) historyOf(key string) ([]activityTick, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row := r.rows[key]
	if row == nil {
		return nil, false
	}
	return append([]activityTick(nil), row.history...), true
}
