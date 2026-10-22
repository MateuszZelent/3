package hdf5

import (
	"errors"
	"github.com/mumax/3/httpfs"
	shdf5 "github.com/scigolib/hdf5"
)

// WriteTableChunk publishes an independently readable HDF5 file. Replacing a
// partial chunk never modifies the previous complete file in place.
func WriteTableChunk(filename string, columns, units []string, values [][]float64) error {
	return httpfs.ReplaceFile(filename, func(tmp string) (result error) {
		w, err := create(tmp)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, w.close()) }()
		for c, name := range columns {
			ds, err := w.file.CreateDataset("/"+name, shdf5.Float64, []uint64{uint64(len(values[c]))})
			if err != nil {
				return err
			}
			err = errors.Join(ds.Write(values[c]), ds.WriteAttribute("unit", units[c]), ds.Close())
			if err != nil {
				return err
			}
		}
		return nil
	})
}

func ReadTableChunk(filename string, columns []string) ([][]float64, error) {
	f, err := shdf5.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	values := make([][]float64, len(columns))
	for c, name := range columns {
		ds, err := findDataset(f, name)
		if err != nil {
			return nil, err
		}
		values[c], err = ds.Read()
		if err != nil {
			return nil, err
		}
	}
	return values, nil
}
