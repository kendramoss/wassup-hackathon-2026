package store

import (
	"fmt"
	"testing"
	"time"
)

// Documents written before the pinned and owner fields existed carry them
// only inside the encoded case. ListRecent(0) and ListOwner share scanAll's
// decode, so these tests run the same decode (decodeDoc) and filters
// (pinnedOf, ownedBy) over old-style and stale-metadata documents.
func TestLegacyPinnedDocsAreFoundFromTheDecodedCase(t *testing.T) {
	now := time.Now().UTC()
	decode := func(c Case, mutate func(*firestoreDoc)) Case {
		t.Helper()
		gz, err := gzipJSON(c)
		if err != nil {
			t.Fatal(err)
		}
		d := docOf(c, gz)
		if mutate != nil {
			mutate(&d)
		}
		got, err := decodeDoc(d)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	noTop := func(d *firestoreDoc) { d.Pinned, d.Owner = false, "" } // as written before those fields existed
	stalePin := func(d *firestoreDoc) { d.Pinned = true }            // top-level says pinned, the case is not
	docs := []Case{
		decode(Case{ID: "case_demo_legacy", Pinned: true, UpdatedAt: now.Add(-90 * 24 * time.Hour)}, noTop),
		decode(Case{ID: "case_demo_new", Pinned: true, UpdatedAt: now.Add(-time.Hour)}, nil),
		decode(Case{ID: "case_private_stale", Owner: "alice", UpdatedAt: now}, stalePin),
		decode(Case{ID: "case_private_legacy", Owner: "alice", UpdatedAt: now.Add(-time.Minute)}, noTop),
		decode(Case{ID: "case_other", Owner: "bob", UpdatedAt: now.Add(-2 * time.Minute)}, nil),
	}
	got := pinnedOf(docs)
	// Pinned first by the shared sort: newest pinned first.
	if fmt.Sprint(ids(got)) != "[case_demo_new case_demo_legacy]" {
		t.Errorf("pinnedOf = %v, want both real demos and no stale-metadata private case", ids(got))
	}
	// Owner scope on the same decoded documents: legacy owner found, stale pin does not matter.
	if fmt.Sprint(ids(ownedBy(docs, "alice", 10))) != "[case_private_stale case_private_legacy]" {
		t.Errorf("ownedBy alice = %v", ids(ownedBy(docs, "alice", 10)))
	}
	if len(pinnedOf(nil)) != 0 {
		t.Error("no documents, no pinned cases")
	}
}
