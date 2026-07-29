package util

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestTerminalProgressAndHide(t *testing.T) {
	previousOut, previousWidth, previousColor := progressOut, progressWidth, progressColor
	previousHidden := progressHidden
	defer func() {
		progressOut, progressWidth, progressColor, progressHidden = previousOut, previousWidth, previousColor, previousHidden
	}()
	var output bytes.Buffer
	progressOut = &output
	progressWidth = func() int { return 40 }
	progressColor = func() bool { return false }
	progressHidden = false
	lastPct, lastProgMsg, lastProgT = -1, "", time.Time{}
	TerminalProgress(1, 2, "Running")
	if text := output.String(); !strings.Contains(text, "Running") || !strings.Contains(text, "50%") {
		t.Fatalf("progress output = %q", text)
	}
	output.Reset()
	SetProgressHidden(true)
	Progress(1, 1, "Hidden")
	if output.Len() != 0 {
		t.Fatalf("hidden progress wrote %q", output.String())
	}
}
