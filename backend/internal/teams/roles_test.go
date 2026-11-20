package teams

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	ann = "ann@example.com" // creator
	ben = "ben@example.com"
	cat = "cat@example.com"
)

func mk(t *testing.T, w Workspace) *Memory {
	t.Helper()
	m := NewMemory()
	if err := m.Create(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestRoleOfLegacyAndExplicit(t *testing.T) {
	legacy := Workspace{ID: "ws_l", Members: []string{ann, ben}, CreatedBy: ann} // no Roles at all
	for email, want := range map[string]Role{ann: RoleAdmin, ben: RoleEditor} {
		if got, ok := legacy.RoleOf(email); !ok || got != want {
			t.Errorf("legacy RoleOf(%s) = %q %v, want %q", email, got, ok, want)
		}
	}
	for _, stranger := range []string{"zed@example.com", "", " "} {
		if got, ok := legacy.RoleOf(stranger); ok || got != "" {
			t.Errorf("non-member %q has role %q", stranger, got)
		}
	}
	legacy.Invited = []string{cat}
	if r, ok := legacy.RoleOf(cat); ok || r != "" {
		t.Errorf("invited-only email has role %q", r)
	}
	if legacy.IsAdmin(cat) || legacy.IsAdmin(ben) || !legacy.IsAdmin(ann) {
		t.Error("IsAdmin on legacy workspace")
	}
	// An explicit role beats the creator rule; an unknown stored value fails closed to viewer.
	w := Workspace{ID: "ws_e", Members: []string{ann, ben, cat}, CreatedBy: ann, Roles: map[string]string{ann: "editor", ben: "admin", cat: "superuser"}}
	for email, want := range map[string]Role{ann: RoleEditor, ben: RoleAdmin, cat: RoleViewer} {
		if got, _ := w.RoleOf(email); got != want {
			t.Errorf("RoleOf(%s) = %q, want %q", email, got, want)
		}
	}
}

func TestLegacyJSONWithoutRolesLoadsAsEditor(t *testing.T) {
	var w Workspace
	raw := `{"id":"ws_old","name":"Old","members":["ann@example.com","ben@example.com"],"invited":[],"created_by":"ann@example.com","created_at":"2026-01-01T00:00:00Z"}`
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		t.Fatal(err)
	}
	if r, _ := w.RoleOf(ben); r != RoleEditor {
		t.Errorf("legacy member = %q, want editor", r)
	}
	if r, _ := w.RoleOf(ann); r != RoleAdmin {
		t.Errorf("legacy creator = %q, want admin", r)
	}
	// The Firestore adapter decodes through struct tags: a missing field must stay optional.
	f, _ := reflect.TypeOf(w).FieldByName("Roles")
	if tag := f.Tag.Get("firestore"); tag != "roles,omitempty" {
		t.Errorf("Roles firestore tag = %q; a legacy document without the field must still decode", tag)
	}
}

func TestParseRole(t *testing.T) {
	for in, ok := range map[string]bool{"viewer": true, "editor": true, "admin": true, "Admin": false, "owner": false, "": false} {
		if _, got := ParseRole(in); got != ok {
			t.Errorf("ParseRole(%q) ok = %v", in, got)
		}
	}
}

func TestSetRoleRules(t *testing.T) {
	ctx := context.Background()
	m := mk(t, Workspace{ID: "ws_a", Name: "A", Members: []string{ann, ben}, CreatedBy: ann, Roles: map[string]string{ann: "admin"}})
	// Joined members default to editor, stored explicitly.
	if _, err := m.Invite(ctx, "ws_a", ann, cat); err != nil {
		t.Fatal(err)
	}
	if w, err := m.Join(ctx, "ws_a", cat); err != nil || w.Roles[cat] != "editor" {
		t.Fatalf("Join = %+v %v", w, err)
	}
	// Non-admin cannot change roles (not even their own).
	for _, actor := range []string{ben, cat, "zed@example.com"} {
		if _, err := m.SetRole(ctx, "ws_a", actor, ben, RoleAdmin); !errors.Is(err, ErrForbidden) {
			t.Errorf("%s sets role = %v, want ErrForbidden", actor, err)
		}
	}
	// Target must be a joined member: an invitation has no role.
	if _, err := m.Invite(ctx, "ws_a", ann, "inv@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetRole(ctx, "ws_a", ann, "inv@example.com", RoleViewer); !errors.Is(err, ErrNotMember) {
		t.Errorf("role for an invited-only email = %v, want ErrNotMember", err)
	}
	if _, err := m.SetRole(ctx, "ws_a", ann, ben, Role("owner")); !errors.Is(err, ErrInvalidRole) {
		t.Errorf("invalid role = %v", err)
	}
	if _, err := m.SetRole(ctx, "ws_x", ann, ben, RoleViewer); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown workspace = %v", err)
	}
	// Promote, then demote the creator once another admin exists.
	if w, err := m.SetRole(ctx, "ws_a", ann, ben, RoleAdmin); err != nil || w.Roles[ben] != "admin" {
		t.Fatalf("promote = %+v %v", w, err)
	}
	if w, err := m.SetRole(ctx, "ws_a", ben, ann, RoleViewer); err != nil || w.IsAdmin(ann) {
		t.Fatalf("demote creator with another admin = %+v %v", w, err)
	}
	// ann is now a viewer: cannot change roles any more (membership role re-read each time).
	if _, err := m.SetRole(ctx, "ws_a", ann, ann, RoleAdmin); !errors.Is(err, ErrForbidden) {
		t.Errorf("demoted admin regains admin = %v, want ErrForbidden", err)
	}
	// ben is the last admin: no demotion, no removal, no leaving.
	if _, err := m.SetRole(ctx, "ws_a", ben, ben, RoleEditor); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("last admin demotes self = %v", err)
	}
	if _, err := m.Remove(ctx, "ws_a", ben, ben); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("last admin leaves = %v", err)
	}
	if w, _ := m.Get(ctx, "ws_a"); !w.IsAdmin(ben) {
		t.Error("last admin lost the admin role despite the refusal")
	}
	// Idempotent.
	if _, err := m.SetRole(ctx, "ws_a", ben, ben, RoleAdmin); err != nil {
		t.Errorf("same role = %v", err)
	}
}

