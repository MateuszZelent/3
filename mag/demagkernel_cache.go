package mag

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/mumax/3/data"
	"github.com/mumax/3/util"
)

// .cache2 retains its original little-endian float32 component order.
// One bounded buffer avoids both per-float syscalls and a second full kernel copy.
const kernelCacheBlockBytes = 1 << 20

type kernelCacheProgress struct {
	label         string
	total, done   int64
	started, last time.Time
}

func newKernelCacheProgress(label string, total int64) *kernelCacheProgress {
	now := time.Now()
	util.Log(fmt.Sprintf("//Demag cache %s: %.1f MiB", label, float64(total)/(1<<20)))
	util.Progress(0, 100, "Demag cache "+label)
	return &kernelCacheProgress{label: label, total: total, started: now, last: now}
}
func (p *kernelCacheProgress) add(n int) {
	p.done += int64(n)
	if time.Since(p.last) < time.Second && p.done != p.total {
		return
	}
	p.last = time.Now()
	percent := int(100 * p.done / p.total)
	util.Progress(percent, 100, "Demag cache "+p.label)
	// Line logs remain useful with redirected output or a GUI progress callback.
	util.Log(fmt.Sprintf("//Demag cache %s: %d%% (%.1f / %.1f MiB), elapsed %s", p.label, percent, float64(p.done)/(1<<20), float64(p.total)/(1<<20), time.Since(p.started).Round(time.Millisecond)))
}
func kernelCacheBytes(size [3]int) (int64, error) {
	n := int64(4 * len(kernelComponents(size)))
	for _, dim := range size {
		if dim <= 0 || n > math.MaxInt64/int64(dim) {
			return 0, fmt.Errorf("invalid kernel cache dimensions: %v", size)
		}
		n *= int64(dim)
	}
	return n, nil
}

func saveKernelCache(filename string, kernel [3][3]*data.Slice) (retErr error) {
	if kernel[X][X] == nil {
		return fmt.Errorf("kernel component 00 is nil")
	}
	size := kernel[X][X].Size()
	total, err := kernelCacheBytes(size)
	if err != nil {
		return err
	}
	for _, c := range kernelComponents(size) {
		s := kernel[c[0]][c[1]]
		if s == nil || s.Size() != size || s.NComp() != 1 {
			return fmt.Errorf("invalid kernel component %d%d", c[0], c[1])
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(filename), ".kernel-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		if retErr != nil {
			_ = os.Remove(tmpName)
		}
	}()
	progress := newKernelCacheProgress("write", total)
	buffer := make([]byte, kernelCacheBlockBytes)
	for _, c := range kernelComponents(size) {
		values := kernel[c[0]][c[1]].Host()[0]
		for offset := 0; offset < len(values); {
			n := min(len(values)-offset, len(buffer)/4)
			block := buffer[:n*4]
			for i, v := range values[offset : offset+n] {
				binary.LittleEndian.PutUint32(block[i*4:], math.Float32bits(v))
			}
			written, err := tmp.Write(block)
			if err != nil {
				return err
			}
			if written != len(block) {
				return io.ErrShortWrite
			}
			progress.add(written)
			offset += n
		}
	}
	util.Log("//Demag cache: synchronizing file to storage", filename)
	if err := syncKernelCache(tmp); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filename); err != nil {
		return err
	}
	util.Log("//Demag cache write completed in", time.Since(progress.started).Round(time.Millisecond))
	return nil
}

func loadKernelCache(filename string, size [3]int) (kernel [3][3]*data.Slice, retErr error) {
	f, err := os.Open(filename)
	if err != nil {
		return kernel, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return kernel, err
	}
	want, err := kernelCacheBytes(size)
	if err != nil {
		return kernel, err
	}
	if info.Size() != want {
		return kernel, fmt.Errorf("invalid kernel cache size: got %d bytes, want %d", info.Size(), want)
	}
	// Release partially read components on failure; symmetry aliases are assigned only on success.
	defer func() {
		if retErr != nil {
			for _, c := range kernelComponents(size) {
				if kernel[c[0]][c[1]] != nil {
					kernel[c[0]][c[1]].Free()
				}
			}
			kernel = [3][3]*data.Slice{}
		}
	}()
	progress := newKernelCacheProgress("read", want)
	buffer := make([]byte, kernelCacheBlockBytes)
	for _, c := range kernelComponents(size) {
		slice := data.NewSlice(1, size)
		kernel[c[0]][c[1]] = slice
		values := slice.Host()[0]
		for offset := 0; offset < len(values); {
			n := min(len(values)-offset, len(buffer)/4)
			block := buffer[:n*4]
			if _, err := io.ReadFull(f, block); err != nil {
				return kernel, err
			}
			for i := 0; i < n; i++ {
				values[offset+i] = math.Float32frombits(binary.LittleEndian.Uint32(block[i*4:]))
			}
			progress.add(len(block))
			offset += n
		}
	}
	var extra [1]byte
	if n, err := f.Read(extra[:]); n != 0 || err != io.EOF {
		return kernel, fmt.Errorf("kernel cache changed while reading")
	}
	kernel[Y][X] = kernel[X][Y]
	kernel[Z][X] = kernel[X][Z]
	kernel[Z][Y] = kernel[Y][Z]
	util.Log("//Demag cache read completed in", time.Since(progress.started).Round(time.Millisecond))
	return kernel, nil
}

// A remote/HPC filesystem may spend a long time flushing data after the last
// write. Keep this stage visible separately from completed byte transfer.
func syncKernelCache(file *os.File) error {
	started := time.Now()
	util.Progress(0, 1, "Synchronizing demag cache")
	done, finished := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				util.Log("//Demag cache: waiting for storage synchronization, elapsed", time.Since(started).Round(time.Second))
			}
		}
	}()
	err := file.Sync()
	close(done)
	<-finished
	if err == nil {
		util.Progress(1, 1, "Synchronizing demag cache")
	}
	return err
}
