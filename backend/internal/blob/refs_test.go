package blob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	caseA = "case_aaaaaaaaaaaa"
	caseB = "case_bbbbbbbbbbbb"
)

// memBlobs is a Store and Deleter in memory.
type memBlobs struct {
	m       map[string][]byte
	failDel bool
}

func newMem() *memBlobs { return &memBlobs{m: map[string][]byte{}} }
func (b *memBlobs) Get(_ context.Context, k string) ([]byte, bool, error) {
	v, ok := b.m[k]
	return v, ok, nil
}
func (b *memBlobs) Put(_ context.Context, k string, v []byte) error { b.m[k] = v; return nil }
func (b *memBlobs) Delete(_ context.Context, k string) error {
	if b.failDel {
		return errors.New("down")
	}
	delete(b.m, k)
	return nil
}

func setup() (*memBlobs, Store, Purger, *MemoryRefs) {
	raw, refs := newMem(), NewMemoryRefs()
	tr := Track(raw, StoreVerdicts, refs)
	return raw, tr, Purger{Refs: refs, Stores: map[string]Store{StoreVerdicts: tr}}, refs
}

func TestOwnerOnlyForServerMintedCaseIDs(t *testing.T) {
	if _, ok := OwnerOf(WithOwner(context.Background(), "demo-kobalt")); ok {
		t.Fatal("a non-case id became an owner")
	}
	if id, ok := OwnerOf(WithOwner(context.Background(), caseA)); !ok || id != caseA {
		t.Fatal("case id not kept")
	}
}

func TestEntryUsedByTwoCasesSurvivesUntilTheLastIsPurged(t *testing.T) {
	ctx := context.Background()
	raw, tr, p, _ := setup()
	_ = tr.Put(WithOwner(ctx, caseA), "k1", []byte("v"))
	if _, ok, _ := tr.Get(WithOwner(ctx, caseB), "k1"); !ok { // B hits A's entry
		t.Fatal("miss")
	}
	_ = tr.Put(WithOwner(ctx, caseA), "only-a", []byte("v"))
	rep, err := p.PurgeCase(ctx, caseA, nil)
	if err != nil || rep.Deleted != 1 || rep.Shared != 1 {
		t.Fatalf("A: %+v %v", rep, err)
	}
	if _, ok := raw.m["k1"]; !ok {
		t.Fatal("shared entry deleted while B still uses it")
	}
	if _, ok := raw.m["only-a"]; ok {
		t.Fatal("A's own entry kept")
	}
	rep, err = p.PurgeCase(ctx, caseB, nil)
	if err != nil || rep.Deleted != 1 || len(raw.m) != 0 {
		t.Fatalf("B: %+v %v left %v", rep, err, raw.m)
	}
	// A second purge of the same case finds nothing.
	if rep, _ := p.PurgeCase(ctx, caseB, nil); rep != (PurgeReport{}) {
		t.Fatalf("again: %+v", rep)
	}
}

func TestUnownedUseIsNotLinked(t *testing.T) {
	ctx := context.Background()
	raw, tr, p, refs := setup()
	_ = tr.Put(ctx, "k", []byte("v"))
	if ids, _ := refs.Cases(ctx); len(ids) != 0 {
		t.Fatalf("linked %v", ids)
	}
	if rep, _ := p.PurgeCase(ctx, caseA, nil); rep.Deleted != 0 || len(raw.m) != 1 {
		t.Fatalf("an entry no case owns was purged: %+v", rep)
	}
}

type failRefs struct{ *MemoryRefs }

func (failRefs) Add(context.Context, Ref, string) error { return errors.New("index down") }

func TestNoLinkNoWriteAndNoServedRead(t *testing.T) {
	ctx := WithOwner(context.Background(), caseA)
	raw := newMem()
	raw.m["old"] = []byte("v")
	tr := Track(raw, StoreVerdicts, failRefs{NewMemoryRefs()})
	if err := tr.Put(ctx, "new", []byte("v")); err == nil || len(raw.m) != 1 {
		t.Fatalf("an entry was written without a link: %v %v", err, raw.m)
	}
	if _, ok, err := tr.Get(ctx, "old"); ok || err != nil {
		t.Fatalf("an untracked read was served: ok=%v err=%v", ok, err)
	}
}

