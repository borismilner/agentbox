package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/borismilner/agentbox/internal/proto"
)

// Shared values, persisted (FR83 slice 4). The blackboard: tiny named state with
// compare-and-swap, for coordination that is neither a turn nor an event.
//
// Every operation here is ONE SQL statement, and that is the design rather than
// terseness. A CAS spread over a read and a write would be atomic only because this
// daemon happens to be a single process holding a single connection pool - true
// today, and the kind of true that a dev instance, a second daemon or a future
// migration tool quietly breaks. SQLite can express all three conditions as
// predicates on the write itself, so the atomicity is a property of the statement
// and needs no lock above it to stay correct:
//
//	if_version omitted -> upsert, always applies
//	if_version 0       -> INSERT ... ON CONFLICT DO NOTHING (wins only if absent)
//	if_version N       -> UPDATE ... WHERE version = N (wins only if unmoved)
//
// A refused write then costs one extra read to say what stopped it, which is the
// right trade: the refusal is the common case for a claim, and it has to come back
// with the current value or the caller needs a second call to learn what it lost to.
//
// Nothing here is trimmed. That is the deliberate difference from signals, whose
// whole retention machinery exists because events are history and history can be
// forgotten. A claim is not history: dropping one hands the same chunk to two
// agents, which is the exact failure the gap check was built to prevent.

// SharedKeyMax bounds how many keys the blackboard may hold at once. Not a knob,
// and not retention: crossing it REFUSES a new key rather than evicting an old one.
// Silently evicting coordination state is how two agents come to own one chunk, so
// the cap has to be the kind that fails loudly - and an agent told "delete the
// claims you have finished" can act on that, where an agent whose claim vanished
// cannot.
//
// It is a guard rail against a leaking loop rather than a quota, which is why the
// count is checked without a transaction around it: two concurrent writers at the
// exact boundary can put the table one key over, and one key over a backstop is not
// worth serializing every write to prevent.
const SharedKeyMax = 1000

// SharedListMax caps one prefix read, for the reason SignalBatchMax exists: a
// pattern that matches a thousand keys must not hand a thousand values to a model
// in one tool result. The reader is told there is more.
const SharedListMax = 200

// ErrSharedFull is the refusal when the table is at SharedKeyMax and the write
// would add a key. Updating or deleting an existing key never hits it.
var ErrSharedFull = errors.New("shared value table is full")

// SharedGet reads one key. The second return is whether it exists at all, which a
// caller must be able to tell from "exists and holds an empty value".
func (s *Store) SharedGet(key string) (proto.SharedValue, bool, error) {
	row := s.db.QueryRow(`SELECT key, value, version, owner, owner_agent, owner_pid, updated_ms
		FROM sync_shared WHERE key = ?`, key)
	v, err := scanShared(row)
	if errors.Is(err, sql.ErrNoRows) {
		return proto.SharedValue{}, false, nil
	}
	if err != nil {
		return proto.SharedValue{}, false, fmt.Errorf("read shared value %s: %w", key, err)
	}
	return v, true, nil
}

