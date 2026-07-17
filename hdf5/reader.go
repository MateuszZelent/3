package hdf5

import (
	"fmt"
	"strings"

	"github.com/mumax/3/data"
	shdf5 "github.com/scigolib/hdf5"
)

func ReadArray(filename, datasetPath string) (*data.Slice, error) {
	file, err := shdf5.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("hdf5: open %s: %w", filename, err)
	}
	defer file.Close()
	dataset, err := findDataset(file, datasetPath)
	if err != nil {
		return nil, err
	}
	flat, err := dataset.Read()
	if err != nil {
		return nil, err
	}
	info, err := dataset.Info()
	if err != nil {
		return nil, err
	}
	dims, err := parseDims(info)
	if err != nil {
		return nil, err
	}
	if len(dims) != 4 {
		return nil, fmt.Errorf("hdf5: expected a 4D array, got %d dimensions", len(dims))
	}
	nz, ny, nx, ncomp := dims[0], dims[1], dims[2], dims[3]
	if len(flat) != nz*ny*nx*ncomp {
		return nil, fmt.Errorf("hdf5: got %d values, expected %d", len(flat), nz*ny*nx*ncomp)
	}
	dst := data.NewSlice(ncomp, [3]int{nx, ny, nz})
	tensors := dst.Tensors()
	position := 0
	for z := 0; z < nz; z++ {
		for y := 0; y < ny; y++ {
			for x := 0; x < nx; x++ {
				for c := 0; c < ncomp; c++ {
					tensors[c][z][y][x] = float32(flat[position])
					position++
				}
			}
		}
	}
	return dst, nil
}

func findDataset(file *shdf5.File, datasetPath string) (*shdf5.Dataset, error) {
	wanted := strings.Trim(datasetPath, "/")
	var found *shdf5.Dataset
	file.Walk(func(path string, object shdf5.Object) {
		if strings.Trim(path, "/") == wanted {
			found, _ = object.(*shdf5.Dataset)
		}
	})
	if found == nil {
		return nil, fmt.Errorf("hdf5: dataset %q not found", datasetPath)
	}
	return found, nil
}

func parseDims(info string) ([]int, error) {
	markers := []string{"array [", "dims=[", "Dimensions: ["}
	start := -1
	for _, marker := range markers {
		if index := strings.Index(info, marker); index >= 0 {
			start = index + len(marker)
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("hdf5: dimensions missing from %q", info)
	}
	end := strings.Index(info[start:], "]")
	if end < 0 {
		return nil, fmt.Errorf("hdf5: malformed dimensions in %q", info)
	}
	fields := strings.Fields(info[start : start+end])
	dims := make([]int, len(fields))
	for i, field := range fields {
		field = strings.TrimRight(field, ",x×")
		if _, err := fmt.Sscanf(field, "%d", &dims[i]); err != nil {
			return nil, err
		}
	}
	return dims, nil
}
