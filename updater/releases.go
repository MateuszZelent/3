package updater

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

const releasesAPI = "https://api.github.com/repos/MateuszZelent/3/releases"

// Release is an installable published release, with its actual asset URL.
type Release struct {
	Tag         string
	PublishedAt time.Time
	Prerelease  bool
	DownloadURL string
}

type githubRelease struct {
	Tag         string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (r githubRelease) installable() (Release, bool) {
	if r.Draft || r.Tag == "" {
		return Release{}, false
	}
	for _, asset := range r.Assets {
		if asset.Name == "mumax3" && asset.URL != "" {
			return Release{Tag: r.Tag, PublishedAt: r.PublishedAt, Prerelease: r.Prerelease, DownloadURL: asset.URL}, true
		}
	}
	return Release{}, false
}

func getReleaseJSON(client *http.Client, endpoint string, result any) error {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mumax3-updater")
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch releases: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var problem struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&problem)
		if problem.Message != "" {
			return fmt.Errorf("fetch releases: HTTP %s: %s", response.Status, problem.Message)
		}
		return fmt.Errorf("fetch releases: HTTP %s", response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return fmt.Errorf("decode releases: %w", err)
	}
	return nil
}

// ListReleases includes every page, excludes drafts/missing binaries, and orders
// by publication date rather than GitHub's release creation date.
func ListReleases(client *http.Client) ([]Release, error) {
	var releases []Release
	seen := make(map[string]bool)
	for page := 1; page <= 100; page++ {
		var items []githubRelease
		if err := getReleaseJSON(client, fmt.Sprintf("%s?per_page=100&page=%d", releasesAPI, page), &items); err != nil {
			return nil, err
		}
		for _, item := range items {
			if release, ok := item.installable(); ok && !seen[release.Tag] {
				releases = append(releases, release)
				seen[release.Tag] = true
			}
		}
		if len(items) < 100 {
			sort.SliceStable(releases, func(i, j int) bool { return releases[i].PublishedAt.After(releases[j].PublishedAt) })
			if len(releases) == 0 {
				return nil, fmt.Errorf("no published releases contain a mumax3 binary")
			}
			return releases, nil
		}
	}
	return nil, fmt.Errorf("release list exceeds 100 pages")
}

// GetRelease resolves latest through GitHub's actual latest endpoint, or looks
// up the exact requested tag. Missing assets never fall back to another version.
func GetRelease(client *http.Client, version string) (Release, error) {
	endpoint := releasesAPI + "/latest"
	if version != "latest" {
		endpoint = releasesAPI + "/tags/" + url.PathEscape(version)
	}
	var item githubRelease
	if err := getReleaseJSON(client, endpoint, &item); err != nil {
		return Release{}, err
	}
	release, ok := item.installable()
	if !ok {
		return Release{}, fmt.Errorf("release %q has no published mumax3 binary", version)
	}
	if version != "latest" && release.Tag != version {
		return Release{}, fmt.Errorf("requested %q but API returned %q", version, release.Tag)
	}
	return release, nil
}

func PrintReleases(output io.Writer, releases []Release, latest, current string) {
	fmt.Fprintf(output, "Installed version: %s\n\n", current)
	table := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(table, "#\tVERSION\tPUBLISHED (UTC)\tSTATUS")
	for i, r := range releases {
		var labels []string
		if r.Tag == latest {
			labels = append(labels, "latest")
		}
		if r.Tag == current {
			labels = append(labels, "installed")
		}
		if r.Prerelease {
			labels = append(labels, "prerelease")
		}
		date := "unknown"
		if !r.PublishedAt.IsZero() {
			date = r.PublishedAt.UTC().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(table, "%d\t%s\t%s\t%s\n", i+1, r.Tag, date, strings.Join(labels, ", "))
	}
	_ = table.Flush()
}

// SelectRelease accepts a number, an exact tag, latest, or cancellation. EOF
// never silently installs the default; scripts can use --update-version.
func SelectRelease(input io.Reader, output io.Writer, releases []Release, latest string) (*Release, error) {
	reader := bufio.NewReader(input)
	for {
		fmt.Fprint(output, "\nSelect release number or tag [Enter = latest, q = cancel]: ")
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("read selection: %w", err)
		}
		if err == io.EOF && len(line) == 0 {
			return nil, fmt.Errorf("no release selected; for unattended updates use --update-version latest or --update-version TAG")
		}
		choice := strings.TrimSpace(line)
		if choice == "q" || choice == "quit" || choice == "cancel" {
			return nil, nil
		}
		if choice == "" || choice == "latest" {
			choice = latest
		}
		for i := range releases {
			if releases[i].Tag == choice {
				return &releases[i], nil
			}
		}
		if n, parseErr := strconv.Atoi(choice); parseErr == nil && n >= 1 && n <= len(releases) {
			return &releases[n-1], nil
		}
		fmt.Fprintf(output, "Unknown release %q. Choose 1–%d, an exact tag, or q.\n", choice, len(releases))
		if err == io.EOF {
			return nil, fmt.Errorf("invalid release selection %q", choice)
		}
	}
}
