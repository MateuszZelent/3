package main

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/mumax/3/updater"
)

func runUpdateCommand() error {
	catalogClient := &http.Client{Timeout: 30 * time.Second}
	current := buildVersion
	fmt.Println(buildSummary())
	var selected *updater.Release
	if *flag_updateVersion != "" {
		release, err := updater.GetRelease(catalogClient, *flag_updateVersion)
		if err != nil {
			return err
		}
		selected = &release
	} else {
		releases, err := updater.ListReleases(catalogClient)
		if err != nil {
			return err
		}
		latest, err := updater.GetRelease(catalogClient, "latest")
		if err != nil {
			return err
		}
		// A release can be published between the catalog and latest requests.
		foundLatest := false
		for _, release := range releases {
			if release.Tag == latest.Tag {
				foundLatest = true
				break
			}
		}
		if !foundLatest {
			releases = append(releases, latest)
			sort.SliceStable(releases, func(i, j int) bool { return releases[i].PublishedAt.After(releases[j].PublishedAt) })
		}
		updater.PrintReleases(os.Stdout, releases, latest.Tag, current)
		if *flag_updateList {
			return nil
		}
		selected, err = updater.SelectRelease(os.Stdin, os.Stdout, releases, latest.Tag)
		if err != nil {
			return err
		}
		if selected == nil {
			fmt.Println("Update cancelled")
			return nil
		}
	}
	fmt.Printf("Updating mumax3: %s -> %s (published %s UTC)\n", current, selected.Tag, selected.PublishedAt.UTC().Format("2006-01-02 15:04"))
	fmt.Println("Downloading", selected.DownloadURL)
	if err := updater.Apply(selected.DownloadURL); err != nil {
		return err
	}
	fmt.Printf("Update complete: %s installed. Run mumax3 --version to verify.\n", selected.Tag)
	return nil
}
