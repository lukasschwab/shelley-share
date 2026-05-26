package server

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestFilteredTsnetLogf(t *testing.T) {
	var buf bytes.Buffer
	default_ := log.Default().Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(default_) })

	// Forwarded lines (the ones we care about).
	filteredTsnetLogf("To start this tsnet server, restart with TS_AUTHKEY set, or go to: %s", "https://login.tailscale.com/a/abcdef")
	filteredTsnetLogf("NeedsLogin")
	// Suppressed lines (chatty internals).
	filteredTsnetLogf("magicsock: derp-1 connected")
	filteredTsnetLogf("netcheck: report: udp=true v4=true v6=false mappingvarieswithdstip=false")

	out := buf.String()
	if !strings.Contains(out, "login.tailscale.com/a/abcdef") {
		t.Errorf("login URL not forwarded:\n%s", out)
	}
	if !strings.Contains(out, "NeedsLogin") {
		t.Errorf("NeedsLogin not forwarded:\n%s", out)
	}
	if strings.Contains(out, "magicsock") || strings.Contains(out, "netcheck") {
		t.Errorf("chatty lines leaked through:\n%s", out)
	}
}
