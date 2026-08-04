package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

// Signals, persisted (FR83 slice 3). The daemon owns delivery; this file owns the
// only thing delivery cannot do by itself, which is answer an agent that was not
// listening when the signal fired.
//
// That is the whole reason signals are in SQLite rather than in a map: the
// walkthrough doctrine, applied to events. A signal is delivered whether or not
// anybody was waiting, and a daemon restart loses nothing inside the retention
// window - so "run the deploy when the tests are green" survives the daemon being
// restarted between the two halves.
//
// The cursor is the rowid, and the migration's comment says why that needs
// AUTOINCREMENT.

// SignalBatchMax caps one catch-up read. A cursor from last week could otherwise
// hand hundreds of signals to a model in one tool result, which is a context bill
// nobody asked for. The reader is told there is more (proto.SyncAwaitResult.More)
// and one more call takes the next batch without parking, so the cap costs a call
// rather than any information.
const SignalBatchMax = 100

// PostSignal stores one signal and returns it as it will be delivered, sequence
// number and timestamp filled in. The caller does the size check: what counts as
// too big is a policy the daemon configures, not a fact about storage.
func (s *Store) PostSignal(topic string, id proto.Identity, data string) (proto.Signal, error) {
	sig := proto.Signal{
		Topic: topic, Agent: id.Agent, Project: id.Project, Key: id.Key,
		AtMS: time.Now().UnixMilli(),
	}
	if data != "" {
		sig.Data = []byte(data)
	}
	res, err := s.db.Exec(`INSERT INTO sync_signals (topic, agent, project, session_key, data, at_ms)
		VALUES (?, ?, ?, ?, ?, ?)`,
		topic, id.Agent, id.Project, id.Key, data, sig.AtMS)
	if err != nil {
		return proto.Signal{}, fmt.Errorf("post signal: %w", err)
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return proto.Signal{}, fmt.Errorf("post signal seq: %w", err)
	}
	sig.Seq = seq
	return sig, nil
}

// SignalsSince is the catch-up read: everything matching, above the cursor, in
// sequence order, capped. The second return says the cap was hit.
//
// Matching is pushed into SQL rather than filtered in Go on purpose. A cursor
// that has been parked for twenty minutes is a scan over every signal every agent
// on the machine has posted since, and the alternative is dragging all of them
// through the process to throw most away. Both halves read the pattern through
// proto.ParseTopic, so the two paths cannot disagree about what a pattern means.
func (s *Store) SignalsSince(after int64, patterns []string, limit int) ([]proto.Signal, bool, error) {
	if limit <= 0 || limit > SignalBatchMax {
		limit = SignalBatchMax
	}
	where, args := topicPredicate(patterns)
	if where == "" {
		// No usable pattern matches nothing, which is the same answer TopicMatches
		// gives. Returning everything here would be a much worse failure.
		return nil, false, nil
	}
	args = append([]any{after}, args...)
	// One row past the limit, so "there is more" is known rather than guessed from
	// a full page.
	args = append(args, limit+1)
	rows, err := s.db.Query(`SELECT seq, topic, agent, project, session_key, data, at_ms
		FROM sync_signals WHERE seq > ? AND (`+where+`)
		ORDER BY seq LIMIT ?`, args...)
	if err != nil {
		return nil, false, fmt.Errorf("read signals: %w", err)
	}
	defer rows.Close()

	var out []proto.Signal
	for rows.Next() {
		var sig proto.Signal
		var data string
		if err := rows.Scan(&sig.Seq, &sig.Topic, &sig.Agent, &sig.Project, &sig.Key, &data, &sig.AtMS); err != nil {
			return nil, false, fmt.Errorf("scan signal: %w", err)
		}
		if data != "" {
			sig.Data = []byte(data)
		}
		out = append(out, sig)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read signals: %w", err)
	}
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
}

// topicPredicate turns patterns into a SQL OR-list. An exact name compares; a
// prefix uses LIKE with the wildcards inside the prefix escaped, because a topic
// containing a literal % or _ must not turn into a wildcard the caller never
// wrote.
func topicPredicate(patterns []string) (string, []any) {
	var parts []string
	var args []any
	for _, p := range patterns {
		value, isPrefix := proto.ParseTopic(p)
		if strings.TrimSpace(p) == "" {
			continue
		}
		if isPrefix {
			parts = append(parts, `topic LIKE ? ESCAPE '\'`)
			args = append(args, likePrefix(value)+"%")
			continue
		}
		parts = append(parts, "topic = ?")
		args = append(args, value)
	}
	return strings.Join(parts, " OR "), args
}

