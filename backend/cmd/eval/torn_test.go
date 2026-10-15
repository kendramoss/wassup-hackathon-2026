package main

import "testing"

func TestGuardDropsConclusionsAndUngroundedAnswers(t *testing.T) {
	file := "FACTS:\nThe housing is injection moulded in Thailand from resin made in Korea.\nLAW AND ANALYSIS: ..."
	ok := guard(qa{Question: "q", Answer: "We mould the housing in Thailand.", Quote: "injection  moulded in Thailand"}, file)
	if ok.Dropped != "" || ok.Answer != "We mould the housing in Thailand." {
		t.Errorf("grounded answer dropped: %+v", ok)
	}
	if g := guard(qa{Answer: "We use Korean-origin resin.", Quote: "resin made in Korea"}, file); g.Dropped != "" {
		t.Errorf("component origin fact dropped: %+v", g)
	}
	for _, a := range []qa{
		{Answer: "The product originates in Thailand.", Quote: "injection moulded in Thailand"},
		{Answer: "We mould the housing in Vietnam.", Quote: "moulded in Vietnam"},
	} {
		if g := guard(a, file); g.Answer != "Not stated." || g.Dropped == "" {
			t.Errorf("leaky or ungrounded answer kept: %+v", g)
		}
	}
	if f, src := requestFile("FACTS: x\n  HOLDING: China", "x"); f != "FACTS: x\n  " || src != "pre_holding" {
		t.Errorf("HQ file = %q %s", f, src)
	}
	if f, src := requestFile("FACTS: x. It is the opinion of this office that ...", "x."); f != "x." || src != "facts_only" {
		t.Errorf("NY file = %q %s", f, src)
	}
}

func TestSplitTorn(t *testing.T) {
	s := newSplit(map[string]int{"China": 3, "Thailand": 2}, "China")
	if !s.torn() || s.Second != "Thailand" || s.settled() {
		t.Errorf("3/2 split = %+v", s)
	}
	if s := newSplit(map[string]int{"China": 4, "Thailand": 1}, "China"); s.torn() || !s.settled() {
		t.Errorf("4/1 split = %+v", s)
	}
}
