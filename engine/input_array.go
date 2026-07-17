package engine

import (
	"strconv"
	"strings"

	"github.com/mumax/3/httpfs"
)

func init() {
	DeclFunc("readArrayFromFile", ReadArrayFromFile, "Read a comma-separated float array from a file")
	DeclFunc("readArrayFromString", ReadArrayFromString, "Parse a comma-separated float array")
}

type InputArray struct {
	data []float64
}

func (array InputArray) Get(index int) float64 {
	if index < 0 || index >= len(array.data) {
		panic(UserErr("InputArray.Get: index out of bounds"))
	}
	return array.data[index]
}

func (array InputArray) Len() int { return len(array.data) }

func ReadArrayFromFile(filename string) InputArray {
	contents, err := httpfs.Read(filename)
	if err != nil {
		panic(UserErr("readArrayFromFile: " + err.Error()))
	}
	return ReadArrayFromString(string(contents))
}

func ReadArrayFromString(input string) InputArray {
	input = strings.TrimSpace(input)
	if input == "" {
		return InputArray{}
	}
	parts := strings.Split(input, ",")
	values := make([]float64, len(parts))
	for index, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			panic(UserErr("readArrayFromString: " + err.Error()))
		}
		values[index] = value
	}
	return InputArray{data: values}
}
