package engine

import (
	"fmt"
	"github.com/mumax/3/cuda"
	"github.com/mumax/3/data"
	"github.com/mumax/3/httpfs"
	"github.com/mumax/3/script"
	"github.com/mumax/3/timer"
	"io"
	"strings"
	"sync"
)

var Table = *newTable("table") // output handle for tabular data (average magnetization etc.)
const TableAutoflushRate = 1   // wall-clock seconds between buffered output checkpoints

func init() {
	DeclFunc("TableAdd", TableAdd, "Add quantity as a column to the data table.")
	DeclFunc("TableAddAs", TableAddAs, "Add a quantity to the data table under a custom column name.")
	DeclFunc("TableAddVar", TableAddVariable, "Add user-defined variable + name + unit to data table.")
	DeclFunc("TableSave", TableSave, "Save the data table right now (appends one line).")
	DeclFunc("TableAutoSave", TableAutoSave, "Auto-save the data table every period (s). Zero disables save.")
	DeclFunc("TablePrint", TablePrint, "Print anyting in the data table")
	Table.Add(&M)
}

type DataTable struct {
	output interface {
		io.Writer
		Flush() error
		Close() error
	}
	info
	outputs []Quantity
	autosave
	flushlock sync.Mutex
	closed    bool
}

func (t *DataTable) Write(p []byte) (int, error) {
	n, err := t.output.Write(p)
	CheckRecoverable(err)
	return n, err
}

func (t *DataTable) Flush() error {
	t.flushlock.Lock()
	defer t.flushlock.Unlock()
	return t.flushLocked()
}

func (t *DataTable) flushLocked() error {
	if t.output == nil || t.closed {
		return nil
	}
	return t.output.Flush()
}

func (t *DataTable) close() error {
	t.flushlock.Lock()
	defer t.flushlock.Unlock()
	if t.output == nil || t.closed {
		return nil
	}
	t.closed = true
	return t.output.Close()
}

func newTable(name string) *DataTable {
	t := new(DataTable)
	t.name = name
	return t
}

func TableAdd(col Quantity) {
	name := NameOf(col)
	if name == NameOf(CorePos) || name == NameOf(SkyrmionPos) || name == NameOf(CoreValid) || name == NameOf(SkyrmionValid) || name == NameOf(SkyrmionPolarization) {
		for _, output := range Table.outputs {
			if NameOf(output) == name {
				return
			}
		}

	}

	Table.Add(col)
	if name == NameOf(CorePos) {
		setCoreMode("vortex")
	}
	if name == NameOf(SkyrmionPos) {
		setCoreMode("skyrmion")
	}
}

func TableAddAs(col Quantity, name string) {
	if name == "" {
		panic(UserErr("TableAddAs: name must not be empty"))
	}
	Table.Add(&namedTableQuantity{Quantity: col, name: name})
}

type namedTableQuantity struct {
	Quantity
	name string
}

func (q *namedTableQuantity) Name() string       { return q.name }
func (q *namedTableQuantity) Unit() string       { return UnitOf(q.Quantity) }
func (q *namedTableQuantity) average() []float64 { return AverageOf(q.Quantity) }

func TableAddVariable(x script.ScalarFunction, name, unit string) {
	Table.AddVariable(x, name, unit)
}

func (t *DataTable) AddVariable(x script.ScalarFunction, name, unit string) {
	TableAdd(&userVar{x, name, unit})
}

type userVar struct {
	value      script.ScalarFunction
	name, unit string
}

func (x *userVar) Name() string       { return x.name }
func (x *userVar) NComp() int         { return 1 }
func (x *userVar) Unit() string       { return x.unit }
func (x *userVar) average() []float64 { return []float64{x.value.Float()} }
func (x *userVar) EvalTo(dst *data.Slice) {
	avg := x.average()
	for c := 0; c < x.NComp(); c++ {
		cuda.Memset(dst.Comp(c), float32(avg[c]))
	}
}

func TableSave() {
	Table.Save()
}

