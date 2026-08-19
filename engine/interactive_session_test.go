package engine

import (
	"context"
	"testing"
	"time"
)

func TestInteractiveSessionTrackerWaitsForFirstClient(t *testing.T) {
	tracker := NewInteractiveSessionTracker(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if tracker.Wait(ctx) {
		t.Fatal("tracker expired before its first client")
	}
	if tracker.SeenClient() {
		t.Fatal("tracker marked an absent client as seen")
	}
}

func TestInteractiveSessionTrackerReconnectCancelsGracePeriod(t *testing.T) {
	tracker := NewInteractiveSessionTracker(20 * time.Millisecond)
	tracker.ClientConnected()
	tracker.ClientConnected()
	if tracker.ActiveClients() != 2 {
		t.Fatalf("active clients = %d", tracker.ActiveClients())
	}
	tracker.ClientDisconnected()
	if tracker.ActiveClients() != 1 {
		t.Fatalf("active clients after one disconnect = %d", tracker.ActiveClients())
	}
	tracker.ClientDisconnected()
	tracker.ClientConnected()
	if tracker.ActiveClients() != 1 || tracker.ExpiredAt(time.Now().Add(time.Hour)) {
		t.Fatal("reconnect did not cancel the grace period")
	}
}

func TestInteractiveSessionTrackerWaitReturnsAfterLastDisconnect(t *testing.T) {
	tracker := NewInteractiveSessionTracker(5 * time.Millisecond)
	tracker.ClientConnected()
	tracker.ClientDisconnected()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !tracker.Wait(ctx) {
		t.Fatal("tracker did not expire after the grace period")
	}
}
