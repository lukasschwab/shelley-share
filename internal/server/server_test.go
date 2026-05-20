package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lukasschwab/shelley-share/internal/share"
	"github.com/lukasschwab/shelley-share/internal/store"
)

func TestHandlerRejectsForgedToken(t *testing.T) {
	st, err := store.Open("/home/exedev/.config/shelley/shelley.db")
	if err != nil {
		t.Skipf("no shelley.db: %v", err)
	}
	defer st.Close()
	h := buildHandler(Config{Secret: []byte("k"), Store: st, Scrubber: nil})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/c/c2SEXJF.AAAAAAAAAAAAAAAA", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("forged token: got %d, want 404", rec.Code)
	}

	tok := share.Sign([]byte("k"), "c2SEXJF")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/c/"+tok, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: got %d\n%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "shelley-share") {
		t.Fatalf("body missing footer: %s", rec.Body.String()[:200])
	}
}

func TestRoundTripToken(t *testing.T) {
	sec := []byte("some-key")
	tok := share.Sign(sec, "abc")
	if id, ok := share.Verify(sec, tok); !ok || id != "abc" {
		t.Fatalf("verify failed: %q %v", id, ok)
	}
	if _, ok := share.Verify([]byte("other"), tok); ok {
		t.Fatal("verify with wrong key should fail")
	}
}