// likePrefix escapes the three characters LIKE treats specially.
func likePrefix(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	return strings.ReplaceAll(s, "_", `\_`)
}

// SignalHighWater is the highest sequence ever issued, which is where "from now
// on" starts.
//
// It comes from sqlite_sequence rather than from MAX(seq), because AUTOINCREMENT
// maintains it as a high-water mark that survives every row being trimmed - and
// without that, an emptied table would read as "nothing has ever happened" to an
// agent holding a cursor from before the trim.
func (s *Store) SignalHighWater() (int64, error) {
	var n sql.NullInt64
	// Missing until the first insert, which is why this tolerates no row.
	if err := s.db.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = 'sync_signals'`).Scan(&n); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("signal high-water: %w", err)
	}
	return n.Int64, nil
}

// SignalOldest is the oldest sequence still stored, or zero when nothing is. Not
// used for the gap check - see SignalGap for why the global minimum cannot answer
// that - but it is how far back history goes, which a read may want to say.
func (s *Store) SignalOldest() (int64, error) {
	var o sql.NullInt64
	if err := s.db.QueryRow(`SELECT MIN(seq) FROM sync_signals`).Scan(&o); err != nil {
		return 0, fmt.Errorf("signal oldest: %w", err)
	}
	return o.Int64, nil
}

// SignalGap answers the only question a cursor really has: was anything on THESE
// topics trimmed after it? It returns the highest sequence retention took from any
// matching topic, or zero when a read from this cursor is complete.
//
// It has to be a recorded fact rather than a deduction. Nothing about what remains
// can tell "trimmed" apart from "never existed", and the deduction that looks
// right - "the cursor is below the oldest surviving signal" - is wrong for a
// per-topic retention policy: one quiet topic's ancient row holds the global
// minimum down while a busy topic's history disappears from under a reader. That
// version shipped, and a live run found it inside the hour.
func (s *Store) SignalGap(after int64, patterns []string) (int64, error) {
	if after <= 0 {
		// "From now on" cannot have missed anything by definition.
		return 0, nil
	}
	where, args := topicPredicate(patterns)
	if where == "" {
		return 0, nil
	}
	args = append(args, after)
	var hw sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(high_water) FROM sync_signal_trim
		WHERE (`+where+`) AND high_water > ?`, args...).Scan(&hw); err != nil {
		return 0, fmt.Errorf("signal gap: %w", err)
	}
	return hw.Int64, nil
}

