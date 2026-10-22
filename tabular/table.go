// Package tabular stores scalar time series as bounded chunks. manifest.json
// publishes the common committed prefix; readers must not infer it from a
// single column's shape during a concurrent write or after interrupted output.
package tabular

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"path"
	"strings"

	"github.com/DataDog/zstd"
	"github.com/mumax/3/hdf5"
	"github.com/mumax/3/httpfs"
	"github.com/mumax/3/zarr"
)

const ChunkRows = 1024

type Manifest struct {
	Version   int      `json:"version"`
	Format    string   `json:"format"`
	Columns   []string `json:"columns"`
	Units     []string `json:"units"`
	Rows      int      `json:"committed_rows"`
	ChunkRows int      `json:"chunk_rows"`
}

type Writer struct {
	dir      string
	manifest Manifest
	tail     [][]float64
	put      func(string, []byte) error
}

func New(dir, format string, columns, units []string) (*Writer, error) {
	m := Manifest{1, format, append([]string(nil), columns...), append([]string(nil), units...), 0, ChunkRows}
	if err := m.validate(); err != nil {
		return nil, err
	}
	if format == "hdf5" && strings.HasPrefix(dir, "http://") {
		return nil, errors.New("HDF5 requires local output")
	}
	if err := httpfs.Mkdir(dir); err != nil {
		if _, readErr := httpfs.ReadDir(dir); readErr != nil {
			return nil, err
		}
	}
	entries, err := httpfs.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, name := range entries {
		if name != ".zgroup" {
			return nil, fmt.Errorf("table directory is not empty: %s", dir)
		}
	}
	if format == "zarr" {
		if err := zarr.WriteGroup(dir); err != nil {
			return nil, err
		}
	}
	w := &Writer{dir: dir, manifest: m, tail: make([][]float64, len(columns)), put: httpfs.PutAtomic}
	if err := w.publish(0); err != nil {
		return nil, err
	}
	return w, nil
}

func (m Manifest) validate() error {
	if m.Version != 1 || (m.Format != "zarr" && m.Format != "hdf5") || m.ChunkRows != ChunkRows || m.Rows < 0 || len(m.Columns) == 0 || len(m.Units) != len(m.Columns) {
		return errors.New("invalid table manifest")
	}
	seen := map[string]bool{}
	for _, unit := range m.Units {
		if strings.IndexFunc(unit, func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			return errors.New("invalid control character in table unit")
		}
	}
	for _, c := range m.Columns {
		if c == "" || c == "." || c == ".." || strings.IndexFunc(c, func(r rune) bool { return r == '/' || r == 92 || r < 32 || r == 127 }) >= 0 || strings.HasPrefix(c, ".") || c == "manifest.json" || seen[c] {
			return fmt.Errorf("invalid or duplicate table column %q", c)
		}
		seen[c] = true
	}
	return nil
}

func (w *Writer) Rows() int { return w.manifest.Rows }

func (w *Writer) publish(rows int) error {
	m := w.manifest
	m.Rows = rows
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return w.put(path.Join(w.dir, "manifest.json"), b)
}

// Append publishes whole rows across every column. Partial chunks are padded
// and replaced atomically, preserving every previously committed value. Memory
// and recompression are bounded by one chunk plus the supplied batch.
func (w *Writer) Append(values [][]float64) error {
	if len(values) != len(w.manifest.Columns) {
		return errors.New("table column count changed")
	}
	n := len(values[0])
	for _, v := range values {
		if len(v) != n {
			return errors.New("unaligned table rows")
		}
	}
	if n == 0 {
		return nil
	}
	tail := make([][]float64, len(values))
	for c := range tail {
		tail[c] = append([]float64(nil), w.tail[c]...)
	}
	rows := w.manifest.Rows
	for start := 0; start < n; {
		part := min(n-start, ChunkRows-rows%ChunkRows)
		for c := range values {
			tail[c] = append(tail[c], values[c][start:start+part]...)
		}
		index := rows / ChunkRows
		if err := w.writeChunk(index, tail); err != nil {
			return err
		}
		rows += part
		start += part
		if rows%ChunkRows == 0 {
			for c := range tail {
				tail[c] = nil
			}
		}
	}
	if w.manifest.Format == "zarr" {
		for c, name := range w.manifest.Columns {
			meta := zarr.SeriesMetadata{Chunks: [1]int{ChunkRows}, Compressor: &zarr.Compressor{ID: "zstd", Level: 1}, Dtype: "<f8", Order: "C", Shape: [1]int{rows}, ZarrFormat: 2}
			b, err := json.Marshal(meta)
			if err != nil {
				return err
			}
			if err = w.put(path.Join(w.dir, name, ".zarray"), b); err != nil {
				return err
			}
			attrs, _ := json.Marshal(map[string]string{"name": name, "unit": w.manifest.Units[c]})
			if err = w.put(path.Join(w.dir, name, ".zattrs"), attrs); err != nil {
				return err
			}
		}
	}
	if err := w.publish(rows); err != nil {
		return err
	}
	w.manifest.Rows = rows
	w.tail = tail
	return nil
}

