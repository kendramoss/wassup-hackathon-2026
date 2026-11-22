package tariffshift

import "testing"

func TestRowsFor(t *testing.T) {
	r, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	one := r.RowsFor("8414.30.80")
	if len(one) != 1 || one[0].ID != "19 CFR 102.20(o), 8414.10-8414.80" {
		t.Fatalf("8414.30.80: %+v", one)
	}
	if got := r.RowsFor("8414"); len(got) != 2 {
		t.Fatalf("heading 8414: want both rows, got %d", len(got))
	}
	if got := r.RowsFor("6306.22"); got != nil {
		t.Fatalf("chapter 63 is not encoded: %+v", got)
	}
	if got := r.RowsFor("2024"); got != nil {
		t.Fatalf("chapter 20 is not encoded: %+v", got)
	}
	if got := r.RowsFor("84"); got != nil {
		t.Fatalf("a chapter alone quotes no row: %+v", got)
	}
}
