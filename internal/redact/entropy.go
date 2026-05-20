package redact

import (
	"math"
	"regexp"
)

// Heuristic high-entropy token redactor. This complements the Trufflehog
// vendor-specific detectors by catching opaque tokens whose surrounding
// context doesn't include a recognized keyword ("password=", "sk-", etc.).
//
// We extract candidate tokens via a regex, compute Shannon entropy in bits
// per character, and redact when entropy exceeds a threshold. Thresholds
// borrow from gitleaks / detect-secrets folklore:
//   - Base64-ish alphabet (A-Za-z0-9+/=): ~4.5 bits/char
//   - Hex alphabet: ~3.5 bits/char
//
// The token regex is conservative: at least 20 characters, made of
// [A-Za-z0-9_+/=.-], with at least one digit AND at least one letter to avoid
// chewing through long identifiers ("foooooooooooooooooooo") or pure numbers.
//
// We deliberately do NOT scan inside code fences/backticks beyond their text
// content — the caller already passes raw text; markdown rendering happens
// after redaction. So a code block of high-entropy hash will be redacted,
// which is what we want for shared conversations.

var tokenRE = regexp.MustCompile(`[A-Za-z0-9_+/=.\-]{20,}`)

// minHexEntropy is the bits/char threshold for hex-like tokens (32+ hex
// chars get redacted only if entropy is high enough; pure hex tops out at 4
// bits/char). minMixedEntropy applies to anything that includes characters
// beyond hex (uppercase, base64 padding, etc.).
const (
	minMixedEntropy = 3.5
	minHexEntropy   = 3.0
	minTokenLen     = 20
)

var hexRE = regexp.MustCompile(`^[0-9a-fA-F]+$`)

func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	var freq [256]int
	for i := 0; i < len(s); i++ {
		freq[s[i]]++
	}
	var h float64
	n := float64(len(s))
	for _, c := range freq {
		if c == 0 {
			continue
		}
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}

func hasLetterAndDigit(s string) bool {
	var hasL, hasD bool
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			hasD = true
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			hasL = true
		}
		if hasL && hasD {
			return true
		}
	}
	return false
}

// scrubHighEntropy returns s with high-entropy tokens replaced by Marker.
func scrubHighEntropy(s string) string {
	return tokenRE.ReplaceAllStringFunc(s, func(tok string) string {
		if len(tok) < minTokenLen || !hasLetterAndDigit(tok) {
			return tok
		}
		thresh := minMixedEntropy
		if hexRE.MatchString(tok) {
			thresh = minHexEntropy
		}
		if shannonEntropy(tok) >= thresh {
			return Marker
		}
		return tok
	})
}
