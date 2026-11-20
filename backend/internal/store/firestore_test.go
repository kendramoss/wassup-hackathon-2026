package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

func TestGzipJSONRoundTrip(t *testing.T) {
	in := Case{ID: "case_a", Facts: strings.Repeat("assembled in Thailand. ", 2000), Plan: &planner.Result{}}
	b, err := gzipJSON(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > len(in.Facts)/10 {
		t.Errorf("gzip barely compressed: %d bytes for %d", len(b), len(in.Facts))
	}
	var out Case
	if err := gunzipJSON(b, &out); err != nil || out.Facts != in.Facts || out.Plan == nil {
		t.Fatalf("round trip = %v (facts equal %t)", err, out.Facts == in.Facts)
	}
}

// TestFirestoreEmulator runs against the Firestore emulator:
//
//	gcloud emulators firestore start --host-port=localhost:8681
//	FIRESTORE_EMULATOR_HOST=localhost:8681 go test ./internal/store
func TestFirestoreEmulator(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	// Own project on the shared emulator. List sorts pinned cases first, so a
	// leftover case_pin from an earlier run of this test would sort ahead of
	// case_b and fail the restart check.
	project := fmt.Sprintf("store-test-%d", time.Now().UnixNano())
	f, err := NewFirestore(ctx, project, "")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Get(ctx, "missing_case"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing = %v", err)
	}
	if _, err := f.Get(ctx, "../etc/passwd"); !errors.Is(err, ErrNotFound) {
		t.Errorf("invalid id not rejected: %v", err)
	}
	_ = f.Put(ctx, Case{ID: "case_gone", UpdatedAt: time.Now().UTC()})
	if err := f.Delete(ctx, "case_gone"); err != nil {
		t.Errorf("Delete = %v", err)
	}
	if err := f.Delete(ctx, "case_gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	_ = f.Put(ctx, Case{ID: "case_a", UpdatedAt: now.Add(-time.Hour)})
	if err := f.Put(ctx, Case{ID: "case_b", UpdatedAt: now, Plan: &planner.Result{}}); err != nil {
		t.Fatal(err)
	}
	f2, err := NewFirestore(ctx, project, "") // a restart
	if err != nil {
		t.Fatal(err)
	}
	defer f2.Close()
	cs, err := f2.List(ctx)
	if err != nil || len(cs) < 2 || cs[0].ID != "case_b" || cs[0].Step() != "plan" || !cs[0].UpdatedAt.Equal(now) {
		t.Errorf("list after restart = %+v %v", cs, err)
	}
	// ListRecent: the pinned case plus the newest others, from the query.
	_ = f.Put(ctx, Case{ID: "case_pin", UpdatedAt: now.Add(-48 * time.Hour), Pinned: true})
	recent, err := f2.ListRecent(ctx, 1)
	if err != nil || len(recent) != 2 || recent[0].ID != "case_pin" || recent[1].ID != "case_b" {
		t.Errorf("ListRecent = %v %v", recent, err)
	}
	// Update replaces the document; the pinned query field has to survive it.
	if _, err := f.Update(ctx, "case_pin", func(c *Case) error {
		c.DisplayName = "still pinned"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	recent, err = f2.ListRecent(ctx, 1)
	if err != nil || len(recent) != 2 || recent[0].ID != "case_pin" || recent[0].DisplayName != "still pinned" {
		t.Errorf("ListRecent after Update = %v %v", recent, err)
	}
	vb := f.Verdicts()
	key := fmt.Sprintf("k%d", now.UnixNano())
	if _, ok, err := vb.Get(ctx, key); ok || err != nil {
		t.Fatalf("missing verdict = %v %v", ok, err)
	}
	if err := vb.Put(ctx, key, []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := vb.Put(ctx, key, []byte(`{"v":2}`)); err != nil { // first write wins, no error
		t.Fatal(err)
	}
	if b, ok, err := f2.Verdicts().Get(ctx, key); !ok || err != nil || string(b) != `{"v":1}` {
		t.Errorf("verdict from another client = %s %v %v", b, ok, err)
	}
	// Workspaces (D68): the query field survives Update, membership is unique.
	wsID := fmt.Sprintf("ws_%d", now.UnixNano())
	_ = f.Put(ctx, Case{ID: "case_team", Workspace: wsID, UpdatedAt: now})
	if _, err := f.Update(ctx, "case_team", func(c *Case) error { c.DisplayName = "renamed"; return nil }); err != nil {
		t.Fatal(err)
	}
	if cs, err := f2.ListWorkspace(ctx, wsID); err != nil || len(cs) != 1 || cs[0].DisplayName != "renamed" {
		t.Errorf("ListWorkspace = %+v %v", cs, err)
	}
	ws, ws2 := f.Workspaces(), f2.Workspaces()
	owner, member := wsID+"@example.com", "m"+wsID+"@example.com"
	if err := ws.Create(ctx, teams.Workspace{ID: wsID, Name: "Acme", Members: []string{owner}, CreatedBy: owner}); err != nil {
		t.Fatal(err)
	}
	if err := ws.Create(ctx, teams.Workspace{ID: wsID + "x", Members: []string{owner}}); !errors.Is(err, teams.ErrMember) {
		t.Errorf("second workspace for the owner = %v", err)
	}
	if w, err := ws.Invite(ctx, wsID, owner, member); err != nil || len(w.Members) != 1 || !w.HasInvite(member) {
		t.Errorf("Invite = %+v %v", w, err)
	}
	if _, err := ws2.Of(ctx, member); !errors.Is(err, teams.ErrNotFound) {
		t.Errorf("Of(invited, not joined) = %v, want ErrNotFound", err)
	}
	if inv, err := ws2.Invites(ctx, member); err != nil || len(inv) != 1 || inv[0].ID != wsID {
		t.Errorf("Invites = %+v %v", inv, err)
	}
	if _, err := ws.Join(ctx, wsID, "uninvited"+member); !errors.Is(err, teams.ErrNotFound) {
		t.Errorf("Join uninvited = %v", err)
	}
	if w, err := ws.Join(ctx, wsID, member); err != nil || len(w.Members) != 2 || w.HasInvite(member) {
		t.Errorf("Join = %+v %v", w, err)
	}
	if _, err := ws.Invite(ctx, wsID, owner, member); !errors.Is(err, teams.ErrMember) {
		t.Errorf("Invite a member = %v", err)
	}
	if w, err := ws2.Of(ctx, member); err != nil || w.ID != wsID || w.Name != "Acme" {
		t.Errorf("Of(member) from another client = %+v %v", w, err)
	}
	if _, err := ws2.Of(ctx, "nobody@example.com"); !errors.Is(err, teams.ErrNotFound) {
		t.Errorf("Of(nobody) = %v", err)
	}
	// Roles (#283). Needs the emulator: not run in the default test pass.
	if w, _ := ws2.Get(ctx, wsID); w.Roles[member] != "editor" || !w.IsAdmin(owner) {
		t.Errorf("roles after join = %+v", w.Roles)
	}
	if _, err := ws.Invite(ctx, wsID, member, "x"+member); !errors.Is(err, teams.ErrForbidden) {
		t.Errorf("an editor invites = %v", err)
	}
	if _, err := ws.SetRole(ctx, wsID, owner, owner, teams.RoleViewer); !errors.Is(err, teams.ErrLastAdmin) {
		t.Errorf("last admin demotes self = %v", err)
	}
	if w, err := ws.SetRole(ctx, wsID, owner, member, teams.RoleAdmin); err != nil || !w.IsAdmin(member) {
		t.Errorf("promote = %+v %v", w, err)
	}
	if _, err := ws.SetRole(ctx, wsID, member, owner, teams.RoleViewer); err != nil {
		t.Errorf("demote the creator with another admin = %v", err)
	}
	if _, err := ws.Remove(ctx, wsID, member, member); !errors.Is(err, teams.ErrLastAdmin) {
		t.Errorf("last admin leaves = %v", err)
	}
	if _, err := ws.Remove(ctx, wsID, owner, member); !errors.Is(err, teams.ErrForbidden) {
		t.Errorf("a viewer removes the admin = %v", err)
	}
	if _, err := ws.Remove(ctx, wsID, owner, owner); err != nil {
		t.Errorf("a viewer leaves = %v", err)
	}
	if _, err := ws2.Of(ctx, owner); !errors.Is(err, teams.ErrNotFound) {
		t.Errorf("Of after leaving = %v", err)
	}
	// A document written before roles existed (no roles field) loads: creator admin, others editor.
	legacy := wsID + "L"
	lo, lm := "lo"+owner, "lm"+owner
	if _, err := f.client.Collection(WorkspaceCollection).Doc(legacy).Set(ctx, map[string]any{
		"id": legacy, "name": "Legacy", "members": []string{lo, lm}, "invited": []string{}, "created_by": lo, "created_at": time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.client.Collection(MemberCollection).Doc(lm).Set(ctx, map[string]any{"workspace_id": legacy}); err != nil {
		t.Fatal(err)
	}
	if w, err := ws2.Of(ctx, lm); err != nil || w.Roles != nil {
		t.Errorf("legacy decode = %+v %v", w, err)
	} else if r, _ := w.RoleOf(lm); r != teams.RoleEditor {
		t.Errorf("legacy member role = %q", r)
	} else if r, _ := w.RoleOf(lo); r != teams.RoleAdmin {
		t.Errorf("legacy creator role = %q", r)
	}
}
