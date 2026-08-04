package proto

import "strings"

// Topic matching for signals (FR83 slice 3).
//
// A pattern is an exact topic name or a plain string prefix ending in `*`:
// `tests:green`, `done:*`, `shared:claims/*`. Deliberately not globbing and
// deliberately not regex - a topic is a name in the kind:scope idiom, and the
// two things an agent actually wants are "this one" and "this family".
//
// The rule lives here, in one place, because it is applied twice: the daemon
// matches a live post against every parked waiter, and the store turns the same
// patterns into a SQL predicate for a catch-up read. Those two must agree about
// what a pattern MEANS or a signal delivered live would be missing from the batch
// a restarted agent reads back, which is the silent divergence FR61's rule exists
// to prevent.

// TopicPrefix is the wildcard suffix. One character, at the end, or nowhere.
const TopicPrefix = "*"

// ParseTopic reads a pattern into what to compare against and how. A pattern
// with a `*` anywhere but the end is treated as an exact name including the
// star: a topic is a name, so a caller that wrote one in the middle meant a
// literal, and silently promoting it to a wildcard would deliver signals it
// never asked for.
func ParseTopic(pattern string) (value string, isPrefix bool) {
	p := strings.TrimSpace(pattern)
	if before, ok := strings.CutSuffix(p, TopicPrefix); ok {
		return before, true
	}
	return p, false
}

// TopicMatches answers whether one pattern covers one topic. An empty pattern
// matches nothing: "wait on everything" has to be asked for as `*`, so that a
// caller whose topic list came out empty by accident parks forever on nothing
// rather than being woken by every signal on the machine.
func TopicMatches(pattern, topic string) bool {
	value, isPrefix := ParseTopic(pattern)
	if pattern == "" {
		return false
	}
	if isPrefix {
		return strings.HasPrefix(topic, value)
	}
	return topic == value
}

// SharedTopicPrefix is the family every shared-value change is posted under, so
// "wait until anybody claims a chunk" is await_signal(["shared:claims/*"]).
const SharedTopicPrefix = "shared:"

// SharedTopic names the topic one key's changes arrive on. It is a function rather
// than a string concatenation at each call site because three places have to agree
// on it - the daemon that posts, the manual that teaches it, and any agent that
// composes a pattern from a key it already has.
func SharedTopic(key string) string { return SharedTopicPrefix + key }

// TopicsMatch is any-of, which is what a waiter's topic list means.
func TopicsMatch(patterns []string, topic string) bool {
	for _, p := range patterns {
		if TopicMatches(p, topic) {
			return true
		}
	}
	return false
}
