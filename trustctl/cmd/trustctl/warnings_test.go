package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"trustissue/trustctl/internal/protocol"
	"trustissue/trustctl/internal/sim"
)

// hello and add warn about each flag word of the device's HELLO line (unlocked, nopin), and only those.
func TestHelloAndAddWarnAboutTheDevice(t *testing.T) {
	for _, c := range []struct {
		name              string
		unlocked, noPIN   bool
		wantLock, wantPIN bool
	}{
		{"a finished device", false, false, false, false},
		{"no PIN", false, true, false, true},
		{"unlocked", true, false, true, false},
		{"both", true, true, true, true},
	} {
		conn, dev := sim.New(sim.AcceptAll)
		dev.SetUnlocked(c.unlocked)
		dev.SetNoPIN(c.noPIN)
		client := protocol.NewClient(conn)
		if _, err := client.WaitBanner(time.Second); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if err := dispatch(client, "hello", nil, &stdout, &stderr, func(string) {}); err != nil {
			t.Fatal(err)
		}
		words := strings.Fields(stdout.String())
		hasWord := func(w string) bool {
			for _, x := range words {
				if x == w {
					return true
				}
			}
			return false
		}
		if hasWord("unlocked") != c.unlocked || hasWord("nopin") != c.noPIN {
			t.Errorf("%s: HELLO line %q", c.name, stdout.String())
		}
		if got := strings.Contains(stderr.String(), "not locked"); got != c.wantLock {
			t.Errorf("%s: unlocked warning %v, want %v: %q", c.name, got, c.wantLock, stderr.String())
		}
		if got := strings.Contains(stderr.String(), "no update PIN"); got != c.wantPIN {
			t.Errorf("%s: no-PIN warning %v, want %v: %q", c.name, got, c.wantPIN, stderr.String())
		}
		conn.Close()
	}
}
