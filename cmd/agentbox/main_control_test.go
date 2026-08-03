package main

import (
	"reflect"
	"testing"
)

func TestPartitionControlFlagsAcceptsAFlagAfterTheReason(t *testing.T) {
	// The bug this exists for: Go's flag package stops parsing at the first
	// non-flag argument, so `control request "reason" --window 12` kept the 20s
	// default AND put "--window 12" into the reason the human reads. An agent
	// writes the reason first because that is the natural order.
	for _, tc := range []struct {
		name  string
		in    []string
		flags []string
		words []string
	}{
		{
			name:  "flag after the reason",
			in:    []string{"clicking the board", "--window", "12"},
			flags: []string{"--window", "12"},
			words: []string{"clicking the board"},
		},
		{
			name:  "flag before the reason, the shape Go already accepted",
			in:    []string{"--window", "12", "clicking the board"},
			flags: []string{"--window", "12"},
			words: []string{"clicking the board"},
		},
		{
			name:  "equals form",
			in:    []string{"clicking", "--window=12", "the board"},
			flags: []string{"--window=12"},
			words: []string{"clicking", "the board"},
		},
		{
			name:  "no flag at all",
			in:    []string{"clicking", "the", "board"},
			flags: nil,
			words: []string{"clicking", "the", "board"},
		},
		{
			name:  "a flag with its value missing is left for flag.Parse to report",
			in:    []string{"--window"},
			flags: []string{"--window"},
			words: nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags, words := partitionControlFlags(tc.in)
			if !reflect.DeepEqual(flags, tc.flags) {
				t.Errorf("flags = %q, want %q", flags, tc.flags)
			}
			if !reflect.DeepEqual(words, tc.words) {
				t.Errorf("words = %q, want %q", words, tc.words)
			}
		})
	}
}
