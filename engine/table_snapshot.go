package engine

import (
	"strconv"
	"strings"

	"github.com/mumax/3/httpfs"
)

// OVF deliberately keeps the legacy float32 text representation. Full explicit
// snapshots are loaded from that file; the live UI never reads full history.
func readTextTable(columns []string) (map[string][]float64, error) {
	if err := Table.Flush(); err != nil {
		return nil, err
	}
	b, err := httpfs.Read(OD() + Table.name + ".txt")
	if err != nil {
		return nil, err
	}
	values := make(map[string][]float64, len(columns))
	for _, c := range columns {
		values[c] = []float64{}
	}
	body := string(b)
	last := strings.LastIndexByte(body, '\n')
	if last < 0 {
		return values, nil
	}
	for _, line := range strings.Split(body[:last], "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != len(columns) {
			continue
		}
		row := make([]float64, len(fields))
		valid := true
		for c, s := range fields {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				valid = false
				break
			}
			row[c] = v
		}
		if valid {
			for c, name := range columns {
				values[name] = append(values[name], row[c])
			}
		}
	}
	return values, nil
}
