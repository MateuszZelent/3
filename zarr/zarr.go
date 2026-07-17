// Package zarr implements the Zarr v2 array subset used by mumax3 output.
package zarr

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"runtime"
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
	if err := httpfs.Mkdir(dir); err != nil && !isAlreadyExists(err) {
		return err
	}
	return httpfs.Put(join(dir, ".zgroup"), []byte(`{"zarr_format":2}`))
}

func WriteMetadata(dir string, metadata ArrayMetadata) error {
	if err := validateMetadata(metadata); err != nil {
		return err
	}
	b, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return httpfs.Put(join(dir, ".zarray"), b)
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
	return httpfs.Put(join(dir, ".zattrs"), b)
}

// WriteSeries writes a complete float64 one-dimensional Zarr array. Table
// columns are finalized through this path.
func WriteSeries(dir string, values []float64) error {
	if len(values) == 0 {
		return nil
	}
	if err := WriteGroup(dir); err != nil {
		return err
	}
	metadata := SeriesMetadata{
		Chunks:     [1]int{len(values)},
		Compressor: &Compressor{ID: "zstd", Level: 1},
		Dtype:      "<f8",
		FillValue:  0,
		Filters:    nil,
		Order:      "C",
		Shape:      [1]int{len(values)},
		ZarrFormat: 2,
	}
	b, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	if err := httpfs.Put(join(dir, ".zarray"), b); err != nil {
		return err
	}
	raw := make([]byte, len(values)*8)
	for i, value := range values {
		binary.LittleEndian.PutUint64(raw[i*8:(i+1)*8], math.Float64bits(value))
	}
	compressed, err := zstd.CompressLevel(nil, raw, metadata.Compressor.Level)
	if err != nil {
		return err
	}
	return httpfs.Put(join(dir, "0"), compressed)
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
	if err := WriteGroup(dir); err != nil {
		return err
	}

	metadata := DefaultMetadata(src.Size(), src.NComp(), step+1, chunks)
	if old, err := ReadMetadata(dir); err == nil {
		if old.Chunks != metadata.Chunks || old.Shape[1] != metadata.Shape[1] || old.Shape[2] != metadata.Shape[2] || old.Shape[3] != metadata.Shape[3] || old.Shape[4] != metadata.Shape[4] {
			return fmt.Errorf("zarr: dataset shape or chunks changed: old shape=%v chunks=%v, new shape=%v chunks=%v", old.Shape, old.Chunks, metadata.Shape, metadata.Chunks)
		}
	}

	tensors := src.Tensors()
	nx, ny, nz := src.Size()[0], src.Size()[1], src.Size()[2]
	cx, cy, cz, cc := chunks[0], chunks[1], chunks[2], chunks[3]
	ncx, ncy, ncz, ncc := ceilDiv(nx, cx), ceilDiv(ny, cy), ceilDiv(nz, cz), ceilDiv(src.NComp(), cc)

	type job struct{ ix, iy, iz, ic int }
	jobs := make(chan job)
	errCh := make(chan error, 1)
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
			compressed, err := zstd.CompressLevel(nil, raw, metadata.Compressor.Level)
			if err == nil {
				key := fmt.Sprintf("%d.%d.%d.%d.%d", step, j.iz, j.iy, j.ix, j.ic)
				err = httpfs.Put(join(dir, key), compressed)
			}
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
		}
	}
	wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go worker()
	}
	for iz := 0; iz < ncz; iz++ {
		for iy := 0; iy < ncy; iy++ {
			for ix := 0; ix < ncx; ix++ {
				for ic := 0; ic < ncc; ic++ {
					jobs <- job{ix: ix, iy: iy, iz: iz, ic: ic}
				}
			}
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
	}
	return WriteMetadata(dir, metadata)
}

// ReadStep reconstructs a full data.Slice from all chunks for one time step.
func ReadStep(dir string, step int) (*data.Slice, error) {
	metadata, err := ReadMetadata(dir)
	if err != nil {
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
