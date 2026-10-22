package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mumax/3/tabular"
	"github.com/mumax/3/util"
)

// This command deliberately runs before flag parsing and CUDA initialization.
// It respects the committed prefix, including after an interrupted writer.
func runTableCommand(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: mumax3 table TABLE_DIRECTORY")
		panic(exitCode(2))
	}
	util.FatalErr(exportTable(args[0], os.Stdout))
}

func exportTable(dir string, out io.Writer) (result error) {
	r, err := tabular.Open(dir)
	if err != nil {
		return err
	}
	m := r.Metadata()
	w := bufio.NewWriterSize(out, 64*1024)
	defer func() { result = errors.Join(result, w.Flush()) }()
	for c, name := range m.Columns {
		prefix := "\t"
		if c == 0 {
			prefix = "# "
		}
		if _, err = fmt.Fprintf(w, "%s%s (%s)", prefix, name, m.Units[c]); err != nil {
			return err
		}
	}
	if _, err = fmt.Fprintln(w); err != nil {
		return err
	}
	return r.Walk(func(_ int, data [][]float64) error {
		for row := range data[0] {
			for c := range data {
				if c > 0 {
					if _, err := w.WriteString("\t"); err != nil {
						return err
					}
				}
				if _, err := fmt.Fprint(w, data[c][row]); err != nil {
					return err
				}
			}
			if _, err := w.WriteString("\n"); err != nil {
				return err
			}
		}
		return nil
	})
}
