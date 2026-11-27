package httpapi

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/engine"
)

type memBlob struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (b *memBlob) Get(_ context.Context, k string) ([]byte, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.m[k]
	return v, ok, nil
}
func (b *memBlob) Put(_ context.Context, k string, v []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.m[k] = v
	return nil
}
func (b *memBlob) Delete(_ context.Context, k string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.m, k)
	return nil
}
func (b *memBlob) has(k string) bool { _, ok, _ := b.Get(context.Background(), k); return ok }

type noDelete struct{ blob.Store }

type ownerSpy struct {
	Predictor
	mu   sync.Mutex
	seen []string
}

func (s *ownerSpy) Predict(ctx context.Context, c domain.Case) (engine.Result, error) {
	id, _ := blob.OwnerOf(ctx)
	s.mu.Lock()
	s.seen = append(s.seen, c.ID+"="+id)
	s.mu.Unlock()
	return s.Predictor.Predict(ctx, c)
}

func TestDispatchMarksTheCaseAsOwnerOfWhatItCaches(t *testing.T) {
	var spy *ownerSpy
	e := newAuditEnvWith(t, true, func(d *Deps) { spy = &ownerSpy{Predictor: d.Engine}; d.Engine = spy })
	id := e.wsCase()
	spy.mu.Lock()
	defer spy.mu.Unlock()
	want := id + "=" + id
	for _, s := range spy.seen {
		if s == want {
			return
		}
	}
	t.Fatalf("engine saw %v, want %s", spy.seen, want)
}

func TestUserDeleteRemovesOnlyWhatNoOtherCaseUses(t *testing.T) {
	raw := &memBlob{m: map[string][]byte{}}
	refs := blob.NewMemoryRefs()
	tracked := blob.Track(raw, blob.StoreVerdicts, refs)
	e := newAuditEnvWith(t, true, func(d *Deps) {
		d.CachePurge = &blob.Purger{Refs: refs, Stores: map[string]blob.Store{blob.StoreVerdicts: tracked}}
	})
	a, b := e.wsCase(), e.wsCase()
	ctx := context.Background()
	_ = tracked.Put(blob.WithOwner(ctx, a), "only-a", []byte("1"))
	_ = tracked.Put(blob.WithOwner(ctx, a), "shared", []byte("2"))
	if _, ok, _ := tracked.Get(blob.WithOwner(ctx, b), "shared"); !ok {
		t.Fatal("shared miss")
	}
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/cases/"+a, "", user("dave")); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
	if raw.has("only-a") || !raw.has("shared") {
		t.Fatalf("after A: only-a=%v shared=%v", raw.has("only-a"), raw.has("shared"))
	}
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/cases/"+b, "", user("dave")); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
	if raw.has("shared") {
		t.Fatal("shared entry kept after its last case went")
	}
}

func TestUserDeleteSucceedsWhenTheCachePurgeFails(t *testing.T) {
	raw := &memBlob{m: map[string][]byte{}}
	refs := blob.NewMemoryRefs()
	tracked := blob.Track(raw, blob.StoreVerdicts, refs)
	e := newAuditEnvWith(t, true, func(d *Deps) {
		d.CachePurge = &blob.Purger{Refs: refs, Stores: map[string]blob.Store{blob.StoreVerdicts: noDelete{tracked}}}
	})
	a := e.wsCase()
	_ = tracked.Put(blob.WithOwner(context.Background(), a), "k", []byte("1"))
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/cases/"+a, "", user("dave")); code != http.StatusNoContent {
		t.Fatalf("a cache failure blocked the delete: %d", code)
	}
	// It stays findable: the sweep retries it.
	p := blob.Purger{Refs: refs, Stores: map[string]blob.Store{blob.StoreVerdicts: tracked}}
	rep, err := p.Orphans(context.Background(), func(context.Context, string) (bool, error) { return true, nil }, false)
	if err != nil || rep.Deleted != 1 || raw.has("k") {
		t.Fatalf("sweep %+v %v", rep, err)
	}
}

func TestDeleteWithoutCachePurgeChangesNothing(t *testing.T) {
	e := newAuditEnv(t)
	a := e.wsCase()
	if code, _ := e.hs.doAs(t, "DELETE", "/v1/cases/"+a, "", user("dave")); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
}

func TestRetentionAuditDays(t *testing.T) {
	e := newAuditEnv(t) // the trail's own window is the 90 day default
	path := "/v1/workspaces/" + e.wsID + "/retention"
	if code, _ := e.hs.doAs(t, "PUT", path, `{"case_days":30}`, user("carol")); code != http.StatusOK {
		t.Fatal(code)
	}
	for _, body := range []string{`{"audit_days":3}`, `{"audit_days":91}`, `{"audit_days":-1}`, `{"audit_days":"30"}`} {
		if code, _ := e.hs.doAs(t, "PUT", path, body, user("carol")); code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d", body, code)
		}
	}
	if code, _ := e.hs.doAs(t, "PUT", path, `{"audit_days":30}`, user("dave")); code != http.StatusForbidden {
		t.Fatalf("editor = %d", code)
	}
	if code, _ := e.hs.doAs(t, "PUT", "/v1/workspaces/"+e.zed+"/retention", `{"audit_days":30}`, user("carol")); code != http.StatusNotFound {
		t.Fatalf("other workspace = %d", code)
	}
	code, out := e.hs.doAs(t, "PUT", path, `{"audit_days":30}`, user("carol"))
	if code != http.StatusOK || out["audit_days"] != float64(30) || out["case_days"] != float64(30) {
		t.Fatalf("audit_days alone must keep case_days: %d %v", code, out)
	}
	code, out = e.hs.doAs(t, "PUT", path, `{"case_days":60}`, user("carol"))
	if code != http.StatusOK || out["audit_days"] != float64(30) || out["case_days"] != float64(60) {
		t.Fatalf("case_days alone must keep audit_days: %d %v", code, out)
	}
	if code, out = e.hs.doAs(t, "GET", path, "", user("vera")); code != http.StatusOK || out["audit_days"] != float64(30) {
		t.Fatalf("GET %d %v", code, out)
	}
	if code, out = e.hs.doAs(t, "PUT", path, `{"audit_days":0}`, user("carol")); code != http.StatusOK || out["audit_days"] != float64(0) {
		t.Fatalf("clear %d %v", code, out)
	}
	var saw bool
	for _, ev := range e.events(e.tenant()) {
		saw = saw || (ev.Action == audit.WorkspaceRetention && ev.Detail == "audit" && ev.Phase == audit.PhaseCompletion)
	}
	if !saw {
		t.Fatal("no workspace.retention completion for an audit-only change")
	}
}

func TestAuditDaysNeedsTheAuditTrail(t *testing.T) {
	e := newAuditEnvWith(t, true, func(d *Deps) { d.Audit = nil })
	path := "/v1/workspaces/" + e.wsID + "/retention"
	if code, _ := e.hs.doAs(t, "PUT", path, `{"audit_days":30}`, user("carol")); code != http.StatusBadRequest {
		t.Fatalf("audit_days with the trail off = %d", code)
	}
	if code, _ := e.hs.doAs(t, "PUT", path, `{"case_days":30}`, user("carol")); code != http.StatusOK {
		t.Fatalf("case_days with the trail off = %d", code)
	}
}
