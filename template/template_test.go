package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateCartesianAndFlatTemplates(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "sweep.mx3")
	contents := "A = \"{prefix=a_;array=[1,2];format=%02.0f}\"\nname := \"{prefix=m_;array=['x','y'];format=%s}\"\n"
	if err := os.WriteFile(source, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	files, err := Generate(source, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("generated %d files, want 4", len(files))
	}
	if files[0] != filepath.Join(dir, "a_01", "m_x.mx3") {
		t.Fatalf("first path = %q", files[0])
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if text := string(data); !strings.Contains(text, "A = 1") || !strings.Contains(text, `name := x`) {
		t.Fatalf("generated content = %q", text)
	}
	flat, err := Generate(source, Options{Flat: true})
	if err != nil {
		t.Fatal(err)
	}
	if flat[3] != filepath.Join(dir, "a_02m_y.mx3") {
		t.Fatalf("flat path = %q", flat[3])
	}
}

func TestGenerateRejectsInvalidTemplate(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bad.mx3")
	if err := os.WriteFile(file, []byte(`x := "{start=0;end=1;step=0}"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(file, Options{}); err == nil {
		t.Fatal("zero step was accepted")
	}
}
