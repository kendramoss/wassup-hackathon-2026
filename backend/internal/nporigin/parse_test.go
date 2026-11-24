package nporigin

import (
	"reflect"
	"testing"
)

func TestParseCode(t *testing.T) {
	for _, c := range []struct {
		code, key, split string
		ex               bool
		applies          []Range
	}{
		{"8527", "8527", "", false, []Range{{Lo: "8527", Hi: "8527"}}},
		{"ex 8473", "8473x", "", true, []Range{{Lo: "8473", Hi: "8473"}}},
		{"0901 11", "090111", "", false, []Range{{Lo: "090111", Hi: "090111"}}},
		{"ex 8202 39 (a)", "820239xa", "a", true, []Range{{Lo: "820239", Hi: "820239"}}},
		{"ex 04 08", "0408x", "", true, []Range{{Lo: "0408", Hi: "0408"}}},
		{"Ex 9021.10", "902110x", "", true, []Range{{Lo: "902110", Hi: "902110"}}},
		{"ex 6911 to ex 6913", "6911-6913x", "", true, []Range{{Lo: "6911", Hi: "6913"}}},
		{"ex 9401 and ex 9403", "9401-9403x", "", true, []Range{{Lo: "9401", Hi: "9401"}, {Lo: "9403", Hi: "9403"}}},
	} {
		applies, ex, split, key, err := ParseCode(c.code)
		if err != nil {
			t.Fatalf("%q: %v", c.code, err)
		}
		if key != c.key || ex != c.ex || split != c.split || !reflect.DeepEqual(applies, c.applies) {
			t.Errorf("%q: got %v %v %q %q", c.code, applies, ex, split, key)
		}
	}
	for _, bad := range []string{"", "84", "ex 847", "8473 1", "ex 8475 to ex 8473"} {
		if _, _, _, _, err := ParseCode(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}

func TestSplitAlternatives(t *testing.T) {
	for _, c := range []struct {
		text string
		want []string
	}{
		{"CTH", []string{"CTH"}},
		{"CTH or Assembly of semi-conductor products", []string{"CTH", "Assembly of semi-conductor products"}},
		{"CTH, except from heading 8538 ; or Assembly of semi-conductor products", []string{"CTH, except from heading 8538", "Assembly of semi-conductor products"}},
		{"Manufacture from yarn\nOr\nPrinting or dyeing of unbleached fabrics", []string{"Manufacture from yarn", "Printing or dyeing of unbleached fabrics"}},
		// An "or" in a list of headings is not a split.
		{"CTH, except from heading 7225 or 7226", []string{"CTH, except from heading 7225 or 7226"}},
		{"CTH, or change from hollow profiles of subheading 7304 19", []string{"CTH", "change from hollow profiles of subheading 7304 19"}},
	} {
		if got := SplitAlternatives(c.text); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %q", c.text, got)
		}
	}
}

func TestParseAlternative(t *testing.T) {
	a := ParseAlternative("CTH, except from heading 7208 , 7209 or 7212 , and except from heading 7213 , 7214 or 7215 when this change results from cutting or bending.")
	if !a.Encoded || a.Kind != KindChange || a.Level != 4 || len(a.Except) != 2 {
		t.Fatalf("got %+v", a)
	}
	if len(a.Except[0].From) != 3 || a.Except[0].Condition != "" || a.Except[1].Condition != "cutting or bending" || len(a.Except[1].From) != 3 {
		t.Errorf("exceptions %+v", a.Except)
	}
	if a := ParseAlternative("CTSHS"); a.Level != 6 || !a.Split || !a.Encoded {
		t.Errorf("CTSHS: %+v", a)
	}
	if a := ParseAlternative("CC"); a.Level != 2 || a.Split {
		t.Errorf("CC: %+v", a)
	}
	if a := ParseAlternative("CTH, except from headings 7213 to 7215"); !a.Encoded || !reflect.DeepEqual(a.Except[0].From, []Range{{Lo: "7213", Hi: "7215"}}) {
		t.Errorf("span: %+v", a)
	}
	if a := ParseAlternative("CTH, except from split heading ex 7208 (b) or ex 7211 (b)"); !a.Encoded || !a.Except[0].Described {
		t.Errorf("split heading exception must be described: %+v", a)
	}
	if a := ParseAlternative("CTH with the exclusion of assembly of uppers affixed to inner or to other sole components of heading 6406"); !a.Encoded || a.Condition == "" {
		t.Errorf("exclusion: %+v", a)
	}
	if a := ParseAlternative("change from headings 7213 to 7215 , provided the material has been cold-formed."); a.Kind != KindChangeFrom || !a.Encoded || a.Condition != "the material has been cold-formed" {
		t.Errorf("change from: %+v", a)
	}
	if a := ParseAlternative("Manufacture in which the value of the materials used does not exceed 40 % of the ex-works price of the product"); a.Kind != KindMaxValue || a.MaxShare != 0.4 {
		t.Errorf("max value: %+v", a)
	}
	if a := ParseAlternative("The origin of the goods of this heading shall be the country in which the animal was fattened for a period of at least three months before slaughtering."); a.Kind != KindProcess || !a.Identifies {
		t.Errorf("identifies: %+v", a)
	}
	// Not read: left to an answer, with the reason.
	for _, s := range []string{"CTHS, except from the same subheading", "CTHS from split heading ex 7228 (a)"} {
		if a := ParseAlternative(s); a.Encoded || a.Kind != KindProcess || a.Reason == "" {
			t.Errorf("%q: want unread, got %+v", s, a)
		}
	}
	if alts := ParseRule("k", "CTSH or CTSHS provided recasting or atomizing of the cast alloy"); len(alts) != 1 || alts[0].Encoded {
		t.Errorf("shared proviso: %+v", alts)
	}
}
