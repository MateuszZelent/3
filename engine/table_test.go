package engine

import (
	"fmt"
	"strings"
	"testing"
)

func resetTableWithDefaultMagnetization(t *testing.T) {
	t.Helper()
	previousOutput, previousInfo := Table.output, Table.info
	previousOutputs, previousAutosave := Table.outputs, Table.autosave
	previousClosed := Table.closed
	t.Cleanup(func() {
		Table.output = previousOutput
		Table.info = previousInfo
		Table.outputs = previousOutputs
		Table.autosave = previousAutosave
		Table.closed = previousClosed
	})
	Table.output = nil
	Table.info = info{name: "table"}
	Table.outputs = nil
	Table.autosave = autosave{}
	Table.closed = false
	Table.Add(&M)
}

func assertTablePanicContains(t *testing.T, want string, action func()) {
	t.Helper()
	defer func() {
		got := recover()
		if got == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if message := fmt.Sprint(got); !strings.Contains(message, want) {
			t.Fatalf("panic = %q, want text containing %q", message, want)
		}
	}()
	action()
}

func TestTableAddDoesNotReaddDefaultMagnetization(t *testing.T) {
	resetTableWithDefaultMagnetization(t)

	TableAdd(&M)

	if len(Table.outputs) != 1 || Table.outputs[0] != &M {
		t.Fatalf("TableAdd(m) changed the default table schema: %#v", Table.outputs)
	}
}

func TestTableAddStillRejectsSameNameDifferentQuantity(t *testing.T) {
	resetTableWithDefaultMagnetization(t)

	assertTablePanicContains(t, "invalid or duplicate table column: mx", func() {
		TableAdd(&namedTableQuantity{Quantity: &M, name: "m"})
	})
	if len(Table.outputs) != 1 {
		t.Fatalf("rejected duplicate changed the table schema: %#v", Table.outputs)
	}
}

type tableTestWriter struct{}

func (tableTestWriter) Write(p []byte) (int, error) { return len(p), nil }
func (tableTestWriter) Flush() error                { return nil }
func (tableTestWriter) Close() error                { return nil }

func TestTableAddStillRejectsSchemaChangesAfterFirstRow(t *testing.T) {
	resetTableWithDefaultMagnetization(t)
	Table.output = tableTestWriter{}

	assertTablePanicContains(t, "add quantities before the first table row", func() {
		TableAddAs(&M, "m_late")
	})
	if len(Table.outputs) != 1 {
		t.Fatalf("rejected late column changed the table schema: %#v", Table.outputs)
	}
}
