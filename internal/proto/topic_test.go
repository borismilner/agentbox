package proto

import "testing"

func TestTopicMatches(t *testing.T) {
	cases := []struct {
		pattern, topic string
		want           bool
		why            string
	}{
		{"tests:green", "tests:green", true, "an exact name matches itself"},
		{"tests:green", "tests:greenish", false, "an exact name is not a prefix"},
		{"done:*", "done:migration-3", true, "a trailing star is a prefix"},
		{"done:*", "done:", true, "the empty remainder still has the prefix"},
		{"done:*", "donee:x", false, "the prefix has to be the prefix"},
		{"*", "anything:at:all", true, "a bare star is every topic, asked for explicitly"},
		{"", "tests:green", false, "an empty pattern matches nothing, so an accidental blank parks rather than firehoses"},
		{"to:ab*cd", "to:abXcd", false, "a star in the middle is a literal, not a wildcard"},
		{"to:ab*cd", "to:ab*cd", true, "and it matches the literal it is"},
		{" tests:green ", "tests:green", true, "surrounding whitespace is not part of a name"},
	}
	for _, c := range cases {
		if got := TopicMatches(c.pattern, c.topic); got != c.want {
			t.Errorf("TopicMatches(%q, %q) = %v, want %v: %s", c.pattern, c.topic, got, c.want, c.why)
		}
	}
}

func TestTopicsMatchIsAnyOf(t *testing.T) {
	list := []string{"tests:green", "done:*"}
	if !TopicsMatch(list, "done:x") {
		t.Fatal("a topic matching the second pattern should match the list")
	}
	if TopicsMatch(list, "build:failed") {
		t.Fatal("a topic matching no pattern should not match the list")
	}
	if TopicsMatch(nil, "tests:green") {
		t.Fatal("an empty list matches nothing")
	}
}

func TestParseTopic(t *testing.T) {
	if v, isPrefix := ParseTopic("done:*"); v != "done:" || !isPrefix {
		t.Fatalf("ParseTopic(done:*) = %q, %v", v, isPrefix)
	}
	if v, isPrefix := ParseTopic("tests:green"); v != "tests:green" || isPrefix {
		t.Fatalf("ParseTopic(tests:green) = %q, %v", v, isPrefix)
	}
}
