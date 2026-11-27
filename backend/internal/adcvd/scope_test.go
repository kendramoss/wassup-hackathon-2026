package adcvd

import "testing"

func scopeTable() *Table {
	return NewTable("test-v1", "2026-11-27", []Order{
		{Case: "A-570-082", Type: "AD", Country: "The People's Republic of China", Product: "Certain Steel Wheels", Status: "active", HTS: []string{"8716.90.5035"},
			ScopeText: "Steel wheels in sizes from 12 to 16.5 inches in diameter for trailers and towable equipment. Excluded are wheels made of aluminum."},
		{Case: "C-570-083", Type: "CVD", Country: "The People's Republic of China", Product: "Certain Steel Wheels", Status: "active", HTS: []string{"8716.90.5035"},
			ScopeText: "Steel wheels in sizes from 12 to 16.5 inches in diameter for trailers and towable equipment. Excluded are wheels made of aluminum."},
		{Case: "A-570-106", Type: "AD", Country: "The People's Republic of China", Product: "Wooden Cabinets and Vanities", Status: "active", HTS: []string{"9403.60.8081"},
			ScopeText: "Wooden cabinets and vanities for kitchens and bathrooms, assembled or unassembled."},
		{Case: "A-570-900", Type: "AD", Country: "The People's Republic of China", Product: "Garlic", Status: "active", HTS: []string{"0703.20.0010"},
			ScopeText: "Garlic, whole or separated into cloves, fresh, chilled or frozen."},
		{Case: "A-533-001", Type: "AD", Country: "India", Product: "Certain Steel Wheels", Status: "active", HTS: []string{"8716.90.5035"},
			ScopeText: "Steel wheels in sizes from 12 to 16.5 inches in diameter for trailers."},
		{Case: "A-570-777", Type: "AD", Country: "The People's Republic of China", Product: "Thing", Status: "active"},
	})
}

func TestCandidatesAreOfTheOriginCountryOnly(t *testing.T) {
	got := scopeTable().Candidates(Query{Description: "steel trailer wheel", Country: "India"}, 10)
	if len(got) != 1 || got[0].Cases[0] != "A-533-001" {
		t.Fatalf("India candidates = %+v", got)
	}
	if got := scopeTable().Candidates(Query{Description: "steel trailer wheel", Country: ""}, 10); len(got) != 0 {
		t.Errorf("no origin must give no candidates, got %d", len(got))
	}
}

func TestCandidatesJoinCompanionOrders(t *testing.T) {
	got := scopeTable().Candidates(Query{Description: "steel wheel for a trailer", Country: "China"}, 10)
	if len(got) == 0 || len(got[0].Cases) != 2 || got[0].Cases[0] != "A-570-082" || got[0].Cases[1] != "C-570-083" {
		t.Fatalf("top candidate = %+v, want the AD and CVD wheel orders as one", got)
	}
}

func TestCandidatesRankByTheWordsOfTheScopeNotTheHTSCode(t *testing.T) {
	// no HTS code given: the written scope alone finds the order
	got := scopeTable().Candidates(Query{Description: "kitchen vanity made of wood", Country: "China"}, 3)
	if len(got) == 0 || got[0].Cases[0] != "A-570-106" {
		t.Fatalf("top candidate = %+v, want the cabinets order", got)
	}
	// an order that lists no code and a product the description shares nothing with is not a candidate
	for _, c := range got {
		if c.Cases[0] == "A-570-900" {
			t.Errorf("garlic is not a candidate for a vanity")
		}
	}
}

func TestCandidatesCapAndHTSHintOnlyBoosts(t *testing.T) {
	tb := scopeTable()
	if got := tb.Candidates(Query{Description: "wooden steel garlic wheel", Country: "China"}, 2); len(got) != 2 {
		t.Fatalf("cap: got %d, want 2", len(got))
	}
	// a description with no product words falls back on the code: the hint finds the heading
	got := tb.Candidates(Query{Description: "item", Country: "China", HTS: "9403.60.0000"}, 1)
	if len(got) != 1 || got[0].Cases[0] != "A-570-106" {
		t.Fatalf("hint candidate = %+v", got)
	}
	// the hint never removes an order a word found
	got = tb.Candidates(Query{Description: "garlic cloves", Country: "China", HTS: "9403.60.0000"}, 5)
	found := false
	for _, c := range got {
		if c.Cases[0] == "A-570-900" {
			found = true
		}
	}
	if !found {
		t.Errorf("hint removed the garlic order a word had found: %+v", got)
	}
}

func TestCandidateCarriesTheLongestScope(t *testing.T) {
	got := scopeTable().Candidates(Query{Description: "steel wheel", Country: "China"}, 1)
	if got[0].Scope == "" || got[0].Product != "Certain Steel Wheels" {
		t.Errorf("candidate = %+v", got[0])
	}
}

func TestGroupOfCaseFindsTheOrderWhateverItsRank(t *testing.T) {
	tb := scopeTable()
	c, ok := tb.GroupOf([]string{"C-570-083"})
	if !ok || len(c.Cases) != 2 || c.Product != "Certain Steel Wheels" || c.Scope == "" {
		t.Fatalf("GroupOf = %+v, %v", c, ok)
	}
	if _, ok := tb.GroupOf([]string{"A-999-999"}); ok {
		t.Error("an unknown case must not be found")
	}
}

func TestHasCountryReadsCommerceAndPlainNames(t *testing.T) {
	tb := scopeTable()
	for _, c := range []string{"China", "People's Republic of China", "india", "China/India"} {
		if !tb.HasCountry(c) {
			t.Errorf("HasCountry(%q) = false", c)
		}
	}
	for _, c := range []string{"", "Peru", "unknown"} {
		if tb.HasCountry(c) {
			t.Errorf("HasCountry(%q) = true", c)
		}
	}
}
