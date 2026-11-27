package textilerules

// ShiftAllows applies the tariff-shift clause of an alternative to one
// foreign material: Yes when the material is classified where the clause
// allows a change, No when it is not (the same chapter or heading, or a
// listed exception), Unknown when the clause is not a plain shift or the
// code is too short. The country of origin does not come from this test
// (the process the alternative names decides it; see the package doc), but
// the clause is the regulation's and a test holds the encoding to the text.
func ShiftAllows(row Row, a Alt, goodCode, material string) Tri {
	if a.Req.Shift == nil {
		return Unknown
	}
	g, m := Digits(goodCode), Digits(material)
	if len(g) < 4 || len(m) < 4 {
		return Unknown
	}
	for _, e := range a.Req.Shift.Except {
		if len(e) > 8 && e[:8] == "Chapter " {
			if m[:2] == pad2(e[8:]) {
				return No
			}
			continue
		}
		lo, hi, ok := rangeOf(e)
		if !ok {
			continue
		}
		switch relate(m, lo, hi) {
		case inside:
			return No
		case partial:
			return Unknown
		}
	}
	switch a.Req.Shift.From {
	case "any other chapter":
		return boolTri(m[:2] != g[:2])
	case "any other heading":
		return boolTri(m[:4] != g[:4])
	case "any heading outside that group":
		switch relate(m, row.Lo, row.Hi) {
		case inside:
			return No
		case partial:
			return Unknown
		}
		return Yes
	case "any heading":
		return Yes
	case "any other subheading":
		if len(m) < 6 || len(g) < 6 {
			return Unknown
		}
		return boolTri(m[:6] != g[:6])
	}
	return Unknown
}

func pad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func boolTri(b bool) Tri {
	if b {
		return Yes
	}
	return No
}
