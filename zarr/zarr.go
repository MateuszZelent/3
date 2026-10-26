// Package zarr implements the Zarr v2 array subset used by mumax3 output.
package zarr

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/DataDog/zstd"
	"github.com/mumax/3/data"
	"github.com/mumax/3/httpfs"
)

type Compressor struct {
	ID    string `json:"id"`
	Level int    `json:"level"`
}

// ArrayMetadata describes a five-dimensional [time,z,y,x,component] array.
type ArrayMetadata struct {
	Chunks     [5]int      `json:"chunks"`
	Compressor *Compressor `json:"compressor"`
	Dtype      string      `json:"dtype"`
	FillValue  float64     `json:"fill_value"`
	Filters    any         `json:"filters"`
	Order      string      `json:"order"`
	Shape      [5]int      `json:"shape"`
	ZarrFormat int         `json:"zarr_format"`
}

type Attributes struct {
	Times []float64 `json:"t"`
	Name  string    `json:"name,omitempty"`
	Unit  string    `json:"unit,omitempty"`
}

type SeriesMetadata struct {
	Chunks     [1]int      `json:"chunks"`
	Compressor *Compressor `json:"compressor"`
	Dtype      string      `json:"dtype"`
	FillValue  float64     `json:"fill_value"`
	Filters    any         `json:"filters"`
	Order      string      `json:"order"`
	Shape      [1]int      `json:"shape"`
	ZarrFormat int         `json:"zarr_format"`
}

func DefaultMetadata(size [3]int, ncomp, steps int, chunks [4]int) ArrayMetadata {
	return ArrayMetadata{
		Chunks:     [5]int{1, chunks[2], chunks[1], chunks[0], chunks[3]},
		Compressor: &Compressor{ID: "zstd", Level: 1},
		Dtype:      "<f4",
		FillValue:  0,
		Filters:    nil,
		Order:      "C",
		Shape:      [5]int{steps, size[2], size[1], size[0], ncomp},
		ZarrFormat: 2,
	}
}

func WriteGroup(dir string) error {
	if err := ensureDir(dir); err != nil {
		return err
	}
	return httpfs.PutAtomic(join(dir, ".zgroup"), []byte(`{"zarr_format":2}`))
}

func ensureDir(dir string) error {
	if err := httpfs.Mkdir(dir); err != nil && !isAlreadyExists(err) {
		return err
	}
	return nil
}

func WriteMetadata(dir string, metadata ArrayMetadata) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	b, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return httpfs.PutAtomic(join(dir, ".zarray"), b)
}

func ReadMetadata(dir string) (ArrayMetadata, error) {
	var metadata ArrayMetadata
	b, err := httpfs.Read(join(dir, ".zarray"))
	if err != nil {
		return metadata, err
	}
	if err := json.Unmarshal(b, &metadata); err != nil {
		return metadata, err
	}
	return metadata, validateMetadata(metadata)
}

func WriteAttributes(dir string, attrs Attributes) error {
	b, err := json.MarshalIndent(attrs, "", "  ")
	if err != nil {
		return err
	}
	return httpfs.PutAtomic(join(dir, ".zattrs"), b)
}

// WriteObjectAttributes writes arbitrary group-level Zarr v2 attributes.
func WriteObjectAttributes(dir string, attrs map[string]any) error {
	b, err := json.MarshalIndent(attrs, "", "  ")
	if err != nil {
		return err
	}
	return httpfs.PutAtomic(join(dir, ".zattrs"), b)
}

// WriteSeries is retired because replacing both .zarray and its single chunk
// cannot be made atomic for ordinary Zarr readers. Use tabular.Writer, whose
// manifest commits a consistent prefix of all columns.
//
// Deprecated: use tabular.Writer.
func WriteSeries(dir string, values []float64) error {
	return fmt.Errorf("zarr.WriteSeries is retired; use tabular.Writer for committed tables")
}

// WriteStep writes every spatial/component chunk for one time step and then
// atomically advances the logical shape in .zarray.
func WriteStep(dir string, step int, src *data.Slice, chunks [4]int) error {
	if step < 0 {
		return errors.New("zarr: negative step")
	}
	if err := validateChunkShape(src.Size(), src.NComp(), chunks); err != nil {
		return err
	}
	if err := ensureDir(dir); err != nil {
		return err
	}

	metadata := DefaultMetadata(src.Size(), src.NComp(), step+1, chunks)
	old, readErr := ReadMetadata(dir)
	if readErr != nil {
		// Older HTTP servers do not expose a typed ENOENT. A directory listing
		// lets us distinguish a genuinely absent .zarray from unreadable or
		// malformed metadata without silently replacing the latter.
		entries, listErr := httpfs.ReadDir(dir)
		if listErr != nil {
			return fmt.Errorf("zarr: inspect existing metadata for %s: %w", dir, errors.Join(readErr, listErr))
		}
		for _, entry := range entries {
			if entry == ".zarray" {
				return fmt.Errorf("zarr: read existing metadata for %s: %w", dir, readErr)
			}
		}
		if step != 0 {
			return fmt.Errorf("zarr: first step must be 0, got %d", step)
		}
	} else {
		if old.Chunks != metadata.Chunks || old.Shape[1] != metadata.Shape[1] || old.Shape[2] != metadata.Shape[2] || old.Shape[3] != metadata.Shape[3] || old.Shape[4] != metadata.Shape[4] || old.Dtype != metadata.Dtype || old.Order != metadata.Order || old.ZarrFormat != metadata.ZarrFormat || old.Filters != nil || old.Compressor == nil || *old.Compressor != *metadata.Compressor {
			return fmt.Errorf("zarr: dataset schema changed: old shape=%v chunks=%v, new shape=%v chunks=%v", old.Shape, old.Chunks, metadata.Shape, metadata.Chunks)
		}
		if old.Shape[0] != step {
			return fmt.Errorf("zarr: expected next step %d, got %d", old.Shape[0], step)
		}
	}
	_, err := writeStepChunks(dir, step, src, chunks, metadata)
	if err != nil {
		return err
	}
	return WriteMetadata(dir, metadata)
}

