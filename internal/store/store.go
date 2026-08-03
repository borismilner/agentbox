// Package store owns the agentbox application database: a single SQLite file,
// auto-created on first open, with embedded forward-only migrations applied
// before any other work (ADR-0005, NFR15).
package store

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/borismilner/agentbox/internal/proto"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Item states. Transitions are append-only history; items hold the current
// state.
const (
	StatePending   = "pending"
	StateAnswered  = "answered"
	StateExpired   = "expired"
	StateCancelled = "cancelled"
	StateDismissed = "dismissed"
)

var ErrNotFound = errors.New("item not found")

// ErrSchemaTooNew means the database was written by a newer agentbox; refusing
// to run protects the data (ADR-0005).
var ErrSchemaTooNew = errors.New("database schema is newer than this binary")

type Store struct {
	db *sql.DB
}

// StoredItem is an Item plus its persistence state.
type StoredItem struct {
	proto.Item
	State           string
	Answer          string
	Reply           string
	Values          map[string]string
	MissedWhileAway bool // FR44: a toast that auto-expired while the user was idle
	CreatedAt       time.Time
	ResolvedAt      time.Time
}

// Outcome is how a pending item ended; at most one field is set. Vetoed is
// in-memory only (it rides into the delivered Result, FR22); the answered
// vs expired state already records the veto outcome for the audit trail.
type Outcome struct {
	Answer string
	Reply  string
	Values map[string]string
	Vetoed bool
	// Approved is the diff-review verdict (FR33). In-memory only, like Vetoed:
	// it rides into the delivered Result; the answer transition (approved vs
	// rejected) and the comment (in Reply) are the persisted audit trail.
	Approved bool
	// Secret is the masked value (FR23). In-memory only: it rides into the
	// delivered Result and is never persisted - history records that a
	// secret was provided (the answered transition), never its value.
	Secret string
	// MissedAway flags a toast auto-dismissed while the user was idle (FR44).
	// Unlike the fields above this one is persisted: it is a history marker the
	// return-from-idle review reads, not a value handed back to the caller.
	MissedAway bool
}

// Open creates the directory and database if missing and applies pending
// migrations. It is safe to call on every start.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}
	dsn := "file:" + url.PathEscape(path) +
		"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	// The daemon is the only writer; a single connection sidesteps
	// SQLITE_BUSY between concurrent goroutines.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, e := range entries {
		name := e.Name()
		num, rest, ok := strings.Cut(name, "_")
		if !ok || !strings.HasSuffix(rest, ".sql") {
			return nil, fmt.Errorf("migration %q does not match NNNN_name.sql", name)
		}
		v, err := strconv.Atoi(num)
		if err != nil {
			return nil, fmt.Errorf("migration %q has a non-numeric version", name)
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{version: v, name: name, sql: string(body)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	for i, m := range ms {
		if m.version != i+1 {
			return nil, fmt.Errorf("migration versions must be sequential from 1; found %q at position %d", m.name, i+1)
		}
	}
	return ms, nil
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	current, err := s.SchemaVersion()
	if err != nil {
		return err
	}
	ms, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}
	if current > len(ms) {
		return fmt.Errorf("%w: database at version %d, binary knows up to %d",
			ErrSchemaTooNew, current, len(ms))
	}
	for _, m := range ms[current:] {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s failed (schema left at version %d): %w", m.name, m.version-1, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
			m.version, m.name, time.Now().UnixMilli()); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s: %w", m.name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", m.name, err)
		}
	}
	return nil
}

func (s *Store) SchemaVersion() (int, error) {
	var v sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return int(v.Int64), nil
}

// CreateItem persists a new item in state pending with its creation
// transition. The item must already carry its daemon-assigned ID.
func (s *Store) CreateItem(it *proto.Item) error {
	if it.ID == "" {
		return errors.New("item has no ID")
	}
	opts, err := json.Marshal(it.Options)
	if err != nil {
		return fmt.Errorf("marshal options: %w", err)
	}
	fields, err := json.Marshal(it.Fields)
	if err != nil {
		return fmt.Errorf("marshal fields: %w", err)
	}
	actions, err := json.Marshal(it.Actions)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO items
		(id, kind, level, title, body, options, fields, actions, cwd, timeout_s, dflt, agent, project, session, state, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.ID, string(it.Kind), string(it.EffectiveLevel()), it.Title, it.Body, string(opts), string(fields),
		string(actions), it.Cwd, it.TimeoutS, it.Default, it.Identity.Agent, it.Identity.Project, it.Identity.Session,
		StatePending, now); err != nil {
		return fmt.Errorf("insert item %s: %w", it.ID, err)
	}
	if _, err := tx.Exec(`INSERT INTO transitions (item_id, from_state, to_state, at) VALUES (?, '', ?, ?)`,
		it.ID, StatePending, now); err != nil {
		return fmt.Errorf("record creation of %s: %w", it.ID, err)
	}
	return tx.Commit()
}

