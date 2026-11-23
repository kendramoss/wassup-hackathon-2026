package usmca

import "testing"

func TestTriTables(t *testing.T) {
	vals := []Tri{False, Unknown3, True}
	and := [3][3]Tri{
		{False, False, False},
		{False, Unknown3, Unknown3},
		{False, Unknown3, True},
	}
	or := [3][3]Tri{
		{False, Unknown3, True},
		{Unknown3, Unknown3, True},
		{True, True, True},
	}
	for i, a := range vals {
		for j, b := range vals {
			if got := a.And(b); got != and[i][j] {
				t.Errorf("%v AND %v = %v want %v", a, b, got, and[i][j])
			}
			if got := a.Or(b); got != or[i][j] {
				t.Errorf("%v OR %v = %v want %v", a, b, got, or[i][j])
			}
		}
	}
}

func TestTriNotAndZeroValue(t *testing.T) {
	if True.Not() != False || False.Not() != True || Unknown3.Not() != Unknown3 {
		t.Error("Not")
	}
	var z Tri
	if z != Unknown3 {
		t.Error("zero value must be Unknown")
	}
}

func TestTriFolds(t *testing.T) {
	if AllOf() != True || AnyOf() != False {
		t.Error("empty folds")
	}
	if AnyOf(Unknown3, True, False) != True {
		t.Error("sufficient alternative must resolve")
	}
	if AllOf(Unknown3, False) != False {
		t.Error("false dominates AND")
	}
	if AnyOf(Unknown3, False) != Unknown3 || AllOf(True, Unknown3) != Unknown3 {
		t.Error("unknown propagation")
	}
}

func TestTriString(t *testing.T) {
	if True.String() != "true" || False.String() != "false" || Unknown3.String() != "unknown" || Tri(9).String() != "unknown" {
		t.Error("String")
	}
}
