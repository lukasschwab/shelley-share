// Package share computes and verifies unguessable share tokens for Shelley
// conversation IDs.
//
// Tokens are of the form "<conversation_id>.<base64url(HMAC(secret,id)[:12])>".
// The HMAC tag is 96 bits of unforgeable proof that the issuer (the VM owner,
// who can read the secret) intended to share this conversation.
//
// The secret is derived from an existing machine-local secret — by default the
// user's SSH private key at ~/.ssh/id_ed25519 — via HMAC-SHA256 under a
// version-tagged domain separation label. We never write the secret to disk
// ourselves, and we only ever store its HMAC output in memory.
package share

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	tagBytes  = 12 // 96-bit truncated HMAC
	domainSep = "shelley-share/v1/share-token"

	// SecretFallbackName is the file used when no SSH key is available.
	SecretFallbackName = "secret"
)

// DefaultSeedPaths is the search order for an existing machine-local secret to
// derive the HMAC key from.
var DefaultSeedPaths = []string{
	"~/.ssh/id_ed25519",
	"~/.ssh/id_rsa",
	"~/.ssh/id_ecdsa",
}

// LoadSecret returns the HMAC key for share tokens, deriving it from the first
// readable file in seedPaths. If none are readable, it falls back to a random
// secret persisted under stateDir/secret.
//
// Pass nil/empty seedPaths to use DefaultSeedPaths.
func LoadSecret(stateDir string, seedPaths []string) ([]byte, string, error) {
	if len(seedPaths) == 0 {
		seedPaths = DefaultSeedPaths
	}
	for _, p := range seedPaths {
		exp, err := expandHome(p)
		if err != nil {
			continue
		}
		b, err := os.ReadFile(exp)
		if err != nil {
			continue
		}
		if len(b) < 32 {
			continue
		}
		return derive(b), exp, nil
	}
	// Fallback: generate-and-persist.
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, "", err
	}
	p := filepath.Join(stateDir, SecretFallbackName)
	b, err := os.ReadFile(p)
	if err == nil && len(b) >= 32 {
		return derive(b), p, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(p, buf, 0o600); err != nil {
		return nil, "", err
	}
	return derive(buf), p, nil
}

func derive(seed []byte) []byte {
	mac := hmac.New(sha256.New, seed)
	mac.Write([]byte(domainSep))
	return mac.Sum(nil)
}

func expandHome(p string) (string, error) {
	if !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p[2:]), nil
}

// Sign returns a share token for the given conversation ID.
func Sign(secret []byte, convID string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(convID))
	tag := mac.Sum(nil)[:tagBytes]
	return convID + "." + base64.RawURLEncoding.EncodeToString(tag)
}

// Verify returns the conversation ID encoded in token if its HMAC is valid.
func Verify(secret []byte, token string) (string, bool) {
	i := strings.LastIndexByte(token, '.')
	if i < 0 {
		return "", false
	}
	convID := token[:i]
	got, err := base64.RawURLEncoding.DecodeString(token[i+1:])
	if err != nil || len(got) != tagBytes {
		return "", false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(convID))
	want := mac.Sum(nil)[:tagBytes]
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return "", false
	}
	return convID, true
}

var _ = fmt.Sprintf