// SharedList reads a family by prefix, in key order, capped. The second return says
// the cap was hit.
//
// The prefix is what makes the one-key-per-item idiom usable: a ten-chunk claim
// table is one read rather than ten, and a splitter that wants to know what is left
// asks once. It reuses the topic wildcard's escaping so a key containing a literal
// % or _ cannot turn into a wildcard nobody wrote.
func (s *Store) SharedList(prefix string, limit int) ([]proto.SharedValue, bool, error) {
	if limit <= 0 || limit > SharedListMax {
		limit = SharedListMax
	}
	// One row past the limit, so "there is more" is known rather than guessed.
	rows, err := s.db.Query(`SELECT key, value, version, owner, owner_agent, owner_pid, updated_ms
		FROM sync_shared WHERE key LIKE ? ESCAPE '\' ORDER BY key LIMIT ?`,
		likePrefix(prefix)+"%", limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("list shared values: %w", err)
	}
	defer rows.Close()

	var out []proto.SharedValue
	for rows.Next() {
		v, err := scanShared(rows)
		if err != nil {
			return nil, false, fmt.Errorf("scan shared value: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("list shared values: %w", err)
	}
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
}

// SharedSet writes one key under the CAS rule ifVersion states, and answers with
// the row that resulted - or, when the write was refused, the row that refused it.
//
// The second return is whether the write landed. False is not an error: for a claim
// it is the normal outcome, and it is exactly the answer a first-writer-wins fan-out
// is asking for.
func (s *Store) SharedSet(key, value string, ifVersion *int64, owner, ownerAgent string, ownerPID int) (proto.SharedValue, bool, error) {
	now := time.Now().UnixMilli()

	// The cap is checked in front of the two statements that can ADD a key, and only
	// costs a primary-key lookup for a key that is already there. An UPDATE cannot
	// grow the table, so the third branch never pays for this at all.
	if ifVersion == nil || *ifVersion == 0 {
		if full, err := s.sharedFullFor(key); err != nil {
			return proto.SharedValue{}, false, err
		} else if full {
			return proto.SharedValue{}, false, ErrSharedFull
		}
	}

	switch {
	case ifVersion == nil:
		// Unconditional. Version still rises, so a CAS writer racing an unconditional
		// one is refused rather than silently overwritten.
		row := s.db.QueryRow(`INSERT INTO sync_shared (key, value, version, owner, owner_agent, owner_pid, updated_ms)
			VALUES (?, ?, 1, ?, ?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value,
				version = sync_shared.version + 1, owner = excluded.owner,
				owner_agent = excluded.owner_agent, owner_pid = excluded.owner_pid,
				updated_ms = excluded.updated_ms
			RETURNING key, value, version, owner, owner_agent, owner_pid, updated_ms`,
			key, value, owner, ownerAgent, ownerPID, now)
		v, err := scanShared(row)
		if err != nil {
			return proto.SharedValue{}, false, fmt.Errorf("write shared value %s: %w", key, err)
		}
		return v, true, nil

	case *ifVersion == 0:
		// Claim from empty: wins only if nobody has this key. DO NOTHING makes losing
		// silent at the SQL level, and RETURNING is what turns that silence into an
		// answer - no row means somebody else got here first.
		row := s.db.QueryRow(`INSERT INTO sync_shared (key, value, version, owner, owner_agent, owner_pid, updated_ms)
			VALUES (?, ?, 1, ?, ?, ?, ?) ON CONFLICT(key) DO NOTHING
			RETURNING key, value, version, owner, owner_agent, owner_pid, updated_ms`,
			key, value, owner, ownerAgent, ownerPID, now)
		v, err := scanShared(row)
		if errors.Is(err, sql.ErrNoRows) {
			return s.sharedRefusal(key)
		}
		if err != nil {
			return proto.SharedValue{}, false, fmt.Errorf("claim shared value %s: %w", key, err)
		}
		return v, true, nil

	default:
		// Must be exactly at this version. An UPDATE rather than an upsert, because
		// "only if it is still at 3" is false for a key that does not exist - and an
		// insert here would create version 1 for a caller who asked about version 3.
		row := s.db.QueryRow(`UPDATE sync_shared
			SET value = ?, version = version + 1, owner = ?, owner_agent = ?,
				owner_pid = ?, updated_ms = ?
			WHERE key = ? AND version = ?
			RETURNING key, value, version, owner, owner_agent, owner_pid, updated_ms`,
			value, owner, ownerAgent, ownerPID, now, key, *ifVersion)
		v, err := scanShared(row)
		if errors.Is(err, sql.ErrNoRows) {
			return s.sharedRefusal(key)
		}
		if err != nil {
			return proto.SharedValue{}, false, fmt.Errorf("write shared value %s: %w", key, err)
		}
		return v, true, nil
	}
}

// SharedDelete removes one key, optionally only at a version it has not moved from.
// The returned value is the row as it was just before it went, so a caller can say
// what it dropped; on a refusal it is the row that refused.
func (s *Store) SharedDelete(key string, ifVersion *int64) (proto.SharedValue, bool, error) {
	query := `DELETE FROM sync_shared WHERE key = ?`
	args := []any{key}
	if ifVersion != nil {
		query += ` AND version = ?`
		args = append(args, *ifVersion)
	}
	row := s.db.QueryRow(query+` RETURNING key, value, version, owner, owner_agent, owner_pid, updated_ms`, args...)
	v, err := scanShared(row)
	if errors.Is(err, sql.ErrNoRows) {
		return s.sharedRefusal(key)
	}
	if err != nil {
		return proto.SharedValue{}, false, fmt.Errorf("delete shared value %s: %w", key, err)
	}
	return v, true, nil
}

// SharedCount is how many keys the blackboard holds, for the surface and for the
// cap's own error message.
func (s *Store) SharedCount() (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sync_shared`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count shared values: %w", err)
	}
	return n, nil
}

// sharedRefusal reads what is there now, which is what a refused write owes its
// caller: a CAS loop that has to make a second call to find out what it lost to
// costs a model turn per collision, and the whole point of returning the current
// version is that the retry is immediate.
func (s *Store) sharedRefusal(key string) (proto.SharedValue, bool, error) {
	v, found, err := s.SharedGet(key)
	if err != nil {
		return proto.SharedValue{}, false, err
	}
	if !found {
		// The key is not there and the write still failed, which means the caller
		// named a version for something that does not exist. Version 0 says exactly
		// that, and it is also the if_version the caller should retry with.
		return proto.SharedValue{Key: key}, false, nil
	}
	return v, false, nil
}

// sharedFullFor answers whether the cap would stop a write that adds this key. An
// update to a key already present is never blocked, however full the table is:
// refusing those would strand a claim its owner is trying to finish.
func (s *Store) sharedFullFor(key string) (bool, error) {
	if _, found, err := s.SharedGet(key); err != nil {
		return false, err
	} else if found {
		return false, nil
	}
	n, err := s.SharedCount()
	if err != nil {
		return false, err
	}
	return n >= SharedKeyMax, nil
}

// scanRow is what QueryRow and Rows have in common, so one scanner serves both.
type scanRow interface{ Scan(dest ...any) error }

func scanShared(r scanRow) (proto.SharedValue, error) {
	var v proto.SharedValue
	var value string
	if err := r.Scan(&v.Key, &value, &v.Version, &v.Owner, &v.OwnerAgent, &v.OwnerPID, &v.UpdatedMS); err != nil {
		return proto.SharedValue{}, err
	}
	if value != "" {
		v.Value = []byte(value)
	}
	return v, nil
}

// SharedKeyValid rejects a key that would make the prefix idiom ambiguous. A
// wildcard is a thing you READ with, never a thing you name: storing a key with a *
// in it would create an entry that no exact get can reach and that every prefix get
// matches by accident - the same rule post_signal enforces on topics.
func SharedKeyValid(key string) bool {
	return strings.TrimSpace(key) != "" && !strings.Contains(key, proto.TopicPrefix)
}
