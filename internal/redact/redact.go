// Package redact best-effort scrubs secrets from text before serving.
//
// It uses Trufflehog's default detectors (~800 vendor-specific rules) via the
// Aho-Corasick core to find candidate secrets, runs each detector's FromData
// against the input (with verification disabled), and replaces each reported
// raw secret with a fixed marker. We never call the network; detector
// verification is disabled.
//
// This is best-effort: detectors can miss novel formats, and may emit false
// positives that we redact anyway. The goal is to avoid accidentally leaking
// API keys, not to be a complete DLP.
package redact

import (
	"context"
	"strings"
	"sync"

	ahocorasick "github.com/trufflesecurity/trufflehog/v3/pkg/engine/ahocorasick"
	"github.com/trufflesecurity/trufflehog/v3/pkg/engine/defaults"
)

// Marker is what we substitute in place of detected secrets.
const Marker = "〈redacted〉"

// Redactor scans text for secrets and returns a scrubbed copy.
type Redactor struct {
	core *ahocorasick.Core
}

var (
	defaultOnce sync.Once
	defaultR    *Redactor
)

// Default returns a process-wide Redactor backed by Trufflehog's default
// detectors. Construction is moderately expensive (builds the Aho-Corasick
// trie over hundreds of keywords); we memoize it.
func Default() *Redactor {
	defaultOnce.Do(func() {
		dets := defaults.DefaultDetectors()
		defaultR = &Redactor{core: ahocorasick.NewAhoCorasickCore(dets)}
	})
	return defaultR
}

// Scrub returns s with any detected secrets replaced by Marker. If no secrets
// are found, the original string is returned unchanged.
func (r *Redactor) Scrub(s string) string {
	if r == nil || s == "" {
		return s
	}
	data := []byte(s)
	matches := r.core.FindDetectorMatches(data)
	if len(matches) == 0 {
		return s
	}
	// Use a non-cancellable context: detectors that hit the network when
	// verify=true would honor cancellation, but we never set verify=true.
	ctx := context.Background()

	// Collect unique raw-secret literals across all detectors, then do a
	// single pass of replacements. Sort by length desc so we don't replace
	// substrings of a longer match first.
	seen := map[string]struct{}{}
	var secrets []string
	for _, m := range matches {
		res, err := m.Detector.FromData(ctx, false, data)
		if err != nil {
			continue
		}
		for _, r := range res {
			add := func(b []byte) {
				if len(b) < 4 {
					return
				}
				str := string(b)
				if _, ok := seen[str]; ok {
					return
				}
				seen[str] = struct{}{}
				secrets = append(secrets, str)
			}
			add(r.Raw)
			// RawV2 is often "id:secret". Redact the whole literal AND
			// each colon-delimited component, so that if the components
			// appear separately we still scrub them.
			add(r.RawV2)
			for _, part := range strings.Split(string(r.RawV2), ":") {
				add([]byte(part))
			}
		}
	}
	if len(secrets) == 0 {
		return s
	}
	// Longest first to avoid partially overlapping replacements.
	sortByLenDesc(secrets)
	out := s
	for _, sec := range secrets {
		out = strings.ReplaceAll(out, sec, Marker)
	}
	return out
}

func sortByLenDesc(ss []string) {
	// Insertion sort: list is short (handful of matches).
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && len(ss[j]) > len(ss[j-1]); j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}
