// Package cli wires up the shelley-share command surface.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/lukasschwab/shelley-share/internal/server"
	"github.com/lukasschwab/shelley-share/internal/share"
	"github.com/lukasschwab/shelley-share/internal/store"
)

type globalFlags struct {
	dbPath     string
	stateDir   string
	seedPath   string // override secret seed path
}

func (g *globalFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&g.dbPath, "db", defaultDB(), "path to shelley.db")
	fs.StringVar(&g.stateDir, "state", defaultState(), "shelley-share state directory")
	fs.StringVar(&g.seedPath, "seed", "", "override secret seed file (default: ~/.ssh/id_ed25519)")
}

func defaultDB() string {
	if v := os.Getenv("SHELLEY_DB"); v != "" {
		return v
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".config", "shelley", "shelley.db")
	}
	return "shelley.db"
}

func defaultState() string {
	if v := os.Getenv("SHELLEY_SHARE_STATE"); v != "" {
		return v
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".config", "shelley-share")
	}
	return ".shelley-share"
}

func seedPaths(override string) []string {
	if override != "" {
		return []string{override}
	}
	return nil
}

// Run dispatches to a subcommand.
func Run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("no subcommand")
	}
	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "link":
		return runLink(args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `shelley-share — read-only Shelley conversation viewer over tsnet

Usage:
  shelley-share serve [flags]
  shelley-share link  [flags] <conversation_id>

Global flags:
  -db path           path to shelley.db
  -state dir         shelley-share state directory
  -seed path         override secret seed file (default ~/.ssh/id_ed25519)

serve flags:
  -hostname name     tsnet node name (default "shelley-share")
  -ts-authkey key    Tailscale auth key on first start (or env TS_AUTHKEY)
  -base-url url      override stored base URL used by 'link'

link flags:
  -base-url url      override base URL
`)
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	var g globalFlags
	g.bind(fs)
	hostname := fs.String("hostname", "shelley-share", "tsnet node name")
	authKey := fs.String("ts-authkey", os.Getenv("TS_AUTHKEY"), "Tailscale auth key")
	baseURL := fs.String("base-url", "", "override base URL written for 'link'")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	secret, src, err := share.LoadSecret(g.stateDir, seedPaths(g.seedPath))
	if err != nil {
		return fmt.Errorf("load secret: %w", err)
	}
	log.Printf("shelley-share: secret derived from %s", src)

	st, err := store.Open(g.dbPath)
	if err != nil {
		return err
	}
	defer st.Close()

	tsState := filepath.Join(g.stateDir, "tsnet")
	if err := os.MkdirAll(tsState, 0o700); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Write base URL hint so `link` can construct URLs without re-running tsnet.
	if *baseURL == "" {
		*baseURL = "http://" + *hostname
	}
	if err := os.WriteFile(filepath.Join(g.stateDir, "base_url"), []byte(*baseURL+"\n"), 0o644); err != nil {
		log.Printf("warning: could not write base_url: %v", err)
	}

	return server.Serve(ctx, server.Config{
		Hostname: *hostname,
		StateDir: tsState,
		AuthKey:  *authKey,
		Secret:   secret,
		Store:    st,
	})
}

func runLink(args []string) error {
	fs := flag.NewFlagSet("link", flag.ContinueOnError)
	var g globalFlags
	g.bind(fs)
	baseURL := fs.String("base-url", "", "override base URL")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: shelley-share link <conversation_id>")
	}
	convID := fs.Arg(0)

	secret, _, err := share.LoadSecret(g.stateDir, seedPaths(g.seedPath))
	if err != nil {
		return fmt.Errorf("load secret: %w", err)
	}

	// Validate that the conversation exists locally before printing a link.
	if st, err := store.Open(g.dbPath); err == nil {
		defer st.Close()
		if ok, _ := st.Exists(convID); !ok {
			return fmt.Errorf("conversation %s not found in %s", convID, g.dbPath)
		}
	}

	base := *baseURL
	if base == "" {
		b, err := os.ReadFile(filepath.Join(g.stateDir, "base_url"))
		if err == nil {
			base = strings.TrimSpace(string(b))
		}
	}
	token := share.Sign(secret, convID)
	if base == "" {
		fmt.Println("/c/" + token)
		return nil
	}
	u, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("base url: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/c/" + token
	fmt.Println(u.String())
	return nil
}
