package retrieval

import (
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

func revRulings(reason string) []domain.Ruling {
	d := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	return []domain.Ruling{{ID: "N100002", Date: d, Facts: "b"}, {ID: "N100001", Date: d, Facts: "a", Reasoning: reason}}
}

func TestRevisionStampedAndUnstamped(t *testing.T) {
	text := func(r domain.Ruling) string { return r.Facts }
	if RevisionOf(NewBM25(revRulings(""), text)) != "" {
		t.Error("unstamped BM25 claims a revision")
	}
	a := NewBM25(revRulings(""), text).WithRevision("s1").Revision()
	if a == "" || a != NewBM25(revRulings(""), text).WithRevision("s1").Revision() {
		t.Error("revision not stable")
	}
	if a == NewBM25(revRulings(""), text).WithRevision("s2").Revision() {
		t.Error("store revision ignored")
	}
	if NewBM25(revRulings(""), text).WithRevision("s1").WithRevision("").Revision() != "" {
		t.Error("empty store revision should leave index unversioned")
	}
	v := NewVectors("m", 1)
	v.Add("N100001", "h", []float32{1})
	h := &Hybrid{BM25: NewBM25(revRulings(""), text).WithRevision("s1"), Vectors: v}
	if h.Revision() == "" || h.Revision() == a {
		t.Error("hybrid revision missing or equal to BM25's")
	}
	if (&Hybrid{BM25: NewBM25(revRulings(""), text), Vectors: v}).Revision() != "" {
		t.Error("hybrid over unstamped BM25 claims a revision")
	}
}
