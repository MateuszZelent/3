package zarr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"

	"github.com/mumax/3/data"
	"github.com/mumax/3/httpfs"
)

// ChunkRecord describes the stored, compressed representation of one chunk.
type ChunkRecord struct {
	Key    string `json:"key"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

// FrameRecord is immutable once its commit is published. PreviousSHA256 links
// each frame to the preceding record, so a pinned head identifies the history.
type FrameRecord struct {
	Step           int           `json:"step"`
	Time           float64       `json:"time"`
	PreviousSHA256 string        `json:"previous_sha256"`
	Chunks         []ChunkRecord `json:"chunks"`
}

// Commit is the single authoritative publication point for a field dataset.
// The ordinary Zarr files are compatibility projections and can lag behind it.
type Commit struct {
	SchemaVersion    int           `json:"schema_version"`
	Generation       int           `json:"generation"`
	CommittedSteps   int           `json:"committed_steps"`
	Metadata         ArrayMetadata `json:"metadata"`
	Name             string        `json:"name"`
	Unit             string        `json:"unit"`
	LastRecordSHA256 string        `json:"last_record_sha256"`
}

func readOptional(dir, key string) ([]byte, error) {
	b, err := httpfs.Read(join(dir, key))
	if err == nil {
		return b, nil
	}
	entries, listErr := httpfs.ReadDir(dir)
	if listErr != nil {
		return nil, errors.Join(err, listErr)
	}
	for _, entry := range entries {
		if entry == key {
			return nil, err
		}
	}
	return nil, fmt.Errorf("%s: %w", join(dir, key), os.ErrNotExist)
}

func ReadCommit(dir string) (Commit, error) {
	var commit Commit
	b, err := readOptional(dir, "commit.json")
	if err != nil {
		return commit, err
	}
	if err := json.Unmarshal(b, &commit); err != nil {
		return commit, fmt.Errorf("zarr: malformed commit in %s: %w", dir, err)
	}
	if commit.SchemaVersion != 1 || commit.Generation != commit.CommittedSteps || commit.CommittedSteps < 0 || commit.Metadata.Shape[0] != commit.CommittedSteps || (commit.CommittedSteps == 0 && commit.LastRecordSHA256 != "") || (commit.CommittedSteps > 0 && len(commit.LastRecordSHA256) != 64) {
		return commit, fmt.Errorf("zarr: invalid commit in %s", dir)
	}
	if err := validateMetadata(commit.Metadata); err != nil {
		return commit, fmt.Errorf("zarr: invalid committed schema in %s: %w", dir, err)
	}
	return commit, nil
}

func writeCommit(dir string, commit Commit) error {
	b, err := json.Marshal(commit)
	if err != nil {
		return err
	}
	return httpfs.PutAtomic(join(dir, "commit.json"), b)
}

// WriteCommittedStep publishes exactly one append-only frame. The initial
// zero-length Zarr projection is never advanced on this path; PublishProjection
// exports a conventional Zarr snapshot after output has drained.
func WriteCommittedStep(dir string, step int, src *data.Slice, chunks [4]int, timestamp float64, name, unit string) error {
	if step < 0 || math.IsNaN(timestamp) || math.IsInf(timestamp, 0) {
		return fmt.Errorf("zarr: invalid step or timestamp")
	}
	if err := validateChunkShape(src.Size(), src.NComp(), chunks); err != nil {
		return err
	}
	if err := ensureDir(dir); err != nil {
		return err
	}
	metadata := DefaultMetadata(src.Size(), src.NComp(), step, chunks)
	commit, err := ReadCommit(dir)
	if errors.Is(err, os.ErrNotExist) {
		if step != 0 {
			return fmt.Errorf("zarr: first committed step must be 0, got %d", step)
		}
		if _, oldErr := readOptional(dir, ".zarray"); !errors.Is(oldErr, os.ErrNotExist) {
			if oldErr == nil {
				return fmt.Errorf("zarr: refusing to create commit over existing dataset %s", dir)
			}
			return fmt.Errorf("zarr: inspect existing dataset %s: %w", dir, oldErr)
		}
		commit = Commit{SchemaVersion: 1, Metadata: metadata, Name: name, Unit: unit}
		// A crash during initialization exposes at most a zero-length legacy
		// projection; it must never expose uncommitted frame data.
		if err := WriteMetadata(dir, metadata); err != nil {
			return err
		}
		if err := WriteAttributes(dir, Attributes{Times: []float64{}, Name: name, Unit: unit}); err != nil {
			return err
		}
		if err := writeCommit(dir, commit); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	// Compressor and Filters contain pointers/interfaces. Compare their JSON
	// values, rather than pointer identity or potentially incomparable values.
	old, oldErr := json.Marshal(commit.Metadata)
	want, wantErr := json.Marshal(metadata)
	if oldErr != nil || wantErr != nil || commit.CommittedSteps != step || commit.Name != name || commit.Unit != unit || string(old) != string(want) {
		return fmt.Errorf("zarr: committed schema or next step does not match %s (expected step %d, got %d)", dir, commit.CommittedSteps, step)
	}
	if err := ensureDir(join(dir, ".frames")); err != nil {
		return err
	}
	records, err := writeStepChunks(dir, step, src, chunks, metadata)
	if err != nil {
		return err
	}
	frame := FrameRecord{Step: step, Time: timestamp, PreviousSHA256: commit.LastRecordSHA256, Chunks: records}
	b, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if err := httpfs.PutAtomic(join(dir, fmt.Sprintf(".frames/%020d.json", step)), b); err != nil {
		return err
	}
	digest := sha256.Sum256(b)
	commit.Generation++
	commit.CommittedSteps++
	commit.Metadata.Shape[0]++
	commit.LastRecordSHA256 = hex.EncodeToString(digest[:])
	return writeCommit(dir, commit)
}

// ReadCommittedFrames checks the record chain against one pinned commit.
func ReadCommittedFrames(dir string, commit Commit) ([]FrameRecord, error) {
	frames := make([]FrameRecord, 0, commit.CommittedSteps)
	previous := ""
	for step := 0; step < commit.CommittedSteps; step++ {
		b, err := httpfs.Read(join(dir, fmt.Sprintf(".frames/%020d.json", step)))
		if err != nil {
			return nil, fmt.Errorf("zarr: read committed frame %d in %s: %w", step, dir, err)
		}
		var frame FrameRecord
		if err := json.Unmarshal(b, &frame); err != nil || frame.Step != step || frame.PreviousSHA256 != previous || math.IsNaN(frame.Time) || math.IsInf(frame.Time, 0) {
			return nil, fmt.Errorf("zarr: invalid committed frame %d in %s", step, dir)
		}
		m := commit.Metadata
		expected := make(map[string]bool)
		for iz := 0; iz < ceilDiv(m.Shape[1], m.Chunks[1]); iz++ {
			for iy := 0; iy < ceilDiv(m.Shape[2], m.Chunks[2]); iy++ {
				for ix := 0; ix < ceilDiv(m.Shape[3], m.Chunks[3]); ix++ {
					for ic := 0; ic < ceilDiv(m.Shape[4], m.Chunks[4]); ic++ {
						expected[fmt.Sprintf("%d.%d.%d.%d.%d", step, iz, iy, ix, ic)] = true
					}
				}
			}
		}
		for _, chunk := range frame.Chunks {
			if !expected[chunk.Key] || chunk.Size <= 0 || len(chunk.SHA256) != 64 {
				return nil, fmt.Errorf("zarr: invalid chunk record in frame %d of %s", step, dir)
			}
			if _, err := hex.DecodeString(chunk.SHA256); err != nil {
				return nil, fmt.Errorf("zarr: invalid chunk checksum in frame %d of %s", step, dir)
			}
			delete(expected, chunk.Key)
		}
		if len(expected) != 0 {
			return nil, fmt.Errorf("zarr: incomplete chunk records in frame %d of %s", step, dir)
		}
		digest := sha256.Sum256(b)
		previous = hex.EncodeToString(digest[:])
		frames = append(frames, frame)
	}
	if previous != commit.LastRecordSHA256 {
		return nil, fmt.Errorf("zarr: committed record hash mismatch in %s", dir)
	}
	return frames, nil
}

// PublishProjection updates ordinary Zarr metadata for tools that do not
// understand commit.json. Transactional readers must continue to pin commit.
func PublishProjection(dir string) error {
	commit, err := ReadCommit(dir)
	if err != nil {
		return err
	}
	frames, err := ReadCommittedFrames(dir, commit)
	if err != nil {
		return err
	}
	times := make([]float64, len(frames))
	for i := range frames {
		times[i] = frames[i].Time
	}
	if err := WriteAttributes(dir, Attributes{Times: times, Name: commit.Name, Unit: commit.Unit}); err != nil {
		return err
	}
	return WriteMetadata(dir, commit.Metadata)
}
