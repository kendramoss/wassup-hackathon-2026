package httpapi

import (
	"fmt"
	"net/http"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// deleteCase removes a case (D45). Pinned cases (the seeded demos) need the
// admin token, so a visitor cannot remove them from a shared deployment.
func (a *api) deleteCase(w http.ResponseWriter, r *http.Request) {
	c, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Pinned && !a.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "this is a pinned demo case; deleting it needs the admin token", nil)
		return
	}
	if err := a.d.Cases.Delete(r.Context(), c.ID); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pinRequest struct {
	Pinned *bool `json:"pinned"`
}

// pinCase pins or unpins a case (admin only). Pinned cases list first.
func (a *api) pinCase(w http.ResponseWriter, r *http.Request) {
	c, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in pinRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if in.Pinned == nil {
		a.fail(w, r, fmt.Errorf("%w: pinned (true or false) is required", domain.ErrInvalid))
		return
	}
	if c.Pinned != *in.Pinned { // pinning is not case activity: updated_at stays
		c.Pinned = *in.Pinned
		kind := "unpinned"
		if c.Pinned {
			kind = "pinned"
		}
		c.Record(store.Event{Kind: kind, Summary: "case " + kind + " by the admin"})
		if err := a.d.Cases.Put(r.Context(), c); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	a.respond(w, http.StatusOK, a.caseFields(c))
}
