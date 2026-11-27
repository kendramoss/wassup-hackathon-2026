package main

import (
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/adcvd"
)

func table() *adcvd.Table {
	return adcvd.NewTable("t", "2026-11-27", []adcvd.Order{
		{Case: "A-570-082", Type: "AD", Country: "The People's Republic of China", Product: "Certain Steel Wheels", Status: "active", HTS: []string{"8716.90.5035"},
			ScopeText: "Steel wheels in sizes from 12 to 16.5 inches in diameter for trailers and towable equipment."},
		{Case: "A-570-106", Type: "AD", Country: "The People's Republic of China", Product: "Wooden Cabinets and Vanities", Status: "active", HTS: []string{"9403.60.8081"},
			ScopeText: "Wooden cabinets and vanities for kitchens and bathrooms."},
		{Case: "A-570-900", Type: "AD", Country: "The People's Republic of China", Product: "Garlic", Status: "active", HTS: []string{"0703.20.0010"},
			ScopeText: "Garlic, whole or separated into cloves, fresh, chilled or frozen."},
	})
}

func TestPoolHoldsRetrievedOrdersAndPlainCheckMatches(t *testing.T) {
	// the words find the wheels; the code (garlic's) is matched by the plain check but ranks nowhere by its words
	ps := Pool(table(), Query{ID: "N1", Description: "steel trailer wheel", Origin: "China", HTS: "0703.20.0010"}, 1)
	if len(ps) != 2 {
		t.Fatalf("pool = %+v, want the retrieved wheels order and the plain check's garlic order", ps)
	}
	if ps[0].Cases[0] != "A-570-082" || ps[0].Rank != 1 || ps[0].HTSMatch {
		t.Errorf("first pair = %+v", ps[0])
	}
	if ps[1].Cases[0] != "A-570-900" || ps[1].Rank != 0 || !ps[1].HTSMatch {
		t.Errorf("second pair = %+v", ps[1])
	}
}

func TestPoolMarksARetrievedOrderThePlainCheckAlsoMatches(t *testing.T) {
	ps := Pool(table(), Query{ID: "N2", Description: "wooden kitchen vanity", Origin: "China", HTS: "9403.60.8081"}, 3)
	if len(ps) == 0 || ps[0].Cases[0] != "A-570-106" || ps[0].Rank != 1 || !ps[0].HTSMatch {
		t.Fatalf("pool = %+v", ps)
	}
	for _, p := range ps {
		if p.ID != "N2" || p.Key == "" {
			t.Errorf("pair without id or key: %+v", p)
		}
	}
}

func TestPoolIsEmptyWithoutOrigin(t *testing.T) {
	if ps := Pool(table(), Query{ID: "N3", Description: "steel wheel", Origin: "", HTS: "8716.90.5035"}, 5); len(ps) != 0 {
		t.Errorf("pool = %+v, want none without an origin", ps)
	}
}
