package httpfs

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// AcquireOutputLock claims a sibling directory using one atomic mkdir. The
// lock survives a crash: stale locks require explicit operator inspection.
// The caller must retain the lock until every output producer has stopped.
func AcquireOutputLock(output string) (func() error, error) {
	lock := strings.TrimRight(output, "/") + ".zarr-owner"
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	owner, err := json.Marshal(map[string]any{
		"token": hex.EncodeToString(token[:]), "pid": os.Getpid(), "host": host,
		"started_utc": time.Now().UTC().Format(time.RFC3339Nano), "output": output,
	})
	if err != nil {
		return nil, err
	}
	if err := Mkdir(lock); err != nil {
		return nil, fmt.Errorf("cannot claim output lock %s (another writer or stale lock): %w", lock, err)
	}
	if err := PutAtomic(lock+"/owner.json", owner); err != nil {
		_ = Remove(lock)
		return nil, err
	}
	return func() error {
		current, err := Read(lock + "/owner.json")
		if err != nil {
			return err
		}
		if !bytes.Equal(current, owner) {
			return fmt.Errorf("output lock ownership changed: %s", lock)
		}
		return Remove(lock)
	}, nil
}