// writeStepChunks persists all chunks before publishing any logical metadata.
// Each checksum covers the exact compressed bytes that a reader will retrieve.
func writeStepChunks(dir string, step int, src *data.Slice, chunks [4]int, metadata ArrayMetadata) ([]ChunkRecord, error) {
	tensors := src.Tensors()
	nx, ny, nz := src.Size()[0], src.Size()[1], src.Size()[2]
	cx, cy, cz, cc := chunks[0], chunks[1], chunks[2], chunks[3]
	ncx, ncy, ncz, ncc := ceilDiv(nx, cx), ceilDiv(ny, cy), ceilDiv(nz, cz), ceilDiv(src.NComp(), cc)

	type job struct{ ix, iy, iz, ic int }
	jobs := make(chan job)
	errCh := make(chan error, 1)
	cancelled := make(chan struct{})
	var cancelOnce sync.Once
	var recordsMu sync.Mutex
	records := make([]ChunkRecord, 0, ncx*ncy*ncz*ncc)
	workerCount := min(runtime.GOMAXPROCS(0), ncx*ncy*ncz*ncc)
	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for j := range jobs {
			raw := make([]byte, cx*cy*cz*cc*4)
			pos := 0
			for dz := 0; dz < cz; dz++ {
				z := j.iz*cz + dz
				for dy := 0; dy < cy; dy++ {
					y := j.iy*cy + dy
					for dx := 0; dx < cx; dx++ {
						x := j.ix*cx + dx
						for dc := 0; dc < cc; dc++ {
							component := j.ic*cc + dc
							var value float32
							if z < nz && y < ny && x < nx && component < src.NComp() {
								value = tensors[component][z][y][x]
							}
							binary.LittleEndian.PutUint32(raw[pos:pos+4], math.Float32bits(value))
							pos += 4
						}
					}
				}
			}
			key := fmt.Sprintf("%d.%d.%d.%d.%d", step, j.iz, j.iy, j.ix, j.ic)
			compressed, err := zstd.CompressLevel(nil, raw, metadata.Compressor.Level)
			if err == nil {
				err = httpfs.PutAtomic(join(dir, key), compressed)
			}
			if err != nil {
				select {
				case errCh <- fmt.Errorf("zarr: write %s/%s: %w", dir, key, err):
				default:
				}
				cancelOnce.Do(func() { close(cancelled) })
				return
			}
			digest := sha256.Sum256(compressed)
			recordsMu.Lock()
			records = append(records, ChunkRecord{Key: key, Size: len(compressed), SHA256: hex.EncodeToString(digest[:])})
			recordsMu.Unlock()
		}
	}
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go worker()
	}
dispatch:
	for iz := 0; iz < ncz; iz++ {
		for iy := 0; iy < ncy; iy++ {
			for ix := 0; ix < ncx; ix++ {
				for ic := 0; ic < ncc; ic++ {
					select {
					case jobs <- job{ix: ix, iy: iy, iz: iz, ic: ic}:
					case <-cancelled:
						break dispatch
					}
				}
			}
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		return nil, err
	default:
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Key < records[j].Key })
	return records, nil
}

