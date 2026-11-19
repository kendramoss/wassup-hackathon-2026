package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	firestorepb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

// plainCases hides every optional lister of the store it wraps, like a
// third-party Cases that only implements the base interface.
type plainCases struct {
	Cases
	lists int
}

func (p *plainCases) List(ctx context.Context) ([]Case, error) {
	p.lists++
	return p.Cases.List(ctx)
}

// ownerCounting is a store with its own ListOwner that must be the only read.
type ownerCounting struct {
	*Memory
	mu    sync.Mutex
	lists int
	owner []string
	ns    []int
}

func (o *ownerCounting) List(ctx context.Context) ([]Case, error) {
	o.mu.Lock()
	o.lists++
	o.mu.Unlock()
	return o.Memory.List(ctx)
}

func (o *ownerCounting) ListOwner(ctx context.Context, owner string, n int) ([]Case, error) {
	o.mu.Lock()
	o.owner, o.ns = append(o.owner, owner), append(o.ns, n)
	o.mu.Unlock()
	return o.Memory.ListOwner(ctx, owner, n)
}

func seedOwners(t *testing.T, cs Cases) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	put := func(id, owner string, pinned bool, age time.Duration) {
		if err := cs.Put(ctx, Case{ID: id, Owner: owner, Pinned: pinned, UpdatedAt: now.Add(-age)}); err != nil {
			t.Fatal(err)
		}
	}
	put("case_a_old", "alice", false, 3*time.Hour)
	put("case_a_mid", "alice", false, 2*time.Hour)
	put("case_a_new", "alice", false, time.Hour)
	put("case_a_pin", "alice", true, 24*time.Hour)
	put("case_b_new", "bob", false, time.Minute) // newer than all of alice's
	put("case_anon", "", false, time.Minute)
	put("case_demo", "", true, 48*time.Hour)
}

func ids(cs []Case) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func TestListOwnerScopesBeforeTheCap(t *testing.T) {
	file, err := NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for name, cs := range map[string]Cases{"memory": NewMemory(), "file": file, "plain": &plainCases{Cases: NewMemory()}} {
		seedOwners(t, cs)
		got, err := ListOwner(context.Background(), cs, "alice", 2)
		// alice's pinned case is kept, then her 2 newest; bob's newer case never appears.
		want := "[case_a_pin case_a_new case_a_mid]"
		if err != nil || fmt.Sprint(ids(got)) != want {
			t.Errorf("%s ListOwner = %v %v, want %s", name, ids(got), err, want)
		}
		if got, err := ListOwner(context.Background(), cs, "", 5); err != nil || len(got) != 0 {
			t.Errorf("%s empty owner listed %v %v", name, ids(got), err)
		}
		if got, err := ListOwner(context.Background(), cs, "nobody", 5); err != nil || len(got) != 0 {
			t.Errorf("%s unknown owner listed %v %v", name, ids(got), err)
		}
	}
}

func TestListOwnerUsesTheStoresOwnQueryNotAFullScan(t *testing.T) {
	oc := &ownerCounting{Memory: NewMemory()}
	seedOwners(t, oc)
	got, err := ListOwner(context.Background(), oc, "alice", 2)
	if err != nil || len(got) != 3 {
		t.Fatalf("ListOwner = %v %v", ids(got), err)
	}
	if oc.lists != 0 || len(oc.owner) != 1 || oc.owner[0] != "alice" || oc.ns[0] != 2 {
		t.Errorf("lists=%d owner=%v n=%v; want no List and one bounded ListOwner", oc.lists, oc.owner, oc.ns)
	}
	if _, err := ListOwner(context.Background(), oc, "", 2); err != nil || len(oc.owner) != 1 {
		t.Errorf("empty owner reached the store: %v %v", err, oc.owner)
	}
}

func TestListOwnerFallbackReadsOnceAndDoesNotWrite(t *testing.T) {
	p := &plainCases{Cases: NewMemory()}
	seedOwners(t, p)
	before, _ := p.Cases.List(context.Background())
	p.lists = 0
	if _, err := ListOwner(context.Background(), p, "alice", 1); err != nil || p.lists != 1 {
		t.Errorf("fallback List calls = %d %v, want 1", p.lists, err)
	}
	after, _ := p.Cases.List(context.Background())
	if fmt.Sprint(ids(before)) != fmt.Sprint(ids(after)) {
		t.Errorf("listing wrote: %v -> %v", ids(before), ids(after))
	}
}

func TestListRecentZeroIsPinnedOnly(t *testing.T) {
	m := NewMemory()
	seedOwners(t, m)
	got, err := ListRecent(context.Background(), m, 0)
	if err != nil || fmt.Sprint(ids(got)) != "[case_demo case_a_pin]" && fmt.Sprint(ids(got)) != "[case_a_pin case_demo]" {
		t.Errorf("ListRecent(0) = %v %v", ids(got), err)
	}
}

func TestMemoryListOwnerIsSafeUnderConcurrentWrites(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = m.Put(ctx, Case{ID: fmt.Sprintf("case_%d_%d", w, i), Owner: "alice", UpdatedAt: time.Now()})
				if got, err := m.ListOwner(ctx, "alice", 3); err != nil || len(got) > 3 {
					t.Errorf("ListOwner = %d %v", len(got), err)
					return
				}
			}
		}(w)
	}
	wg.Wait()
}

// An offline client builds queries without any network or credentials; the
// query is only serialized, never run.
func offlineFirestore(t *testing.T) *Firestore {
	t.Helper()
	c, err := firestore.NewClient(context.Background(), "owner-query-test",
		option.WithoutAuthentication(), option.WithEndpoint("localhost:1"),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
	if err != nil {
		t.Skipf("offline firestore client unavailable: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return NewFirestoreFromClient(c)
}

func TestFirestoreOwnerQueryIsOwnerEqualityOnly(t *testing.T) {
	f := offlineFirestore(t)
	b, err := f.ownerQuery("owner_hash").Serialize()
	if err != nil {
		t.Fatal(err)
	}
	var req firestorepb.RunQueryRequest
	if err := proto.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	q := req.GetStructuredQuery()
	fl := q.GetWhere().GetFieldFilter()
	if fl == nil || fl.GetField().GetFieldPath() != "owner" || fl.GetOp() != firestorepb.StructuredQuery_FieldFilter_EQUAL ||
		fl.GetValue().GetStringValue() != "owner_hash" {
		t.Errorf("where = %v, want owner == owner_hash", q.GetWhere())
	}
	// No OrderBy (that needs a composite index with the equality) and no
	// Limit (the cap is applied after the app-side sort).
	if len(q.GetOrderBy()) != 0 || q.GetLimit() != nil {
		t.Errorf("owner query has order %v / limit %v; it must be a single-field equality only", q.GetOrderBy(), q.GetLimit())
	}
	if fs := q.GetFrom(); len(fs) != 1 || fs[0].GetCollectionId() != FirestoreCollection {
		t.Errorf("from = %v", fs)
	}
}

func TestFirestoreDocCarriesOwnerForTheQuery(t *testing.T) {
	d := docOf(Case{ID: "case_x", Owner: "o1"}, nil)
	if d.Owner != "o1" {
		t.Errorf("owner not stored: %+v", d)
	}
}
