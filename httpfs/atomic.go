package httpfs

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const atomicAck = "mumax-httpfs-atomic-v1\n"

// ReplaceFile publishes a complete local file with rename in the destination
// directory. Both the file and directory are synced before success is reported.
// write must close its handles and must not modify the existing destination.
func ReplaceFile(filename string, write func(temporary string) error) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".mumax-write-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	mode := os.FileMode(0644)
	if info, err := os.Stat(filename); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		f.Close()
		return err
	}
	if err := errors.Join(f.Chmod(mode), f.Close()); err != nil {
		return err
	}
	if err := write(tmp); err != nil {
		return err
	}
	f, err = os.OpenFile(tmp, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := errors.Join(f.Sync(), f.Close()); err != nil {
		return err
	}
	if err := os.Rename(tmp, filename); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(d.Sync(), d.Close())
}

func localPutAtomic(filename string, data []byte) error {
	return ReplaceFile(filename, func(tmp string) error { return os.WriteFile(tmp, data, 0) })
}

// PutAtomic requires server support for the atomic PUT action on remote output.
// It intentionally does not fall back to an unsafe overwrite on older servers.
func PutAtomic(URL string, data []byte) error {
	URL = addWorkDir(URL)
	if isRemote(URL) {
		ack, err := do(PUTATOMIC, URL, data, nil)
		if err != nil {
			return err
		}
		if string(ack) != atomicAck {
			return fmt.Errorf("httpfs server did not acknowledge atomic write protocol v1: %s", URL)
		}
		return nil
	}
	return localPutAtomic(URL, data)
}

// Sync flushes buffered writers through their normal interface. Durability of
// append-only text output is provided by localAppend, including on the server.
func Sync(w io.Writer) error {
	if f, ok := w.(interface{ Flush() error }); ok {
		return f.Flush()
	}
	return nil
}
