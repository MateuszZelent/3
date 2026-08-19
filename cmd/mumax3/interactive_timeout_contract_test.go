package main

import (
	"testing"
	"time"

	"github.com/mumax/3/engine"
)

func TestConfigureInteractiveTimeoutUsesCLIFlag(t *testing.T) {
	oldFlag := *engine.Flag_interactiveTimeout
	oldTimeout := engine.Timeout
	defer func() {
		*engine.Flag_interactiveTimeout = oldFlag
		engine.Timeout = oldTimeout
	}()

	*engine.Flag_interactiveTimeout = 137 * time.Millisecond
	configureInteractiveTimeout()
	if engine.Timeout != 137*time.Millisecond {
		t.Fatalf("engine timeout = %s, want 137ms", engine.Timeout)
	}
}
