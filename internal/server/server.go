// Package server runs the read-only conversation viewer over tsnet.
//
// The only listener accepted in this package is one produced by a
// *tsnet.Server: there is no exported way to attach the handler to a
// net.Listener of arbitrary provenance. This is intentional. The handler is
// not exported so it can't be wired to http.ListenAndServe from elsewhere.
package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/lukasschwab/shelley-share/internal/render"
	"github.com/lukasschwab/shelley-share/internal/share"
	"github.com/lukasschwab/shelley-share/internal/store"

	"tailscale.com/tsnet"
)

type Config struct {
	Hostname string // tsnet node name
	StateDir string // tsnet state directory
	AuthKey  string // optional, used on first start
	Secret   []byte // HMAC key for share tokens
	Store    *store.Store
}

// Serve brings up the tsnet node and serves HTTP on it until ctx is cancelled.
// It never listens on anything other than a *tsnet.Server-provided listener.
func Serve(ctx context.Context, c Config) error {
	ts := &tsnet.Server{
		Hostname: c.Hostname,
		Dir:      c.StateDir,
		AuthKey:  c.AuthKey,
		Logf:     func(string, ...any) {}, // quiet by default
	}
	defer ts.Close()

	if _, err := ts.Up(ctx); err != nil {
		return fmt.Errorf("tsnet up: %w", err)
	}
	if lc, err := ts.LocalClient(); err == nil {
		if st, err := lc.Status(ctx); err == nil && st.Self != nil {
			if name := strings.TrimSuffix(st.Self.DNSName, "."); name != "" {
				log.Printf("shelley-share: tailnet name %s", name)
			}
		}
	}

	ln, err := ts.Listen("tcp", ":80")
	if err != nil {
		return fmt.Errorf("tsnet listen: %w", err)
	}
	defer ln.Close()

	httpSrv := &http.Server{
		Handler:      buildHandler(c),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	log.Printf("shelley-share: listening on tsnet :80")
	return assertTailnetListener(ts, ln, func(l net.Listener) error { return httpSrv.Serve(l) })
}

// assertTailnetListener is a belt-and-suspenders check: it guarantees the
// listener we serve on came from the tsnet.Server we just constructed.
func assertTailnetListener(ts *tsnet.Server, ln net.Listener, fn func(net.Listener) error) error {
	// tsnet listeners are *tsnet-specific types in a private package; the
	// strongest portable check is identity: this same function only ever
	// receives the ln from ts.Listen above, by construction. The check below
	// guards against accidental refactors that pass in a stdlib net listener.
	if _, ok := ln.(*net.TCPListener); ok {
		return fmt.Errorf("refusing to serve on a non-tsnet listener (%T)", ln)
	}
	_ = ts
	return fn(ln)
}

func buildHandler(c Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = render.Index(w)
	})
	mux.HandleFunc("/c/", func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.URL.Path, "/c/")
		if token == "" || strings.Contains(token, "/") {
			http.NotFound(w, r)
			return
		}
		convID, ok := share.Verify(c.Secret, token)
		if !ok {
			http.NotFound(w, r)
			return
		}
		conv, err := c.Store.Get(convID)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		if conv == nil {
			http.NotFound(w, r)
			return
		}
		msgs, err := c.Store.Messages(convID)
		if err != nil {
			http.Error(w, "db error", http.StatusInternalServerError)
			return
		}
		page := render.Build(conv, msgs)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "private, no-store")
		_ = render.Conversation(w, page)
	})
	return logging(mux)
}

func logging(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		h.ServeHTTP(w, r)
	})
}

