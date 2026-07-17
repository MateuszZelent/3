// Package hdf5 provides pure-Go HDF5 storage for mumax3 arrays and tables.
package hdf5

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	shdf5 "github.com/scigolib/hdf5"
)

type writer struct {
	file     *shdf5.FileWriter
	filename string
}

func create(filename string) (*writer, error) {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return nil, fmt.Errorf("hdf5: mkdir: %w", err)
	}
	file, err := shdf5.CreateForWrite(filename, shdf5.CreateTruncate)
	if err != nil {
		return nil, fmt.Errorf("hdf5: create %s: %w", filename, err)
	}
	return &writer{file: file, filename: filename}, nil
}

func (w *writer) close() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *writer) writeArray(path string, tensors [][][][]float32, size [3]int, ncomp int) error {
	if w.file == nil {
		return fmt.Errorf("hdf5: %s is closed", w.filename)
	}
	nx, ny, nz := size[0], size[1], size[2]
	flat := make([]float32, nx*ny*nz*ncomp)
	position := 0
	for z := 0; z < nz; z++ {
		for y := 0; y < ny; y++ {
			for x := 0; x < nx; x++ {
				for c := 0; c < ncomp; c++ {
					flat[position] = tensors[c][z][y][x]
					position++
				}
			}
		}
	}
	dataset, err := w.file.CreateDataset(path, shdf5.Float32, []uint64{uint64(nz), uint64(ny), uint64(nx), uint64(ncomp)})
	if err != nil {
		return fmt.Errorf("hdf5: create dataset %s: %w", path, err)
	}
	if err := dataset.Write(flat); err != nil {
		_ = dataset.Close()
		return fmt.Errorf("hdf5: write dataset %s: %w", path, err)
	}
	return dataset.Close()
}

func (w *writer) writeFloat64(path string, values []float64) error {
	dataset, err := w.file.CreateDataset(path, shdf5.Float64, []uint64{uint64(len(values))})
	if err != nil {
		return fmt.Errorf("hdf5: create dataset %s: %w", path, err)
	}
	if err := dataset.Write(values); err != nil {
		_ = dataset.Close()
		return fmt.Errorf("hdf5: write dataset %s: %w", path, err)
	}
	return dataset.Close()
}

// MultiWriter keeps one HDF5 file per quantity, matching Amumax's layout.
type MultiWriter struct {
	mu      sync.Mutex
	baseDir string
	writers map[string]*writer
	closed  bool
}

func NewMultiWriter(baseDir string) (*MultiWriter, error) {
	if len(baseDir) >= 7 && (baseDir[:7] == "http://" || (len(baseDir) >= 8 && baseDir[:8] == "https://")) {
		return nil, errors.New("hdf5: remote output directories are unsupported")
	}
	return &MultiWriter{baseDir: baseDir, writers: make(map[string]*writer)}, nil
}

func (mw *MultiWriter) get(name string) (*writer, error) {
	if mw.closed {
		return nil, errors.New("hdf5: writer is closed")
	}
	if existing := mw.writers[name]; existing != nil {
		return existing, nil
	}
	created, err := create(filepath.Join(mw.baseDir, name, name+".h5"))
	if err != nil {
		return nil, err
	}
	mw.writers[name] = created
	return created, nil
}

func (mw *MultiWriter) SaveArray(name string, step int, tensors [][][][]float32, size [3]int, ncomp int) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	w, err := mw.get(name)
	if err != nil {
		return err
	}
	return w.writeArray(fmt.Sprintf("/%d", step), tensors, size, ncomp)
}

func (mw *MultiWriter) SaveTimestamps(name string, times []float64) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	w, err := mw.get(name)
	if err != nil {
		return err
	}
	return w.writeFloat64("/t", times)
}

func (mw *MultiWriter) SaveTableColumn(name string, values []float64) error {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	w, err := mw.get("table")
	if err != nil {
		return err
	}
	return w.writeFloat64("/"+name, values)
}

func (mw *MultiWriter) Close() error {
	mw.mu.Lock()
	defer mw.mu.Unlock()
	if mw.closed {
		return nil
	}
	mw.closed = true
	var result error
	for name, w := range mw.writers {
		if err := w.close(); err != nil {
			result = errors.Join(result, fmt.Errorf("hdf5: close %s: %w", name, err))
		}
	}
	return result
}
