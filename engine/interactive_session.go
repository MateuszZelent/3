package engine

import (
	"context"
	"sync"
	"time"
)

// InteractiveSessionTracker is the lifecycle contract for an interactive
// WebUI session. It deliberately has no dependency on Echo or WebSocket
// implementations so legacy and new frontends can share the same rules.
type InteractiveSessionTracker struct {
	mu           sync.Mutex
	grace        time.Duration
	seen         bool
	clients      int
	lastActivity time.Time
	wake         chan struct{}
}

// NewInteractiveSessionTracker creates a tracker with the supplied grace
// period. A non-positive period is still accepted so tests and callers can
// explicitly request immediate expiry.
func NewInteractiveSessionTracker(grace time.Duration) *InteractiveSessionTracker {
	return &InteractiveSessionTracker{grace: grace, wake: make(chan struct{})}
}

func (t *InteractiveSessionTracker) notifyLocked() {
	close(t.wake)
	t.wake = make(chan struct{})
}

// ClientConnected registers one main WebSocket client.
func (t *InteractiveSessionTracker) ClientConnected() {
	t.ClientConnectedAt(time.Now())
}

func (t *InteractiveSessionTracker) ClientConnectedAt(now time.Time) {
	t.mu.Lock()
	t.seen = true
	t.clients++
	t.lastActivity = now
	t.notifyLocked()
	t.mu.Unlock()
}

// ClientDisconnected unregisters one main WebSocket client. Duplicate
// disconnects are harmless and cannot make the count negative.
func (t *InteractiveSessionTracker) ClientDisconnected() {
	t.ClientDisconnectedAt(time.Now())
}

func (t *InteractiveSessionTracker) ClientDisconnectedAt(now time.Time) {
	t.mu.Lock()
	if t.clients > 0 {
		t.clients--
		if t.clients == 0 {
			t.lastActivity = now
		}
	}
	t.notifyLocked()
	t.mu.Unlock()
}

// Touch records activity from a legacy request-based GUI. Legacy GUI has no
// reliable WebSocket client count, so requests keep the grace deadline alive.
func (t *InteractiveSessionTracker) Touch() {
	t.TouchAt(time.Now())
}

func (t *InteractiveSessionTracker) TouchAt(now time.Time) {
	t.mu.Lock()
	t.seen = true
	t.lastActivity = now
	t.notifyLocked()
	t.mu.Unlock()
}

func (t *InteractiveSessionTracker) SeenClient() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.seen
}

func (t *InteractiveSessionTracker) ActiveClients() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.clients
}

func (t *InteractiveSessionTracker) expiredAtLocked(now time.Time) bool {
	return t.seen && t.clients == 0 && !t.lastActivity.IsZero() && now.Sub(t.lastActivity) >= t.grace
}

// ExpiredAt reports whether the session should close at now. It is useful for
// deterministic tests and for compatibility with the old keepalive adapter.
func (t *InteractiveSessionTracker) ExpiredAt(now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.expiredAtLocked(now)
}

// Wait blocks until the session has seen a client and its last client has
// remained disconnected for the grace period. It returns false when ctx is
// cancelled before expiry.
func (t *InteractiveSessionTracker) Wait(ctx context.Context) bool {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		t.mu.Lock()
		now := time.Now()
		if t.expiredAtLocked(now) {
			t.mu.Unlock()
			return true
		}
		wake := t.wake
		var deadline time.Time
		if t.seen && t.clients == 0 && !t.lastActivity.IsZero() {
			deadline = t.lastActivity.Add(t.grace)
		}
		t.mu.Unlock()

		if deadline.IsZero() {
			select {
			case <-ctx.Done():
				return false
			case <-wake:
				continue
			}
		}

		delay := time.Until(deadline)
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return false
		case <-wake:
			if !timer.Stop() {
				<-timer.C
			}
			continue
		case <-timer.C:
			continue
		}
	}
}
