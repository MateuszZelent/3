package engine

import (
	"testing"
	"time"
)

func TestInteractiveWaitsForFirstBrowserConnection(t *testing.T) {
	g := &guistate{}

	if g.browserDisconnected(time.Now().Add(24 * time.Hour)) {
		t.Fatal("interactive mode must not exit before a browser has connected")
	}
}

func TestInteractiveExitsAfterConnectedBrowserTimesOut(t *testing.T) {
	g := &guistate{}
	g.UpdateKeepAlive()

	lastSeen := g.KeepAlive()
	if g.browserDisconnected(lastSeen.Add(Timeout - time.Nanosecond)) {
		t.Fatal("interactive mode exited before the browser timeout")
	}
	if !g.browserDisconnected(lastSeen.Add(Timeout)) {
		t.Fatal("interactive mode did not exit after a connected browser timed out")
	}
}

func TestInteractiveKeepAliveRefreshesConnection(t *testing.T) {
	g := &guistate{}
	g.UpdateKeepAlive()
	first := g.KeepAlive()
	time.Sleep(time.Millisecond)
	g.UpdateKeepAlive()
	if !g.KeepAlive().After(first) {
		t.Fatal("browser keepalive was not refreshed")
	}
}