// ReadStep reconstructs a full data.Slice from all chunks for one time step.
func ReadStep(dir string, step int) (*data.Slice, error) {
	var metadata ArrayMetadata
	var expectedChunks map[string]ChunkRecord
	commit, err := ReadCommit(dir)
	if err == nil {
		metadata = commit.Metadata
		if step < 0 || step >= commit.CommittedSteps {
			return nil, fmt.Errorf("zarr: step %d outside committed prefix [0,%d)", step, commit.CommittedSteps)
		}
		frames, frameErr := ReadCommittedFrames(dir, commit)
		if frameErr != nil {
			return nil, frameErr
		}
		expectedChunks = make(map[string]ChunkRecord, len(frames[step].Chunks))
		for _, chunk := range frames[step].Chunks {
			expectedChunks[chunk.Key] = chunk
		}
	} else if errors.Is(err, os.ErrNotExist) {
		entries, listErr := httpfs.ReadDir(dir)
		if listErr != nil {
			return nil, listErr
		}
		for _, entry := range entries {
			if entry == ".frames" {
				return nil, fmt.Errorf("zarr: missing commit for recorded dataset %s", dir)
			}
		}
		metadata, err = ReadMetadata(dir)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	if step < 0 || step >= metadata.Shape[0] {
		return nil, fmt.Errorf("zarr: step %d outside [0,%d)", step, metadata.Shape[0])
	}
	if metadata.Dtype != "<f4" || metadata.Order != "C" || metadata.Compressor == nil || metadata.Compressor.ID != "zstd" {
		return nil, fmt.Errorf("zarr: unsupported metadata dtype=%q order=%q compressor=%v", metadata.Dtype, metadata.Order, metadata.Compressor)
	}

	nz, ny, nx, ncomp := metadata.Shape[1], metadata.Shape[2], metadata.Shape[3], metadata.Shape[4]
	cz, cy, cx, cc := metadata.Chunks[1], metadata.Chunks[2], metadata.Chunks[3], metadata.Chunks[4]
	dst := data.NewSlice(ncomp, [3]int{nx, ny, nz})
	tensors := dst.Tensors()
	for izc := 0; izc < ceilDiv(nz, cz); izc++ {
		for iyc := 0; iyc < ceilDiv(ny, cy); iyc++ {
			for ixc := 0; ixc < ceilDiv(nx, cx); ixc++ {
				for icc := 0; icc < ceilDiv(ncomp, cc); icc++ {
					key := fmt.Sprintf("%d.%d.%d.%d.%d", step, izc, iyc, ixc, icc)
					compressed, err := httpfs.Read(join(dir, key))
					if err != nil {
						dst.Free()
						return nil, err
					}
					if expectedChunks != nil {
						expected, ok := expectedChunks[key]
						digest := sha256.Sum256(compressed)
						if !ok || expected.Size != len(compressed) || expected.SHA256 != hex.EncodeToString(digest[:]) {
							dst.Free()
							return nil, fmt.Errorf("zarr: committed chunk %s failed integrity verification", key)
						}
					}
					raw, err := zstd.Decompress(nil, compressed)
					if err != nil {
						dst.Free()
						return nil, fmt.Errorf("zarr: decompress %s: %w", key, err)
					}
					expected := cz * cy * cx * cc * 4
					if len(raw) != expected {
						dst.Free()
						return nil, fmt.Errorf("zarr: chunk %s has %d bytes, want %d", key, len(raw), expected)
					}
					pos := 0
					for dz := 0; dz < cz; dz++ {
						z := izc*cz + dz
						for dy := 0; dy < cy; dy++ {
							y := iyc*cy + dy
							for dx := 0; dx < cx; dx++ {
								x := ixc*cx + dx
								for dc := 0; dc < cc; dc++ {
									component := icc*cc + dc
									if z < nz && y < ny && x < nx && component < ncomp {
										tensors[component][z][y][x] = math.Float32frombits(binary.LittleEndian.Uint32(raw[pos : pos+4]))
									}
									pos += 4
								}
							}
						}
					}
				}
			}
		}
	}
	return dst, nil
}

func validateMetadata(metadata ArrayMetadata) error {
	if metadata.ZarrFormat != 2 {
		return fmt.Errorf("zarr: unsupported format %d", metadata.ZarrFormat)
	}
	for i, value := range metadata.Chunks {
		if value <= 0 {
			return fmt.Errorf("zarr: chunks[%d] must be positive", i)
		}
	}
	if metadata.Chunks[0] != 1 || metadata.Shape[0] < 0 {
		return errors.New("zarr: invalid time shape or chunk size")
	}
	for i := 1; i < len(metadata.Shape); i++ {
		if metadata.Shape[i] <= 0 || metadata.Chunks[i] > metadata.Shape[i] {
			return fmt.Errorf("zarr: invalid shape[%d] or chunk", i)
		}
	}
	if metadata.Dtype != "<f4" || metadata.Order != "C" || metadata.Filters != nil || metadata.Compressor == nil || metadata.Compressor.ID != "zstd" {
		return errors.New("zarr: unsupported array schema")
	}
	return nil
}

func validateChunkShape(size [3]int, ncomp int, chunks [4]int) error {
	dims := [4]int{size[0], size[1], size[2], ncomp}
	for axis, chunk := range chunks {
		if chunk <= 0 || chunk > dims[axis] {
			return fmt.Errorf("zarr: chunk size %d for axis %d outside [1,%d]", chunk, axis, dims[axis])
		}
	}
	return nil
}

func ceilDiv(value, divisor int) int { return (value + divisor - 1) / divisor }

func join(dir, name string) string { return strings.TrimSuffix(dir, "/") + "/" + name }

func isAlreadyExists(err error) bool {
	return errors.Is(err, os.ErrExist) || strings.Contains(strings.ToLower(err.Error()), "file exists")
}
