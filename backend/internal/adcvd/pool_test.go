package adcvd

import "testing"

func TestPoolIsTheTopKThenThePlainChecksMatches(t *testing.T) {
	tb := refineTable()
	// "wheels" retrieves the wheel group; the part's code matches the wheel group and, by heading, nothing else
	pool := tb.Pool(Query{Description: "steel wheel for a trailer", Country: "China", HTS: "8716.90.5035"}, 12)
	if len(pool) != 1 {
		t.Fatalf("pool = %d groups, want the one wheel group (AD and CVD are one)", len(pool))
	}
	e := pool[0]
	if e.Rank != 1 || !e.HTSMatch || len(e.Cases) != 2 || e.Product != "Certain Steel Wheels" {
		t.Errorf("entry = %+v, want rank 1, HTS match, two cases", e)
	}
	// a group the retrieval does not propose (k = 0) but the plain check matches is rank 0
	pool = tb.Pool(Query{Description: "gizmo", Country: "China", HTS: "9403.60.8081"}, 0)
	if len(pool) != 1 || pool[0].Rank != 0 || !pool[0].HTSMatch || pool[0].Product != "Wooden Cabinets and Vanities" {
		t.Fatalf("pool = %+v, want only the cabinet group as an HTS-only entry", pool)
	}
	// no country, no pool
	if got := tb.Pool(Query{Description: "steel wheel", Country: ""}, 12); len(got) != 0 {
		t.Errorf("pool without a country = %d, want 0", len(got))
	}
}
