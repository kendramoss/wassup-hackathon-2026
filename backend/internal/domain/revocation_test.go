package domain

import (
	"reflect"
	"testing"
)

// N345891 was revoked by H346161 on 2025-05-05 (corpus letter); N023537 by
// H314991 effective 2026-11-29 (Customs Bulletin); 080651 has a proposal only.
func TestRulingAlertsTimeEdges(t *testing.T) {
	today := day("2026-11-22")
	cases := []struct {
		name       string
		caseDate   string
		want       bool
		beforeCase bool
	}{
		{"case before the revocation", "2025-03-01", true, false},
		{"case on the revocation date: still CBP's position then (D370)", "2025-05-05", true, false},
		{"case after the revocation: the guard would refuse it", "2025-06-01", true, true},
	}
	for _, tc := range cases {
		as := RulingAlerts(day(tc.caseDate), today, []RulingRef{{ID: "N345891", Role: RoleCited}})
		if len(as) != 1 {
			t.Fatalf("%s: %d alerts", tc.name, len(as))
		}
		a := as[0]
		if a.Status != AlertFinal || a.By != "H346161" || a.Kind != "revoked" || a.Date != "2025-05-05" || !a.InEffect || a.BeforeCase != tc.beforeCase {
			t.Errorf("%s: %+v", tc.name, a)
		}
		if a.Note != "revoked by H346161 on 2025-05-05" || a.URL != RulingURL("H346161") {
			t.Errorf("%s: note %q url %q", tc.name, a.Note, a.URL)
		}
	}
}

func TestRulingAlertsScheduledBulletinChange(t *testing.T) {
	ref := []RulingRef{{ID: "N023537", Role: RolePlanSupport}}
	a := RulingAlerts(day("2025-01-01"), day("2026-11-22"), ref)[0]
	if a.Status != AlertFinal || a.InEffect || a.Bulletin == "" || a.URL == "" || a.Date != "2026-11-29" {
		t.Errorf("a final change effective after today is scheduled, not in effect: %+v", a)
	}
	if a = RulingAlerts(day("2025-01-01"), day("2026-11-29"), ref)[0]; !a.InEffect {
		t.Error("in effect from its effective date")
	}
}

func TestRulingAlertsProposedVersusFinal(t *testing.T) {
	as := RulingAlerts(day("2026-01-01"), day("2026-11-22"), []RulingRef{
		{ID: "080651", Role: RoleCited},
		{ID: "N345891", Role: RoleCited},
	})
	if len(as) != 2 || as[0].Status != AlertProposed || as[1].Status != AlertFinal {
		t.Fatalf("%+v", as)
	}
	p := as[0]
	if p.By != "H346396" || p.Date != "" || p.InEffect || p.BeforeCase || p.Bulletin == "" {
		t.Errorf("proposal: %+v", p)
	}
	if n := CountAlerts(as); n != (AlertCounts{Final: 1, Proposed: 1}) {
		t.Errorf("counts %+v", n)
	}
}

func TestRulingAlertsMultipleCitations(t *testing.T) {
	as := RulingAlerts(day("2025-03-01"), day("2026-11-22"), []RulingRef{
		{ID: "N357771", Role: RoleCited}, // never changed
		{ID: "N345891", Role: RoleRead},
		{ID: "N345891", Role: RoleCited},
		{ID: "N345891", Role: RoleCited},
		{ID: "N023537", Role: RoleRead},
		{ID: "", Role: RoleCited},
	})
	if len(as) != 2 {
		t.Fatalf("%+v", as)
	}
	if as[0].Ruling != "N345891" || !reflect.DeepEqual(as[0].Roles, []string{RoleCited, RoleRead}) || !as[0].Relied() {
		t.Errorf("roles merged once per ruling: %+v", as[0])
	}
	if as[1].Ruling != "N023537" || as[1].Relied() {
		t.Errorf("read only: %+v", as[1])
	}
	if n := CountAlerts(as); n != (AlertCounts{Final: 1, ReadOnly: 1}) {
		t.Errorf("counts %+v", n)
	}
	if RulingAlerts(day("2025-03-01"), day("2026-11-22"), []RulingRef{{ID: "N357771", Role: RoleCited}}) != nil {
		t.Error("no alert for an unchanged ruling")
	}
}

// D490 (judge panel round 10, N4): H325607 revoked N331974 on its own date
// and cites it. Scored against H325607, an answer citing N331974 is not told
// to look again because of H325607: the case is the revoking ruling. Nor is
// a case that cites the revoker beside the ruling it revoked.
func TestRulingAlertsSkipTheCaseOwnRevocation(t *testing.T) {
	today := day("2026-11-22")
	cited := RulingRef{ID: "N331974", Role: RoleCited}
	if as := RulingAlerts(day("2025-08-29"), today, []RulingRef{cited}); len(as) != 1 || as[0].By != "H325607" {
		t.Fatalf("without the case's own ruling the alert stands: %+v", as)
	}
	if as := RulingAlerts(day("2025-08-29"), today, []RulingRef{{ID: "H325607", Role: RoleSelf}, cited}); len(as) != 0 {
		t.Errorf("the case is the revoking ruling: %+v", as)
	}
	if as := RulingAlerts(day("2025-08-29"), today, []RulingRef{cited, {ID: "H325607", Role: RoleCited}}); len(as) != 0 {
		t.Errorf("the case names the revoker: %+v", as)
	}
	// Only read, the revoker does not silence the alert: the answer did not
	// rely on it.
	if as := RulingAlerts(day("2025-08-29"), today, []RulingRef{cited, {ID: "H325607", Role: RoleRead}}); len(as) != 1 {
		t.Errorf("a revoker only read does not count: %+v", as)
	}
	// Another ruling as the case: N356467 citing N344694, modified by
	// H344999 after the case date, still alerts.
	if as := RulingAlerts(day("2025-12-22"), today, []RulingRef{{ID: "N356467", Role: RoleSelf}, {ID: "N344694", Role: RoleCited}}); len(as) != 1 || as[0].By != "H344999" || as[0].BeforeCase {
		t.Errorf("N356467: %+v", as)
	}
}
