package duty

import (
	"testing"
	"time"
)

func TestNoLiveRateCoversColumn2Spellings(t *testing.T) {
	for _, s := range []string{"DPRK", "Korea, North", "Democratic People's Republic of Korea", "Republic of Cuba", "Byelorussia", "NORTH KOREA ", "Russia"} {
		if !NoLiveRate(s) {
			t.Errorf("NoLiveRate(%q) = false", s)
		}
	}
	for _, s := range []string{"UAE", "Chinese Taipei", "Korea, South", "Vietnam"} {
		if NoLiveRate(s) {
			t.Errorf("NoLiveRate(%q) = true", s)
		}
	}
}

func TestFillUsesDutyIdentity(t *testing.T) {
	r := 0.037
	m := MFN{HTS: "8516.21.00", Supported: true, Rate: &r, FetchedAt: time.Now()}
	out, filled := m.Fill(nil, []string{"DPRK", "Byelorussia", "UAE", "United Arab Emirates", "Korea, South"})
	if len(filled) != 2 || out["United Arab Emirates"] != r || out["South Korea"] != r || len(out) != 2 {
		t.Errorf("filled %v out %v", filled, out)
	}
	// A manual rate under an alias is kept and the canonical name is not filled over it.
	out, filled = m.Fill(map[string]float64{"UAE": 0}, []string{"United Arab Emirates"})
	if len(filled) != 0 || len(out) != 1 || out["UAE"] != 0 {
		t.Errorf("manual rate overwritten: %v %v", out, filled)
	}
}

// "S. Korea" is an accepted spelling already in the country alias table; duty
// pricing must read it as South Korea like "Korea, South" (#718 review).
func TestFillPricesSKoreaAsSouthKorea(t *testing.T) {
	r := 0.037
	m := MFN{HTS: "8516.21.00", Supported: true, Rate: &r, FetchedAt: time.Now()}
	out, filled := m.Fill(nil, []string{"S. Korea", "South Korea"})
	if len(filled) != 1 || out["South Korea"] != r || len(out) != 1 {
		t.Errorf("filled %v out %v", filled, out)
	}
}
