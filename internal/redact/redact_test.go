package redact

import (
	"strings"
	"testing"
)

func TestScrubAWS(t *testing.T) {
	in := "deploy with AKIAIOSFODNN7EXAMPLE and secret wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY okay"
	out := Default().Scrub(in)
	if strings.Contains(out, "AKIAIOSFODNN7EXAMPLE") {
		t.Fatalf("access key not redacted: %s", out)
	}
	if strings.Contains(out, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY") {
		t.Fatalf("secret not redacted: %s", out)
	}
	if !strings.Contains(out, Marker) {
		t.Fatalf("no marker: %s", out)
	}
}

func TestScrubNoMatch(t *testing.T) {
	in := "just some normal text without any keys"
	if got := Default().Scrub(in); got != in {
		t.Fatalf("unexpected change: %q", got)
	}
}

func TestScrubEmpty(t *testing.T) {
	if got := Default().Scrub(""); got != "" {
		t.Fatalf("empty: %q", got)
	}
}