// trimMarks is the highest sequence per topic that a pending delete is about to
// take, read before it runs.
func (s *Store) trimMarks(cond string, args ...any) (map[string]int64, error) {
	rows, err := s.db.Query(`SELECT topic, MAX(seq) FROM sync_signals
		WHERE `+cond+` GROUP BY topic`, args...)
	if err != nil {
		return nil, fmt.Errorf("read trim marks: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var topic string
		var hw int64
		if err := rows.Scan(&topic, &hw); err != nil {
			return nil, fmt.Errorf("read trim marks: %w", err)
		}
		out[topic] = hw
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read trim marks: %w", err)
	}
	return out, nil
}

// recordTrim remembers how far retention got in one topic. Callers pass the
// highest sequence they deleted; the watermark only ever moves forward, because a
// second trim of an already-trimmed topic must not forget the first.
func (s *Store) recordTrim(topic string, highWater int64) error {
	if topic == "" || highWater <= 0 {
		return nil
	}
	_, err := s.db.Exec(`INSERT INTO sync_signal_trim (topic, high_water) VALUES (?, ?)
		ON CONFLICT(topic) DO UPDATE SET high_water = MAX(high_water, excluded.high_water)`,
		topic, highWater)
	if err != nil {
		return fmt.Errorf("record trim for %s: %w", topic, err)
	}
	return nil
}

// TrimTopic applies the per-topic count to ONE topic: the one just written, which
// is the only topic whose count can have crossed the line.
//
// It exists so a post does not pay for a scan of every other topic's history. The
// whole-table sweep below is the tick's job, where it costs nothing anybody is
// waiting on.
func (s *Store) TrimTopic(topic string, keepPerTopic int) (int, error) {
	if keepPerTopic <= 0 || topic == "" {
		return 0, nil
	}
	// The boundary is read before the delete rather than derived after it: what a
	// reader needs recorded is the highest sequence that went, and once the rows are
	// gone there is nothing left to ask.
	var boundary sql.NullInt64
	err := s.db.QueryRow(`SELECT seq FROM sync_signals WHERE topic = ?
		ORDER BY seq DESC LIMIT 1 OFFSET ?`, topic, keepPerTopic).Scan(&boundary)
	if errors.Is(err, sql.ErrNoRows) || !boundary.Valid {
		return 0, nil // under the cap; nothing to trim
	}
	if err != nil {
		return 0, fmt.Errorf("trim topic %s: %w", topic, err)
	}
	// The watermark is written BEFORE the delete, and the order is the whole
	// safety argument. These are two statements with no transaction around them, so
	// a process that dies between them leaves one of two states: a watermark for
	// signals that are still present (a reader is told about a gap that is not
	// there - it re-reads, costing a call), or signals deleted with nothing
	// recording it (a reader is served a hole and told nothing, which is the
	// failure this whole mechanism exists to prevent). Only one of those is
	// survivable, so the record goes first.
	if err := s.recordTrim(topic, boundary.Int64); err != nil {
		return 0, err
	}
	res, err := s.db.Exec(`DELETE FROM sync_signals WHERE topic = ? AND seq <= ?`,
		topic, boundary.Int64)
	if err != nil {
		return 0, fmt.Errorf("trim topic %s: %w", topic, err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// TrimSignals applies retention: at most keepPerTopic signals per topic, and
// nothing older than maxAge, whichever bites first. Returns how many rows went.
//
// Retention is what makes a gap possible, so the two are one design: this trims,
// SignalBounds reports what is left, and a reader whose cursor is below that is
// told rather than served a batch with holes in it.
func (s *Store) TrimSignals(keepPerTopic int, maxAge time.Duration) (int, error) {
	trimmed := 0
	if maxAge > 0 {
		cutoff := time.Now().Add(-maxAge).UnixMilli()
		// Per topic again, and again before the delete: the age sweep crosses every
		// topic, so each one needs its own watermark or a topic aged away entirely
		// leaves no record that it ever existed - which is the case the wrong version
		// of this check got most wrong.
		marks, err := s.trimMarks(`at_ms < ?`, cutoff)
		if err != nil {
			return trimmed, err
		}
		// Recorded before the delete, for the reason TrimTopic spells out: a crash
		// in between must leave a gap over-reported rather than unreported.
		for topic, hw := range marks {
			if err := s.recordTrim(topic, hw); err != nil {
				return trimmed, err
			}
		}
		res, err := s.db.Exec(`DELETE FROM sync_signals WHERE at_ms < ?`, cutoff)
		if err != nil {
			return trimmed, fmt.Errorf("trim signals by age: %w", err)
		}
		n, _ := res.RowsAffected()
		trimmed += int(n)
	}
	if keepPerTopic <= 0 {
		return trimmed, nil
	}
	// Per topic rather than globally: one chatty topic must not evict another
	// topic's only signal, which is the whole point of keeping a per-topic count.
	rows, err := s.db.Query(`SELECT topic FROM sync_signals GROUP BY topic HAVING COUNT(*) > ?`, keepPerTopic)
	if err != nil {
		return trimmed, fmt.Errorf("trim signals by count: %w", err)
	}
	var topics []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			rows.Close()
			return trimmed, fmt.Errorf("trim signals by count: %w", err)
		}
		topics = append(topics, t)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return trimmed, fmt.Errorf("trim signals by count: %w", err)
	}
	for _, t := range topics {
		// Through TrimTopic rather than a second copy of the same delete, so the
		// watermark is recorded by exactly one piece of code.
		n, err := s.TrimTopic(t, keepPerTopic)
		if err != nil {
			return trimmed, err
		}
		trimmed += n
	}
	return trimmed, nil
}
