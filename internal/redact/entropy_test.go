package redact

import (
	"strings"
	"testing"
)

func TestScrubHighEntropyMixed(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		redact bool
	}{
		{"user-provided secret", "Here's a super-secret string 7F9B2C84A3D16E5F8H9K2M3N4P5Q6R7T8V9W2X3Y4Z5A6B7C8D9E1F2G3H4J5K6 ok?", true},
		{"github classic pat-shape", "token=ghp_abcdefghijklmnopqrstuvwxyz0123456789AB", true},
		{"sha256-ish hex", "hash 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", true},
		{"short hex (ok)", "commit abc1234", false},
		{"long lowercase word", "antidisestablishmentarianism", false},
		{"snake_case identifier", "some_pretty_long_identifier_name", false},
		{"prose untouched", "This is a sentence with no secrets at all.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := scrubHighEntropy(tc.in)
			got := strings.Contains(out, Marker)
			if got != tc.redact {
				t.Fatalf("redact=%v, want %v\nin:  %q\nout: %q", got, tc.redact, tc.in, out)
			}
		})
	}
}

func TestScrubHighEntropyLeavesNormalIdentifiers(t *testing.T) {
	in := "call myReallyLongFunctionName(arg1, arg2) // and a comment with words like configuration"
	if out := scrubHighEntropy(in); out != in {
		t.Fatalf("unexpected change:\nin:  %q\nout: %q", in, out)
	}
}
