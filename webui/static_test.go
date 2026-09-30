package webui

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
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

// Catch frontend builds that were not copied into the directory embedded by Go.
func TestEmbeddedFrontendMatchesBuild(t *testing.T) {
	if _, err := os.Stat("../frontend/dist/index.html"); os.IsNotExist(err) {
		t.Skip("frontend build is not available")
	} else if err != nil {
		t.Fatal(err)
	}
	built := os.DirFS("../frontend/dist")
	err := fs.WalkDir(built, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		want, err := fs.ReadFile(built, path)
		if err != nil {
			return err
		}
		got, err := staticFiles.ReadFile("static/" + path)
		if err != nil {
			t.Errorf("embedded frontend missing %s: run npm run build: %v", path, err)
			return nil
		}
		if !bytes.Equal(got, want) {
			t.Errorf("embedded frontend is stale: %s; run npm run build", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
