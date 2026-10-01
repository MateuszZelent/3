package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func releaseClient(t *testing.T, handler func(*http.Request) (int, any)) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("User-Agent") == "" {
			t.Error("missing API headers")
		}
		status, body := handler(r)
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)), Body: io.NopCloser(strings.NewReader(string(encoded))), Request: r}, nil
	})}
}
func wireRelease(tag, date string) map[string]any {
	return map[string]any{"tag_name": tag, "published_at": date, "assets": []map[string]string{{"name": "mumax3", "browser_download_url": "https://github.com/MateuszZelent/3/releases/download/" + tag + "/mumax3"}}}
}

func TestReleaseCatalogPaginationFilteringAndPublicationOrder(t *testing.T) {
	calls := 0
	client := releaseClient(t, func(r *http.Request) (int, any) {
		calls++
		if r.URL.Query().Get("per_page") != "100" {
			t.Fatal(r.URL)
		}
		if calls == 1 {
			if r.URL.Query().Get("page") != "1" {
				t.Fatal(r.URL)
			}
			items := make([]map[string]any, 100)
			for i := range items {
				items[i] = wireRelease("v-old", "2026-09-30T10:00:00Z")
			}
			items[1]["draft"] = true
			items[2]["assets"] = nil
			items[3] = wireRelease("v-preview", "2026-10-02T08:00:00Z")
			items[3]["prerelease"] = true
			return 200, items
		}
		if r.URL.Query().Get("page") != "2" {
			t.Fatal(r.URL)
		}
		return 200, []map[string]any{wireRelease("v-new", "2026-10-01T09:00:00Z")}
	})
	releases, err := ListReleases(client)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(releases) != 3 || releases[0].Tag != "v-preview" || !releases[0].Prerelease || releases[1].Tag != "v-new" || releases[2].Tag != "v-old" {
		t.Fatalf("calls=%d releases=%+v", calls, releases)
	}
	var table strings.Builder
	PrintReleases(&table, releases, "v-new", "v-old")
	for _, want := range []string{"2026-10-01 09:00", "PUBLISHED (UTC)", "latest", "installed", "prerelease"} {
		if !strings.Contains(table.String(), want) {
			t.Fatalf("missing %q: %s", want, table.String())
		}
	}
}

func TestGetExactReleaseAndMissingAssetNeverFallBack(t *testing.T) {
	for _, version := range []string{"latest", "v3.12.12-20261001"} {
		client := releaseClient(t, func(r *http.Request) (int, any) {
			suffix := "/latest"
			if version != "latest" {
				suffix = "/tags/" + version
			}
			if !strings.HasSuffix(r.URL.Path, suffix) {
				t.Fatal(r.URL)
			}
			return 200, wireRelease("v3.12.12-20261001", "2026-10-01T10:39:22Z")
		})
		release, err := GetRelease(client, version)
		if err != nil || !strings.Contains(release.DownloadURL, "/v3.12.12-20261001/") {
			t.Fatalf("%+v %v", release, err)
		}
	}
	noAsset := releaseClient(t, func(r *http.Request) (int, any) { return 200, map[string]any{"tag_name": "empty", "assets": []any{}} })
	if _, err := GetRelease(noAsset, "empty"); err == nil {
		t.Fatal("release without binary accepted")
	}
	wrong := releaseClient(t, func(r *http.Request) (int, any) { return 200, wireRelease("wrong", "2026-10-01T00:00:00Z") })
	if _, err := GetRelease(wrong, "requested"); err == nil {
		t.Fatal("incorrect release accepted")
	}
	limited := releaseClient(t, func(r *http.Request) (int, any) { return 403, map[string]string{"message": "API rate limit exceeded"} })
	if _, err := ListReleases(limited); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Fatal(err)
	}
}

func TestReleaseSelectionAndCancellation(t *testing.T) {
	releases := []Release{{Tag: "preview", Prerelease: true}, {Tag: "stable", PublishedAt: time.Now()}, {Tag: "old"}}
	for _, test := range []struct {
		input, want string
		failure     bool
	}{
		{"2\n", "stable", false}, {"old\n", "old", false}, {"\n", "stable", false}, {"latest\n", "stable", false},
		{"q\n", "", false}, {"nonsense\n3\n", "old", false}, {"", "", true}, {"4", "", true},
	} {
		var output strings.Builder
		got, err := SelectRelease(strings.NewReader(test.input), &output, releases, "stable")
		if (err != nil) != test.failure {
			t.Fatalf("%q: %v", test.input, err)
		}
		if test.failure {
			continue
		}
		tag := ""
		if got != nil {
			tag = got.Tag
		}
		if tag != test.want {
			t.Fatalf("%q: got %q want %q", test.input, tag, test.want)
		}
	}
}