func TableAutoSave(period float64) {
	Table.autosave = autosave{period, Time, -1, nil} // count -1 allows output on t=0
}

func (t *DataTable) Add(output Quantity) {
	if t.inited() {
		panic(UserErr(fmt.Sprint("data table add ", NameOf(output), ": add quantities before the first table row")))
	}
	columns := map[string]bool{"t": true}
	for _, q := range append(append([]Quantity(nil), t.outputs...), output) {
		if strings.IndexFunc(UnitOf(q), func(r rune) bool { return r < 32 || r == 127 }) >= 0 {
			panic(UserErr("invalid table unit for " + NameOf(q)))
		}
		for c := 0; c < q.NComp(); c++ {
			name := NameOf(q)
			if q.NComp() > 1 {
				name += compname[c]
			}
			if name == "" || strings.IndexFunc(name, func(r rune) bool { return r == '/' || r == 92 || r < 32 || r == 127 }) >= 0 || strings.HasPrefix(name, ".") || columns[name] {
				panic(UserErr("invalid or duplicate table column: " + name))
			}
			columns[name] = true
		}
	}
	t.outputs = append(t.outputs, output)
}

func (t *DataTable) Save() {
	t.flushlock.Lock() // flush during write gives errShortWrite
	defer t.flushlock.Unlock()

	if cuda.Synchronous {
		timer.Start("io")
	}
	if t.closed {
		panic(UserErr("table is closed"))
	}
	t.init()
	columns := []string{"t"}
	units := []string{"s"}
	values := []float64{Time}
	beginCoreSample()
	defer endCoreSample()
	var row strings.Builder
	fmt.Fprint(&row, Time)
	for _, o := range t.outputs {
		vec := AverageOf(o)
		for component, v := range vec {
			fmt.Fprint(&row, "\t", float32(v))
			name := NameOf(o)
			if o.NComp() > 1 {
				name += compname[component]
			}
			columns = append(columns, name)
			units = append(units, tableComponentUnit(o, component))
			values = append(values, v)
		}
	}
	fmt.Fprintln(&row)
	fprint(t, row.String())
	recordTableHistory(columns, units, values)
	if t.count <= 0 || tableHistory.needsFlush() {
		CheckRecoverable(t.flushLocked())
		CheckRecoverable(tableHistory.flushStructured())
	}
	t.count++

	if cuda.Synchronous {
		timer.Stop("io")
	}
}

func (t *DataTable) Println(msg ...interface{}) {
	t.flushlock.Lock()
	defer t.flushlock.Unlock()
	if t.closed {
		panic(UserErr("table is closed"))
	}
	t.init()
	fprintln(t, msg...)
}

func TablePrint(msg ...interface{}) {
	Table.Println(msg...)
}

// open writer and write header
func (t *DataTable) init() {
	if t.inited() {
		return
	}
	f, err := httpfs.Create(OD() + t.name + ".txt")
	CheckRecoverable(err)
	t.output = f

	// write header
	fprint(t, "# t (s)")
	for _, o := range t.outputs {
		if o.NComp() == 1 {
			fprint(t, "\t", NameOf(o), " (", UnitOf(o), ")")
		} else {
			for c := 0; c < o.NComp(); c++ {
				fprint(t, "\t", NameOf(o)+string('x'+c), " (", tableComponentUnit(o, c), ")")
			}
		}
	}
	fprintln(t)
	CheckRecoverable(t.flushLocked())
}

func (t *DataTable) inited() bool {
	return t.output != nil
}

func (t *DataTable) flush() { CheckRecoverable(t.Flush()) }

// Safe fmt.Fprint, will fail on error
func fprint(out io.Writer, x ...interface{}) {
	_, err := fmt.Fprint(out, x...)
	CheckRecoverable(err)
}

// Safe fmt.Fprintln, will fail on error
func fprintln(out io.Writer, x ...interface{}) {
	_, err := fmt.Fprintln(out, x...)
	CheckRecoverable(err)

}