func TestRemoveRules(t *testing.T) {
	ctx := context.Background()
	m := mk(t, Workspace{ID: "ws_a", Members: []string{ann, ben, cat}, CreatedBy: ann, Roles: map[string]string{ann: "admin"}})
	if _, err := m.Remove(ctx, "ws_a", ben, cat); !errors.Is(err, ErrForbidden) {
		t.Errorf("non-admin removes another = %v", err)
	}
	if _, err := m.Remove(ctx, "ws_a", ann, "zed@example.com"); !errors.Is(err, ErrNotMember) {
		t.Errorf("remove non-member = %v", err)
	}
	if _, err := m.Remove(ctx, "ws_x", ann, ben); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown workspace = %v", err)
	}
	// A member may leave on their own.
	if w, err := m.Remove(ctx, "ws_a", cat, cat); err != nil || w.HasMember(cat) {
		t.Fatalf("leave = %+v %v", w, err)
	}
	if _, err := m.Of(ctx, cat); !errors.Is(err, ErrNotFound) {
		t.Errorf("Of after leaving = %v", err)
	}
	// Admin removes a member; role entry goes with them.
	if w, err := m.Remove(ctx, "ws_a", ann, ben); err != nil || w.HasMember(ben) {
		t.Fatalf("remove = %+v %v", w, err)
	}
	if w, _ := m.Get(ctx, "ws_a"); len(w.Roles) != 1 {
		t.Errorf("roles left behind: %v", w.Roles)
	}
	// The removed can be invited again, and join as editor (creator rule does not resurrect admin).
	if _, err := m.Remove(ctx, "ws_a", ann, ann); !errors.Is(err, ErrLastAdmin) {
		t.Errorf("sole admin leaves = %v", err)
	}
	if _, err := m.Invite(ctx, "ws_a", ann, ben); err != nil {
		t.Fatal(err)
	}
	if w, err := m.Join(ctx, "ws_a", ben); err != nil || !w.HasMember(ben) || w.IsAdmin(ben) {
		t.Fatalf("rejoin = %+v %v", w, err)
	}
}

