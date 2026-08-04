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

// SignalBounds is the oldest sequence still stored and the highest ever issued.
//
// Both are needed to answer one question honestly: has this caller's cursor
// fallen off the trimmed edge? Oldest comes from the table and is zero when it is
// empty; newest comes from sqlite_sequence, which AUTOINCREMENT maintains as a
// high-water mark, so it survives every row being trimmed. Without that second
// number an empty table would read as "nothing has ever happened" to an agent
// holding a cursor from before the trim.
func (s *Store) SignalBounds() (oldest, newest int64, err error) {
	var o sql.NullInt64
	if err := s.db.QueryRow(`SELECT MIN(seq) FROM sync_signals`).Scan(&o); err != nil {
		return 0, 0, fmt.Errorf("signal bounds: %w", err)
	}
	var n sql.NullInt64
	// Missing until the first insert, which is why this tolerates no row.
	if err := s.db.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = 'sync_signals'`).Scan(&n); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, 0, fmt.Errorf("signal high-water: %w", err)
	}
	return o.Int64, n.Int64, nil
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
	res, err := s.db.Exec(`DELETE FROM sync_signals WHERE topic = ? AND seq <= (
		SELECT seq FROM sync_signals WHERE topic = ? ORDER BY seq DESC LIMIT 1 OFFSET ?)`,
		topic, topic, keepPerTopic)
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
		// Everything at or below the keepPerTopic-th newest goes. Expressed as an
		// offset into the topic's own descending order so the boundary is one row
		// rather than a count the delete has to maintain.
		res, err := s.db.Exec(`DELETE FROM sync_signals WHERE topic = ? AND seq <= (
			SELECT seq FROM sync_signals WHERE topic = ? ORDER BY seq DESC LIMIT 1 OFFSET ?)`,
			t, t, keepPerTopic)
		if err != nil {
			return trimmed, fmt.Errorf("trim signals by count: %w", err)
		}
		n, _ := res.RowsAffected()
		trimmed += int(n)
	}
	return trimmed, nil
}
