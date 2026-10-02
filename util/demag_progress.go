package util

import (
	"strings"
	"sync"
	"time"
)

// Kept separately from engine locks so the UI can poll during initialization.
type DemagProgressState struct {
	ID        uint64   `json:"id"`
	Active    bool     `json:"active"`
	Failed    bool     `json:"failed"`
	Stage     string   `json:"stage"`
	Percent   int      `json:"percent"`
	StartedAt int64    `json:"startedAt"`
	Logs      []string `json:"logs"`
}

var demagProgress struct {
	sync.Mutex
	state DemagProgressState
}

func BeginDemagInitialization() uint64 {
	demagProgress.Lock()
	defer demagProgress.Unlock()
	id := demagProgress.state.ID + 1
	demagProgress.state = DemagProgressState{ID: id, Active: true, Stage: "Preparing demagnetization", Percent: -1, StartedAt: time.Now().UnixMilli(), Logs: []string{}}
	return id
}
func EndDemagInitialization(id uint64, success bool) {
	demagProgress.Lock()
	defer demagProgress.Unlock()
	if id != demagProgress.state.ID {
		return
	}
	demagProgress.state.Active = false
	demagProgress.state.Failed = !success
	if success {
		demagProgress.state.Stage = "Demagnetization ready"
		demagProgress.state.Percent = 100
	} else {
		demagProgress.state.Stage = "Demagnetization initialization failed"
		demagProgress.state.Percent = -1
	}
}
func DemagProgressSnapshot() DemagProgressState {
	demagProgress.Lock()
	defer demagProgress.Unlock()
	state := demagProgress.state
	state.Logs = append([]string{}, state.Logs...)
	return state
}
func recordDemagLog(message string) {
	lower := strings.ToLower(message)
	if !strings.Contains(lower, "demag") && !strings.Contains(lower, "kernel cache") {
		return
	}
	demagProgress.Lock()
	defer demagProgress.Unlock()
	s := &demagProgress.state
	if !s.Active {
		return
	}
	s.Logs = append(s.Logs, time.Now().Format("15:04:05")+" "+strings.TrimSpace(message))
	if len(s.Logs) > 80 {
		s.Logs = s.Logs[len(s.Logs)-80:]
	}
	s.Stage = message
	s.Percent = -1
}
func recordDemagProgress(done, total int, message string) {
	if !strings.Contains(strings.ToLower(message), "demag") {
		return
	}
	demagProgress.Lock()
	defer demagProgress.Unlock()
	s := &demagProgress.state
	if !s.Active {
		return
	}
	s.Stage = message
	s.Percent = -1
	if total > 0 {
		s.Percent = min(100, max(0, int(float64(done)*100/float64(total))))
	}
}
