package script

import "testing"

func TestStringEquality(t *testing.T) {
	w := NewWorld()
	w.MustExec(`point_source := "sinc"`)
	for src, want := range map[string]bool{
		`point_source == "sinc"`:  true,
		`point_source == "gauss"`: false,
		`point_source != "point"`: true,
		`point_source != "sinc"`:  false,
		`1 == 1.0`:                true,
		`1 != 2`:                  true,
	} {
		if got := w.MustEval(src); got != want {
			t.Errorf("%s = %v, want %v", src, got, want)
		}
	}
	w.MustExec(`if point_source == "sinc" { point_source = "gauss" }`)
	if got := w.MustEval(`point_source == "gauss"`); got != true {
		t.Fatal(got)
	}
}
