package teams

import (
	"context"
	"errors"
	"testing"
)

func TestProvisionAddsMemberWithRole(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if err := m.Create(ctx, Workspace{ID: "ws_a", Members: []string{"admin@a.com"}, CreatedBy: "admin@a.com", Roles: map[string]string{"admin@a.com": "admin"}}); err != nil {
		t.Fatal(err)
	}
	w, err := m.Provision(ctx, "ws_a", "new@a.com", RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := w.RoleOf("new@a.com"); !ok || r != RoleViewer {
		t.Fatalf("role = %v %v", r, ok)
	}
	if got, err := m.Of(ctx, "new@a.com"); err != nil || got.ID != "ws_a" {
		t.Fatalf("Of = %v %v", got.ID, err)
	}
	// Idempotent: a second provision keeps the role it has.
	if _, err := m.SetRole(ctx, "ws_a", "admin@a.com", "new@a.com", RoleEditor); err != nil {
		t.Fatal(err)
	}
	w, err = m.Provision(ctx, "ws_a", "new@a.com", RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	if r, _ := w.RoleOf("new@a.com"); r != RoleEditor {
		t.Fatalf("re-provision changed role to %v", r)
	}
}

func TestProvisionRefusesMemberOfAnotherWorkspace(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	for _, id := range []string{"ws_a", "ws_b"} {
		admin := "admin@" + id + ".com"
		if err := m.Create(ctx, Workspace{ID: id, Members: []string{admin}, CreatedBy: admin}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Provision(ctx, "ws_a", "admin@ws_b.com", RoleEditor); !errors.Is(err, ErrMember) {
		t.Fatalf("err = %v, want ErrMember", err)
	}
	if _, err := m.Provision(ctx, "ws_missing", "x@a.com", RoleEditor); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if _, err := m.Provision(ctx, "ws_a", "x@a.com", Role("owner")); !errors.Is(err, ErrInvalidRole) {
		t.Fatalf("err = %v, want ErrInvalidRole", err)
	}
}

func TestDeprovisionKeepsAnAdmin(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if err := m.Create(ctx, Workspace{ID: "ws_a", Members: []string{"admin@a.com"}, CreatedBy: "admin@a.com", Roles: map[string]string{"admin@a.com": "admin"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Deprovision(ctx, "ws_a", "admin@a.com"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("err = %v, want ErrLastAdmin", err)
	}
	if _, err := m.Provision(ctx, "ws_a", "u@a.com", RoleEditor); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Deprovision(ctx, "ws_a", "u@a.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Of(ctx, "u@a.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Of after deprovision = %v", err)
	}
	if _, err := m.Deprovision(ctx, "ws_a", "u@a.com"); !errors.Is(err, ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestProvisionUsesPendingInvitation(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if err := m.Create(ctx, Workspace{ID: "ws_a", Members: []string{"admin@a.com"}, CreatedBy: "admin@a.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Invite(ctx, "ws_a", "admin@a.com", "inv@a.com"); err != nil {
		t.Fatal(err)
	}
	w, err := m.Provision(ctx, "ws_a", "inv@a.com", RoleViewer)
	if err != nil {
		t.Fatal(err)
	}
	if w.HasInvite("inv@a.com") || !w.HasMember("inv@a.com") {
		t.Fatalf("invite not used: %+v", w)
	}
	if r, _ := w.RoleOf("inv@a.com"); r != RoleViewer {
		t.Fatalf("role = %v", r)
	}
}
