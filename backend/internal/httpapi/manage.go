package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

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

type renameRequest struct {
	DisplayName *string `json:"display_name"`
}

// MaxDisplayName caps a case's display name, in characters.
const MaxDisplayName = 200

// renameCase sets the case's display name (D48). Pinned cases need the admin
// token, as for delete. No model call; updated_at is unchanged.
func (a *api) renameCase(w http.ResponseWriter, r *http.Request) {
	c, err := a.d.Cases.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in renameRequest
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	if c.Pinned && !a.isAdmin(r) {
		writeError(w, http.StatusForbidden, "forbidden", "this is a pinned demo case; renaming it needs the admin token", nil)
		return
	}
	name := ""
	if in.DisplayName != nil {
		name = strings.Join(strings.Fields(*in.DisplayName), " ")
	}
	if n := utf8.RuneCountInString(name); n == 0 || n > MaxDisplayName {
		a.fail(w, r, fmt.Errorf("%w: display_name must be 1 to %d characters", domain.ErrInvalid, MaxDisplayName))
		return
	}
	saved, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if name == latest.DisplayName {
			return nil
		}
		old := summaryRow(*latest).DisplayName
		latest.DisplayName = name
		latest.Record(store.Event{Kind: "renamed", Summary: fmt.Sprintf("renamed from %q to %q", old, name)})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, summaryRow(saved))
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
	saved, err := a.d.Cases.Update(r.Context(), c.ID, func(latest *store.Case) error {
		if latest.Pinned == *in.Pinned { // pinning is not case activity: updated_at stays
			return nil
		}
		latest.Pinned = *in.Pinned
		kind := "unpinned"
		if latest.Pinned {
			kind = "pinned"
		}
		latest.Record(store.Event{Kind: kind, Summary: "case " + kind + " by the admin"})
		return nil
	})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.respond(w, http.StatusOK, a.caseFields(saved))
}
