// Package updater atomically replaces the running binary with a GitHub release.
package updater

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const DefaultURL = "https://github.com/MateuszZelent/3/releases/latest/download/mumax3"

func Apply(downloadURL string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return err
	}
	return ApplyTo(downloadURL, executable, http.DefaultClient)
}

func ApplyTo(downloadURL, executable string, client *http.Client) (retErr error) {
	response, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download update: HTTP %s", response.Status)
	}
	info, err := os.Stat(executable)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(executable), ".mumax3-update-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if retErr != nil {
			_ = os.Remove(name)
		}
	}()
	written, err := io.Copy(temporary, response.Body)
	if err != nil {
		return err
	}
	if written == 0 {
		return fmt.Errorf("download update: empty response body")
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, executable); err != nil {
		return fmt.Errorf("replace executable: %w", err)
	}
	return nil
}
