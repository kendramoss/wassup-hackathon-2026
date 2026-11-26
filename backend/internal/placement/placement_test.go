package placement

import (
	"context"
	"errors"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestFromEnvDefaultsAreInactive(t *testing.T) {
	c, err := FromEnv(env(nil))
	if err != nil || c.Tier != Shared || c.Active() {
		t.Fatalf("got %+v, %v; want shared and inactive", c, err)
	}
}

func TestFromEnvValidation(t *testing.T) {
	for _, m := range []map[string]string{
		{"TENANT_TIER": "dedicated"},
		{"TENANT_TIER": "private"},
		{"TENANT_WORKSPACE": "acme"},
		{"DATA_REGION": "EU West!"},
	} {
		if _, err := FromEnv(env(m)); err == nil {
			t.Errorf("%v: want an error", m)
		}
	}
	c, err := FromEnv(env(map[string]string{"TENANT_TIER": "Dedicated", "TENANT_WORKSPACE": "acme", "DATA_REGION": "EU"}))
	if err != nil || c.Tier != Dedicated || c.Region != "eu" || !c.Active() {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestWrapInactiveIsIdentity(t *testing.T) {
	m := store.NewMemory()
	if Wrap(m, Config{Tier: Shared}) != store.Cases(m) {
		t.Fatal("an inactive placement must return the store unchanged")
	}
}

func TestRegionStampsAndHidesOtherRegions(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()
	_ = m.Put(ctx, store.Case{ID: "us1", DataRegion: "us"})
	_ = m.Put(ctx, store.Case{ID: "old"})
	eu := Wrap(m, Config{Tier: Shared, Region: "eu"})

	if err := eu.Put(ctx, store.Case{ID: "eu1"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Get(ctx, "eu1"); got.DataRegion != "eu" {
		t.Fatalf("new case region %q, want eu", got.DataRegion)
	}
	if _, err := eu.Get(ctx, "us1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("reading a us case from eu: %v, want not found", err)
	}
	if _, err := eu.Update(ctx, "us1", func(*store.Case) error { return nil }); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("updating a us case from eu: %v, want not found", err)
	}
	if err := eu.Put(ctx, store.Case{ID: "us1"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("overwriting a us case from eu: %v, want not found", err)
	}
	if err := eu.Delete(ctx, "us1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleting a us case from eu: %v, want not found", err)
	}
	if got, _ := m.Get(ctx, "us1"); got.DataRegion != "us" {
		t.Fatal("the us case changed")
	}
	// An untagged case (before residency) is served and tagged on write.
	if _, err := eu.Update(ctx, "old", func(*store.Case) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Get(ctx, "old"); got.DataRegion != "eu" {
		t.Fatalf("old case region %q after write, want eu", got.DataRegion)
	}
	all, _ := eu.List(ctx)
	recent, _ := store.ListRecent(ctx, eu, 10)
	for _, l := range [][]store.Case{all, recent} {
		for _, c := range l {
			if c.ID == "us1" {
				t.Fatal("a list from eu shows the us case")
			}
		}
		if len(l) != 2 {
			t.Fatalf("listed %d cases, want 2", len(l))
		}
	}
}

func TestDedicatedServesOneWorkspaceAndDemos(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()
	_ = m.Put(ctx, store.Case{ID: "other", Workspace: "beta"})
	_ = m.Put(ctx, store.Case{ID: "demo", Pinned: true})
	_ = m.Put(ctx, store.Case{ID: "anon"})
	d := Wrap(m, Config{Tier: Dedicated, Workspace: "acme"})

	if err := d.Put(ctx, store.Case{ID: "mine", Workspace: "acme"}); err != nil {
		t.Fatal(err)
	}
	if err := d.Put(ctx, store.Case{ID: "x", Workspace: "beta"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("writing another workspace's case: %v", err)
	}
	if err := d.Put(ctx, store.Case{ID: "y"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("writing an anonymous case: %v", err)
	}
	for _, id := range []string{"other", "anon"} {
		if _, err := d.Get(ctx, id); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("reading %s: %v", id, err)
		}
	}
	if _, err := d.Get(ctx, "demo"); err != nil {
		t.Fatalf("pinned demo: %v", err)
	}
	// A handler must not move a case into another workspace.
	if _, err := d.Update(ctx, "mine", func(c *store.Case) error { c.Workspace = "beta"; return nil }); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("moving a case out: %v", err)
	}
	ws, _ := store.ListWorkspace(ctx, d, "beta")
	if len(ws) != 0 {
		t.Fatalf("listed %d beta cases", len(ws))
	}
	all, _ := d.List(ctx)
	if len(all) != 2 {
		t.Fatalf("listed %d, want mine and demo", len(all))
	}
}