func TestCreatorLeavingCannotRegainAdminByRejoining(t *testing.T) {
	ctx := context.Background()
	m := mk(t, Workspace{ID: "ws_a", Members: []string{ann, ben}, CreatedBy: ann, Roles: map[string]string{ann: "admin"}})
	if _, err := m.SetRole(ctx, "ws_a", ann, ben, RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Remove(ctx, "ws_a", ann, ann); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Invite(ctx, "ws_a", ben, ann); err != nil {
		t.Fatal(err)
	}
	w, err := m.Join(ctx, "ws_a", ann)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := w.RoleOf(ann); r != RoleEditor {
		t.Errorf("creator rejoined as %q, want editor", r)
	}
}

func TestInviteNeedsAdmin(t *testing.T) {
	ctx := context.Background()
	m := mk(t, Workspace{ID: "ws_a", Members: []string{ann, ben}, CreatedBy: ann, Roles: map[string]string{ann: "admin", ben: "viewer"}})
	if _, err := m.Invite(ctx, "ws_a", ben, cat); !errors.Is(err, ErrForbidden) {
		t.Errorf("viewer invites = %v", err)
	}
	if _, err := m.Invite(ctx, "ws_a", "zed@example.com", cat); !errors.Is(err, ErrForbidden) {
		t.Errorf("outsider invites = %v", err)
	}
	if _, err := m.Invite(ctx, "ws_a", "", cat); !errors.Is(err, ErrForbidden) {
		t.Errorf("empty actor invites = %v", err)
	}
}

// Two admins demote each other at once: the check and the write are one step,
// so exactly one demotion wins and an admin always remains.
func TestConcurrentDemotionsKeepAnAdmin(t *testing.T) {
	for i := 0; i < 200; i++ {
		ctx := context.Background()
		m := mk(t, Workspace{ID: "ws_a", Members: []string{ann, ben}, CreatedBy: ann, Roles: map[string]string{ann: "admin", ben: "admin"}})
		var wg sync.WaitGroup
		var ok, last atomic.Int32
		for _, p := range [][2]string{{ann, ben}, {ben, ann}} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := m.SetRole(ctx, "ws_a", p[0], p[1], RoleViewer)
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, ErrLastAdmin), errors.Is(err, ErrForbidden):
					last.Add(1)
				default:
					t.Errorf("unexpected error %v", err)
				}
			}()
		}
		wg.Wait()
		w, _ := m.Get(ctx, "ws_a")
		admins := 0
		for _, e := range w.Members {
			if w.IsAdmin(e) {
				admins++
			}
		}
		if admins != 1 || ok.Load() != 1 {
			t.Fatalf("round %d: %d admins, %d successes (want exactly 1 and 1)", i, admins, ok.Load())
		}
	}
}

// A demoted admin's in-flight invite loses to the demotion.
func TestConcurrentRemoveAndLeaveKeepAnAdmin(t *testing.T) {
	for i := 0; i < 200; i++ {
		ctx := context.Background()
		m := mk(t, Workspace{ID: "ws_a", Members: []string{ann, ben}, CreatedBy: ann, Roles: map[string]string{ann: "admin", ben: "admin"}})
		var wg sync.WaitGroup
		for _, p := range [][2]string{{ann, ann}, {ben, ben}, {ann, ben}} {
			wg.Add(1)
			go func() { defer wg.Done(); _, _ = m.Remove(ctx, "ws_a", p[0], p[1]) }()
		}
		wg.Wait()
		w, _ := m.Get(ctx, "ws_a")
		admins := 0
		for _, e := range w.Members {
			if w.IsAdmin(e) {
				admins++
			}
		}
		if admins < 1 {
			t.Fatalf("round %d: workspace left with no admin: %+v", i, w)
		}
	}
}
