package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestEmbeddedFrontendIndex(t *testing.T) {
	e := echo.New()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	if err := indexFileHandler()(e.NewContext(request, recorder)); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, "<html") || !strings.Contains(body, "_app/immutable") {
		t.Fatalf("embedded index does not look like the production frontend: %.120q", body)
	}
}

func TestEmbeddedStaticHandler(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/favicon.png", nil)
	recorder := httptest.NewRecorder()
	staticFileHandler("").ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.Len() == 0 {
		t.Fatalf("favicon response status=%d bytes=%d", recorder.Code, recorder.Body.Len())
	}
}