// Resolve moves a pending item to a terminal state, recording the
// transition atomically. It fails if the item is not pending, which is what
// makes answer delivery exactly-once (FR6).
func (s *Store) Resolve(id, toState string, out Outcome) error {
	var valuesJSON any
	if len(out.Values) > 0 {
		raw, err := json.Marshal(out.Values)
		if err != nil {
			return fmt.Errorf("marshal form values: %w", err)
		}
		valuesJSON = string(raw)
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`UPDATE items SET state = ?, answer = ?, reply = ?, form_values = ?, missed_while_away = ?, resolved_at = ?
		WHERE id = ? AND state = ?`,
		toState, nullable(out.Answer), nullable(out.Reply), valuesJSON, boolInt(out.MissedAway), now, id, StatePending)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("resolve %s to %s: %w", id, toState, ErrNotFound)
	}
	if _, err := tx.Exec(`INSERT INTO transitions (item_id, from_state, to_state, at) VALUES (?, ?, ?, ?)`,
		id, StatePending, toState, now); err != nil {
		return fmt.Errorf("record transition of %s: %w", id, err)
	}
	return tx.Commit()
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Pending returns unresolved items oldest first, for re-presentation after
// a restart (NFR7).
func (s *Store) Pending() ([]StoredItem, error) {
	return s.query(`SELECT id, kind, level, title, body, options, fields, actions, cwd, timeout_s, dflt,
		agent, project, session, state, answer, reply, form_values, missed_while_away, created_at, resolved_at
		FROM items WHERE state = ? ORDER BY created_at ASC`, StatePending)
}

// Recent returns the newest items first, pending or not.
func (s *Store) Recent(limit int) ([]StoredItem, error) {
	return s.query(`SELECT id, kind, level, title, body, options, fields, actions, cwd, timeout_s, dflt,
		agent, project, session, state, answer, reply, form_values, missed_while_away, created_at, resolved_at
		FROM items ORDER BY created_at DESC LIMIT ?`, limit)
}

