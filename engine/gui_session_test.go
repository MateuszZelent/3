package engine

import (
	"testing"
	"time"
)

func TestInteractiveClientCountControlsGracePeriod(t *testing.T) {
	g := &guistate{}
	g.BrowserConnected()
	g.BrowserConnected()
	if got := g.ActiveClients(); got != 2 {
		t.Fatalf("active clients = %d", got)
	}
	if g.browserDisconnected(time.Now().Add(24 * time.Hour)) {
		t.Fatal("session closed while a client was active")
	}
	g.BrowserDisconnected()
	if g.ActiveClients() != 1 || g.browserDisconnected(time.Now().Add(24*time.Hour)) {
		t.Fatal("session closed after only one of two clients disconnected")
	}
	g.BrowserDisconnected()
	last := g.KeepAlive()
	if g.browserDisconnected(last.Add(Timeout - time.Nanosecond)) {
		t.Fatal("session closed before grace period")
	}
	if !g.browserDisconnected(last.Add(Timeout)) {
		t.Fatal("session did not close after grace period")
	}
}

func TestInteractiveReconnectCancelsGracePeriod(t *testing.T) {
	g := &guistate{}
	g.BrowserConnected()
	g.BrowserDisconnected()
	last := g.KeepAlive()
	if !g.browserDisconnected(last.Add(Timeout)) {
		t.Fatal("expected grace period to expire")
	}
	g.BrowserConnected()
	if g.ActiveClients() != 1 || g.browserDisconnected(time.Now().Add(24*time.Hour)) {
		t.Fatal("reconnect did not cancel shutdown")
	}
}
