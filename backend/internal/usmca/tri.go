package usmca

// Tri is a Kleene three-valued truth value. The zero value is Unknown3, so an
// unset condition never reads as a definite result.
type Tri uint8

// Tri values. Unknown3 is the zero value.
const (
	Unknown3 Tri = iota
	False
	True
)

// String returns "true", "false" or "unknown".
func (t Tri) String() string {
	switch t {
	case True:
		return "true"
	case False:
		return "false"
	default:
		return "unknown"
	}
}

// Not is Kleene negation; Unknown3 stays Unknown3.
func (t Tri) Not() Tri {
	switch t {
	case True:
		return False
	case False:
		return True
	default:
		return Unknown3
	}
}

// And is Kleene conjunction: False dominates, then Unknown3, then True.
func (t Tri) And(o Tri) Tri {
	if t == False || o == False {
		return False
	}
	if t == True && o == True {
		return True
	}
	return Unknown3
}

// Or is Kleene disjunction: True dominates, then Unknown3, then False.
func (t Tri) Or(o Tri) Tri {
	if t == True || o == True {
		return True
	}
	if t == False && o == False {
		return False
	}
	return Unknown3
}

// AllOf folds And over ts; AllOf() is True.
func AllOf(ts ...Tri) Tri {
	out := True
	for _, t := range ts {
		out = out.And(t)
	}
	return out
}

// AnyOf folds Or over ts; AnyOf() is False.
func AnyOf(ts ...Tri) Tri {
	out := False
	for _, t := range ts {
		out = out.Or(t)
	}
	return out
}
