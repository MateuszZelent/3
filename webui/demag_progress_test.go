package webui

import (
	"encoding/json"
	"github.com/labstack/echo/v4"
	"github.com/mumax/3/util"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDemagProgressIndependentOfEngineState(t *testing.T) {
	id := util.BeginDemagInitialization()
	defer util.EndDemagInitialization(id, true)
	// Holding the state lock simulates an engine-backed request in progress.
	ws := &WebSocketManager{}
	ws.stateMu.Lock()
	defer ws.stateMu.Unlock()
	e := echo.New()
	rec := httptest.NewRecorder()
	if err := getDemagProgress(e.NewContext(httptest.NewRequest(http.MethodGet, "/api/demag/progress", nil), rec)); err != nil {
		t.Fatal(err)
	}
	var s util.DemagProgressState
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatal(err)
	}
	if !s.Active || s.ID != id || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("bad progress response: %s", rec.Body)
	}
}