func TestExtraEntriesAreDeletedUnlessAnotherCaseHasThem(t *testing.T) {
	ctx := context.Background()
	raw, tr, p, _ := setup()
	raw.m["supplier-doc-x"], raw.m["supplier-doc-y"] = []byte("1"), []byte("2")
	_ = tr.Put(WithOwner(ctx, caseB), "supplier-doc-y", []byte("2")) // B shares y
	rep, err := p.PurgeCase(ctx, caseA, []Ref{{StoreVerdicts, "supplier-doc-x"}, {StoreVerdicts, "supplier-doc-y"}})
	if err != nil {
		t.Fatal(err)
	}
	// y is listed for A only through "extra" and B still uses it: the index
	// has no entry for A on y, so B's link is what must save it.
	if _, ok := raw.m["supplier-doc-x"]; ok {
		t.Fatal("x kept")
	}
	if _, ok := raw.m["supplier-doc-y"]; !ok {
		t.Fatalf("y deleted although B uses it: %+v", rep)
	}
}

func TestFailedDeleteIsRetriedByTheOrphanSweep(t *testing.T) {
	ctx := context.Background()
	raw, tr, p, _ := setup()
	_ = tr.Put(WithOwner(ctx, caseA), "k", []byte("v"))
	raw.failDel = true
	rep, err := p.PurgeCase(ctx, caseA, nil)
	if err == nil || rep.Failed != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	raw.failDel = false
	exists := func(context.Context, string) (bool, error) { return true, nil }
	dry, err := p.Orphans(ctx, exists, true)
	if err != nil || dry.Cases != 1 || len(raw.m) != 1 {
		t.Fatalf("dry %+v %v", dry, err)
	}
	got, err := p.Orphans(ctx, exists, false)
	if err != nil || got.Deleted != 1 || len(raw.m) != 0 {
		t.Fatalf("sweep %+v %v", got, err)
	}
}

func TestOrphansFindOnlyVanishedCases(t *testing.T) {
	ctx := context.Background()
	raw, tr, p, _ := setup()
	_ = tr.Put(WithOwner(ctx, caseA), "a", []byte("v"))
	_ = tr.Put(WithOwner(ctx, caseB), "b", []byte("v"))
	exists := func(_ context.Context, id string) (bool, error) { return id == caseB, nil }
	rep, err := p.Orphans(ctx, exists, false)
	if err != nil || rep.Cases != 1 || rep.Deleted != 1 {
		t.Fatalf("%+v %v", rep, err)
	}
	if _, ok := raw.m["b"]; !ok || len(raw.m) != 1 {
		t.Fatalf("left %v", raw.m)
	}
}

func TestNoDeleterIsAFailureNotASilentKeep(t *testing.T) {
	ctx := context.Background()
	refs := NewMemoryRefs()
	_ = refs.Add(ctx, Ref{StoreVerdicts, "k"}, caseA)
	p := Purger{Refs: refs, Stores: map[string]Store{StoreVerdicts: onlyGetPut{}}}
	rep, _ := p.PurgeCase(ctx, caseA, nil)
	if rep.Failed != 1 {
		t.Fatalf("%+v", rep)
	}
}

type onlyGetPut struct{}

func (onlyGetPut) Get(context.Context, string) ([]byte, bool, error) { return nil, false, nil }
func (onlyGetPut) Put(context.Context, string, []byte) error         { return nil }

func TestFileRefsPersistAndDirDelete(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r, err := NewFileRefs(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Add(ctx, Ref{StoreEngine, "k"}, caseA)
	r2, err := NewFileRefs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ids, _ := r2.Cases(ctx); len(ids) != 1 || ids[0] != caseA {
		t.Fatalf("reloaded %v", ids)
	}
	d := Dir{Dir: dir}
	_ = d.Put(ctx, "blobkey", []byte("x"))
	if err := d.Delete(ctx, "blobkey"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "blobkey.json")); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
	if err := d.Delete(ctx, "blobkey"); err != nil {
		t.Fatalf("deleting a missing key: %v", err)
	}
}
