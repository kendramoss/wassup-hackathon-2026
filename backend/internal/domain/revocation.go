package domain

import (
	"sort"
	"time"
)

// Roles a ruling plays in a saved case, for revocation alerts (D480).
const (
	// RoleCited: the prediction named the ruling in its reasoning.
	RoleCited = "cited"
	// RolePlanSupport: a shown plan's precedent check rests on the ruling.
	RolePlanSupport = "plan_support"
	// RoleCurrentSetup: the user gave the ruling as CBP's holding on the
	// current setup.
	RoleCurrentSetup = "current_setup"
	// RoleRead: the ruling was shown to the model and not cited.
	RoleRead = "read"
	// RoleSelf: the case is this ruling (an evaluation row scored against
	// CBP's ruling). It is never alerted on; it only tells RulingAlerts that
	// a change made by this ruling is the case's own act (D490).
	RoleSelf = "self"
)

// Alert statuses.
const (
	AlertFinal    = "final"
	AlertProposed = "proposed"
)

// RulingRef is a ruling a saved case relies on and the role it plays.
type RulingRef struct {
	ID   string
	Role string
}

// RulingAlert says that CBP changed, or proposed to change, a ruling a saved
// case relies on (D480). It is information for the reader: the stored answer
// is never changed by it.
type RulingAlert struct {
	Ruling string   `json:"ruling"`
	Roles  []string `json:"roles"`
	// Status is "final" (a revocation or modification in force or scheduled)
	// or "proposed" (published for comment, not made final in the fetched
	// notices: information only).
	Status string `json:"status"`
	Kind   string `json:"kind"` // "revoked" or "modified"
	By     string `json:"by"`
	// Date is the change's date: the revoking letter's issue date, or a
	// Customs Bulletin notice's effective date (D400). Empty for a proposal.
	Date string `json:"date,omitempty"`
	// InEffect is false for a final change whose effective date is after
	// today; it is scheduled, not yet in force.
	InEffect bool `json:"in_effect,omitempty"`
	// BeforeCase marks a final change dated before the case date: the
	// supersession guard (D320, D370) would refuse to cite the ruling today,
	// so only an answer stored before the guard can rely on it.
	BeforeCase bool `json:"before_case,omitempty"`
	// Bulletin cites the Customs Bulletin notice, when one published it.
	Bulletin string `json:"bulletin,omitempty"`
	URL      string `json:"url,omitempty"`
	// Note is the change in words, e.g. "revoked by H325607 on 2025-08-29".
	Note string `json:"note"`
	// HeldBefore and HeldAfter are the origins the changed ruling held and
	// the changing ruling holds, from the shipped labels ("" when not
	// known); Answer is the case's predicted origin. Relevance is one of
	// the Relevance* labels and RelevanceNote says it in words (D531);
	// RelevanceRole is the role it was judged for: a cited ruling against
	// the answer, a plan's support against the plan's target, the current
	// setup's ruling against CBP's holding there (D561). Set by
	// WithRelevanceBy.
	HeldBefore    string `json:"held_before,omitempty"`
	HeldAfter     string `json:"held_after,omitempty"`
	Answer        string `json:"answer,omitempty"`
	Relevance     string `json:"relevance,omitempty"`
	RelevanceNote string `json:"relevance_note,omitempty"`
	RelevanceRole string `json:"relevance_role,omitempty"`
}

// Relied reports whether the ruling is more than read: cited, a plan's
// support or the current setup's holding.
func (a RulingAlert) Relied() bool {
	for _, r := range a.Roles {
		if r != RoleRead {
			return true
		}
	}
	return false
}

// RulingAlerts lists the CBP changes to the rulings a case relies on (D480).
// A final change is reported whatever its date relative to the case: dated
// on or after the case date, the ruling was CBP's position when the case was
// decided and has changed since; dated before it (BeforeCase), the answer
// was stored before the guard. A ruling with no final change but a pending
// proposal is reported as proposed. today decides only InEffect. Rulings
// appear once, in the order first given, with their roles merged.
//
// A change made by the case itself, or by a ruling the case relies on, is
// not an alert (D490): a revoking ruling cites the ruling it revokes, and an
// answer that names the revoker already knows of the change. So a final
// change whose revoker is a RoleSelf ref, or a relied-on ref (cited, a
// plan's support, the current setup), is left out.
func RulingAlerts(caseDate, today time.Time, refs []RulingRef) []RulingAlert {
	var out []RulingAlert
	at := map[string]int{}
	known := map[string]bool{}
	for _, ref := range refs {
		if ref.ID != "" && ref.Role != RoleRead {
			known[ref.ID] = true
		}
	}
	for _, ref := range refs {
		if ref.ID == "" || ref.Role == RoleSelf {
			continue
		}
		if i, ok := at[ref.ID]; ok {
			out[i].Roles = addRole(out[i].Roles, ref.Role)
			continue
		}
		a, ok := alertFor(ref.ID, caseDate, today)
		if !ok || (a.Status == AlertFinal && known[a.By]) {
			continue
		}
		a.Roles = []string{ref.Role}
		at[ref.ID] = len(out)
		out = append(out, a)
	}
	for i := range out {
		sort.Strings(out[i].Roles)
	}
	return out
}

func alertFor(id string, caseDate, today time.Time) (RulingAlert, bool) {
	if s, ok := SupersededAsOf(id, time.Time{}); ok {
		a := RulingAlert{Ruling: id, Status: AlertFinal, Kind: s.Kind, By: s.By, Date: s.Date, Note: s.Note(), URL: RulingURL(s.By)}
		d, _ := s.Time()
		a.InEffect = !d.After(today)
		a.BeforeCase = !caseDate.IsZero() && d.Before(caseDate)
		if s.Bulletin != nil {
			a.Bulletin, a.URL = s.Bulletin.String(), s.Bulletin.URL
		}
		return a, true
	}
	if p, ok := ProposedFor(id); ok {
		return RulingAlert{Ruling: id, Status: AlertProposed, Kind: p.Kind, By: p.By, Bulletin: p.Bulletin.String(), URL: p.Bulletin.URL, Note: p.Note()}, true
	}
	return RulingAlert{}, false
}

func addRole(rs []string, r string) []string {
	for _, x := range rs {
		if x == r {
			return rs
		}
	}
	return append(rs, r)
}

// AlertCounts summarises alerts on the rulings a case relies on (cited, a
// plan's support or the current setup); rulings it only read are counted
// apart.
type AlertCounts struct {
	Final    int `json:"final"`
	Proposed int `json:"proposed"`
	// ReadOnly counts final changes to rulings the case read but did not cite.
	ReadOnly int `json:"read_only,omitempty"`
	// TouchesAnswer counts the final changes to relied-on rulings whose
	// held origin moved to or from what the case relies on them for: the
	// answer, a shown plan's target or the current setup's holding (D531,
	// D561).
	TouchesAnswer int `json:"touches_answer,omitempty"`
}

// CountAlerts counts alerts by status, relied-on rulings only, plus final
// changes to rulings only read.
func CountAlerts(as []RulingAlert) AlertCounts {
	var n AlertCounts
	for _, a := range as {
		switch {
		case !a.Relied():
			if a.Status == AlertFinal {
				n.ReadOnly++
			}
		case a.Status == AlertFinal:
			n.Final++
			if a.Relevance == RelevanceTouchesAnswer {
				n.TouchesAnswer++
			}
		default:
			n.Proposed++
		}
	}
	return n
}