func (w *Writer) writeChunk(index int, values [][]float64) error {
	if w.manifest.Format == "hdf5" {
		padded := make([][]float64, len(values))
		for c := range padded {
			padded[c] = make([]float64, ChunkRows)
			copy(padded[c], values[c])
		}
		return hdf5.WriteTableChunk(path.Join(w.dir, fmt.Sprintf("%012d.h5", index)), w.manifest.Columns, w.manifest.Units, padded)
	}
	for c, name := range w.manifest.Columns {
		raw := make([]byte, ChunkRows*8)
		for i, v := range values[c] {
			binary.LittleEndian.PutUint64(raw[8*i:], math.Float64bits(v))
		}
		compressed, err := zstd.CompressLevel(nil, raw, 1)
		if err != nil {
			return err
		}
		if err = w.put(path.Join(w.dir, name, fmt.Sprint(index)), compressed); err != nil {
			return err
		}
	}
	return nil
}

// Reader holds one immutable manifest snapshot. Atomic replacement of a partial
// chunk preserves its old prefix while later rows are being appended.
type Reader struct {
	dir      string
	manifest Manifest
}

func Open(dir string) (*Reader, error) {
	var m Manifest
	b, err := httpfs.Read(path.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if err = m.validate(); err != nil {
		return nil, err
	}
	return &Reader{dir: dir, manifest: m}, nil
}

func (r *Reader) Metadata() Manifest {
	m := r.manifest
	m.Columns = append([]string(nil), m.Columns...)
	m.Units = append([]string(nil), m.Units...)
	return m
}

// Walk streams committed, aligned row batches with bounded memory. The caller
// may retain a batch, but doing so is not required to export a long simulation.
func (r *Reader) Walk(visit func(start int, values [][]float64) error) error {
	m := r.manifest
	for index, start := 0, 0; start < m.Rows; index, start = index+1, start+ChunkRows {
		n := min(ChunkRows, m.Rows-start)
		values := make([][]float64, len(m.Columns))
		if m.Format == "hdf5" {
			chunk, err := hdf5.ReadTableChunk(path.Join(r.dir, fmt.Sprintf("%012d.h5", index)), m.Columns)
			if err != nil {
				return err
			}
			for c := range values {
				if len(chunk[c]) != ChunkRows {
					return errors.New("invalid HDF5 table chunk")
				}
				values[c] = chunk[c][:n]
			}
		} else {
			for c, name := range m.Columns {
				compressed, err := httpfs.Read(path.Join(r.dir, name, fmt.Sprint(index)))
				if err != nil {
					return err
				}
				raw, err := zstd.Decompress(nil, compressed)
				if err != nil {
					return err
				}
				if len(raw) != ChunkRows*8 {
					return errors.New("invalid Zarr table chunk")
				}
				values[c] = make([]float64, n)
				for i := 0; i < n; i++ {
					values[c][i] = math.Float64frombits(binary.LittleEndian.Uint64(raw[i*8:]))
				}
			}
		}
		if err := visit(start, values); err != nil {
			return err
		}
	}
	return nil
}

// Read explicitly loads the complete committed prefix. Use Open and Walk when
// processing an arbitrarily long table with bounded memory.
func Read(dir string) (Manifest, [][]float64, error) {
	r, err := Open(dir)
	if err != nil {
		return Manifest{}, nil, err
	}
	m := r.Metadata()
	values := make([][]float64, len(m.Columns))
	for c := range values {
		values[c] = make([]float64, 0, min(m.Rows, ChunkRows))
	}
	err = r.Walk(func(_ int, batch [][]float64) error {
		for c := range values {
			values[c] = append(values[c], batch[c]...)
		}
		return nil
	})
	if err != nil {
		return m, nil, err
	}
	return m, values, nil
}