func (s *Store) query(q string, args ...any) ([]StoredItem, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredItem
	for rows.Next() {
		var it StoredItem
		var opts, fields, actions string
		var answer, reply, values sql.NullString
		var created int64
		var resolved sql.NullInt64
		var missed int
		var kind, level string
		if err := rows.Scan(&it.ID, &kind, &level, &it.Title, &it.Body, &opts, &fields, &actions, &it.Cwd, &it.TimeoutS,
			&it.Default, &it.Identity.Agent, &it.Identity.Project, &it.Identity.Session,
			&it.State, &answer, &reply, &values, &missed, &created, &resolved); err != nil {
			return nil, err
		}
		it.MissedWhileAway = missed != 0
		it.Kind = proto.Kind(kind)
		it.Level = proto.Level(level)
		if err := json.Unmarshal([]byte(opts), &it.Options); err != nil {
			return nil, fmt.Errorf("item %s has corrupt options: %w", it.ID, err)
		}
		if err := json.Unmarshal([]byte(fields), &it.Fields); err != nil {
			return nil, fmt.Errorf("item %s has corrupt fields: %w", it.ID, err)
		}
		if err := json.Unmarshal([]byte(actions), &it.Actions); err != nil {
			return nil, fmt.Errorf("item %s has corrupt actions: %w", it.ID, err)
		}
		it.Answer = answer.String
		it.Reply = reply.String
		if values.Valid {
			if err := json.Unmarshal([]byte(values.String), &it.Values); err != nil {
				return nil, fmt.Errorf("item %s has corrupt form values: %w", it.ID, err)
			}
		}
		it.CreatedAt = time.UnixMilli(created)
		if resolved.Valid {
			it.ResolvedAt = time.UnixMilli(resolved.Int64)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Prune evicts resolved items older than maxAge whose level ranks below
// keepAtOrAbove, together with their transitions (FR10 retention). Pending
// items are never evicted regardless of age. Returns the number of items
// removed.
func (s *Store) Prune(maxAge time.Duration, keepAtOrAbove proto.Level) (int, error) {
	var evictable []string
	for _, l := range []proto.Level{proto.LevelInfo, proto.LevelSuccess, proto.LevelWarning, proto.LevelError, proto.LevelUrgent} {
		if l.Rank() < keepAtOrAbove.Rank() {
			evictable = append(evictable, string(l))
		}
	}
	if len(evictable) == 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-maxAge).UnixMilli()
	placeholders := strings.Repeat(",?", len(evictable))[1:]
	args := []any{cutoff}
	for _, l := range evictable {
		args = append(args, l)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM transitions WHERE item_id IN (
			SELECT id FROM items WHERE state != 'pending' AND created_at < ? AND level IN (`+placeholders+`))`,
		args...); err != nil {
		return 0, fmt.Errorf("prune transitions: %w", err)
	}
	res, err := tx.Exec(`DELETE FROM items
			WHERE state != 'pending' AND created_at < ? AND level IN (`+placeholders+`)`,
		args...)
	if err != nil {
		return 0, fmt.Errorf("prune items: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), tx.Commit()
}

// Stats aggregates items created at or after `since` into interruption
// insights (FR35): totals and median time-to-answer overall, per agent and
// per calendar day. A zero `since` covers the whole history.
func (s *Store) Stats(since time.Time) (proto.Stats, error) {
	rows, err := s.db.Query(`SELECT kind, agent, state, created_at, resolved_at
		FROM items WHERE created_at >= ? ORDER BY created_at ASC`, since.UnixMilli())
	if err != nil {
		return proto.Stats{}, fmt.Errorf("query stats: %w", err)
	}
	defer rows.Close()

	type agg struct {
		total, questions, answered int
		answerMS                   []int64
	}
	overall := &agg{}
	byAgent := map[string]*agg{}
	var agentOrder []string
	byDay := map[string]int{}
	var dayOrder []string

	for rows.Next() {
		var kind, agent, state string
		var created int64
		var resolved sql.NullInt64
		if err := rows.Scan(&kind, &agent, &state, &created, &resolved); err != nil {
			return proto.Stats{}, err
		}
		a := byAgent[agent]
		if a == nil {
			a = &agg{}
			byAgent[agent] = a
			agentOrder = append(agentOrder, agent)
		}
		blocking := proto.Kind(kind) != proto.KindNotify
		a.total++
		overall.total++
		if blocking {
			a.questions++
			overall.questions++
		}
		if state == StateAnswered {
			a.answered++
			overall.answered++
			if resolved.Valid {
				ms := max(resolved.Int64-created, 0)
				a.answerMS = append(a.answerMS, ms)
				overall.answerMS = append(overall.answerMS, ms)
			}
		}
		day := time.UnixMilli(created).Format("2006-01-02")
		if _, ok := byDay[day]; !ok {
			dayOrder = append(dayOrder, day)
		}
		byDay[day]++
	}
	if err := rows.Err(); err != nil {
		return proto.Stats{}, err
	}

	st := proto.Stats{
		SinceMS:        since.UnixMilli(),
		Total:          overall.total,
		Questions:      overall.questions,
		Answered:       overall.answered,
		MedianAnswerMS: medianMS(overall.answerMS),
	}
	for _, name := range agentOrder {
		a := byAgent[name]
		st.ByAgent = append(st.ByAgent, proto.AgentStat{
			Agent: name, Total: a.total, Questions: a.questions,
			Answered: a.answered, MedianAnswerMS: medianMS(a.answerMS),
		})
	}
	sort.SliceStable(st.ByAgent, func(i, j int) bool { return st.ByAgent[i].Total > st.ByAgent[j].Total })
	for _, day := range dayOrder {
		st.ByDay = append(st.ByDay, proto.DayCount{Day: day, Count: byDay[day]})
	}
	return st, nil
}

func medianMS(v []int64) int64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]int64(nil), v...)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// Transitions returns the audit trail for one item, oldest first.
func (s *Store) Transitions(itemID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT from_state, to_state FROM transitions WHERE item_id = ? ORDER BY seq`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var from, to string
		if err := rows.Scan(&from, &to); err != nil {
			return nil, err
		}
		if from == "" {
			from = "(new)"
		}
		out = append(out, from+" -> "+to)
	}
	return out, rows.Err()
}
