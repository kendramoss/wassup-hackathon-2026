package httpapi

import (
	"net/http"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/demos"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// can_write (#283 capability). Case responses and list rows tell the caller
// whether their write routes would pass authorization for that case, so a UI
// can gate rename, delete and the other writes on server truth instead of a
// guess. It is computed per request from the request's principal, is never
// stored with the case and is never read from the client.
//
// It is the conjunction of the three checks a write route applies, and adds no
// policy of its own: the case is visible to the principal (canSee, which is
// what loadCase answers 404 on), it is not a pinned demo unless the principal
// is the admin token (demoReadOnly / refuseDemoEdit), and the principal's
// workspace role allows an edit (allows(permEdit), what loadCase answers 403
// on). A case the principal cannot see never gets a DTO, so there is nothing
// to leak.
//
// For a pinned demo it means only "no persisted change". The demo's read-only
// analysis paths (Ask, scenarios and plan previews) are unchanged: routes and
// their guards, not this flag, decide what is allowed.
func (p principal) canWrite(c store.Case, ownership bool) bool {
	return p.canSee(c, ownership) && !(c.Pinned && !p.Admin) && p.allows(c, permEdit)
}

// canWrite is principal.canWrite for the request.
func (a *api) canWrite(r *http.Request, c store.Case) bool {
	return a.principalOf(r).canWrite(c, a.d.CaseOwnership)
}

// summaryFor is a list row for c as seen by p: the row, plus can_write.
func (a *api) summaryFor(p principal, c store.Case) caseSummary {
	row := summaryRow(a.withCalibration(c))
	w := p.canWrite(c, a.d.CaseOwnership)
	row.CanWrite = &w
	if c.Pinned && !p.Admin { // a visitor sees no links on a demo, as in the case read
		row.ReviewStatus, row.ReviewExpiresAt = "", nil
	}
	if c.Pinned {
		row.DemoID = demoIDFor(a.d.Demos, row.DisplayName)
	}
	return row
}

// demoIDFor maps a pinned case's display name to the demo fixture it was
// seeded from. The seed names cases after demo titles; an unrecognised name
// simply has no demo_id, which is what an older or custom seed gets.
func demoIDFor(ds []demos.Demo, displayName string) string {
	for _, d := range ds {
		if d.Title != "" && d.Title == displayName {
			return d.ID
		}
	}
	return ""
}
