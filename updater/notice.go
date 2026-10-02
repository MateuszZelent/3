package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"
)

const noticeTimeout = 3 * time.Second
const noticeTTL = 6 * time.Hour
const noticeRetry = time.Hour

var releaseVersion = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-(\d{8}))?$`)

// newerVersion compares official version tags numerically. Development builds
// and prerelease tags cannot be reliably ordered and never produce a notice.
func newerVersion(latest, current string) bool {
	a, b := releaseVersion.FindStringSubmatch(latest), releaseVersion.FindStringSubmatch(current)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i <= 4; i++ {
		x, errX := strconv.ParseUint("0"+a[i], 10, 64)
		y, errY := strconv.ParseUint("0"+b[i], 10, 64)
		if errX != nil || errY != nil {
			return false
		}
		if x != y {
			return x > y
		}
	}
	return false
}

type noticeCache struct {
	NextCheck time.Time `json:"next_check"`
	Release   Release   `json:"release"`
}

// StartReleaseNotice performs no disk or network work on the caller's thread.
// Cancellation never waits for the checker; failures remain silent. No work is
// scheduled on the solver or CUDA context. notify is called at most once.
func StartReleaseNotice(current string, notify func(string)) context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	if releaseVersion.FindStringSubmatch(current) == nil {
		return cancel
	}
	go func() {
		ctx, stop := context.WithTimeout(ctx, noticeTimeout)
		defer stop()
		dir, err := os.UserCacheDir()
		if err != nil {
			return
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.DisableKeepAlives = true
		client := &http.Client{Timeout: noticeTimeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		release := checkNotice(ctx, client, releasesAPI+"/latest", filepath.Join(dir, "mumax3", "release-notice"), time.Now())
		if ctx.Err() != nil || release.Prerelease || !newerVersion(release.Tag, current) {
			return
		}
		notify(fmt.Sprintf("A newer mumax3 release is available: %s (published %s UTC; installed %s). Run mumax3 --update to choose an update. https://github.com/MateuszZelent/3/releases/tag/%s", release.Tag, release.PublishedAt.UTC().Format("2006-01-02"), current, release.Tag))
	}()
	return cancel
}

func checkNotice(ctx context.Context, client *http.Client, endpoint, dir string, now time.Time) Release {
	// A private cache and nonblocking process lock prevent request storms in batch
	// jobs. An unwritable cache disables the optional check rather than bypassing
	// throttling. The OS releases the lock even if the process is terminated.
	if os.MkdirAll(dir, 0700) != nil {
		return Release{}
	}
	filename := filepath.Join(dir, "latest.json")
	read := func() noticeCache {
		f, err := os.Open(filename)
		if err != nil {
			return noticeCache{}
		}
		defer f.Close()
		var cached noticeCache
		if json.NewDecoder(io.LimitReader(f, 16384)).Decode(&cached) != nil {
			return noticeCache{}
		}
		return cached
	}
	cached := read()
	if cached.NextCheck.After(now) && cached.NextCheck.Before(now.Add(noticeTTL+time.Minute)) {
		return cached.Release
	}
	lock, err := os.OpenFile(filepath.Join(dir, "check.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return cached.Release
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		return cached.Release
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	cached = read() // another process may have completed since the first read
	if cached.NextCheck.After(now) && cached.NextCheck.Before(now.Add(noticeTTL+time.Minute)) {
		return cached.Release
	}
	write := func(value noticeCache) bool {
		f, err := os.CreateTemp(dir, ".latest-*")
		if err != nil {
			return false
		}
		defer os.Remove(f.Name())
		err = json.NewEncoder(f).Encode(value)
		closeErr := f.Close()
		return err == nil && closeErr == nil && os.Rename(f.Name(), filename) == nil
	}
	cached.NextCheck = now.Add(noticeRetry)
	if !write(cached) || ctx.Err() != nil {
		return cached.Release
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return cached.Release
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mumax3-release-notice")
	response, err := client.Do(req)
	if err != nil {
		return cached.Release
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return cached.Release
	}
	var item githubRelease
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&item) != nil {
		return cached.Release
	}
	release, ok := item.installable()
	if !ok || release.Prerelease || releaseVersion.FindStringSubmatch(release.Tag) == nil || release.PublishedAt.IsZero() {
		return cached.Release
	}
	_ = write(noticeCache{NextCheck: now.Add(noticeTTL), Release: release})
	return release
}
