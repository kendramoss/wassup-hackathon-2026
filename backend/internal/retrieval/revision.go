package retrieval

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"sort"
)

// Revisioned is implemented by retrievers that can name the exact evidence
// they search. Revision is immutable and computed once at construction,
// never per request. An empty string means the retriever cannot vouch for
// its evidence: callers must not reuse persisted answers keyed on it.
type Revisioned interface {
	Revision() string
}

// RevisionOf returns r's evidence revision, or "" when r does not report a
// trustworthy one.
func RevisionOf(r Retriever) string {
	if v, ok := r.(Revisioned); ok {
		return v.Revision()
	}
	return ""
}

// WithRevision records the revision of the exact rulings idx was built
// from. Pass corpus.RevisionOf(rulings) over the same slice given to NewBM25
// (the single canonical producer, #317; it covers every field the index and
// prompt read, including Reasoning attached by WithReasoning). It is stored
// verbatim: no hashing happens here. An empty rev leaves idx unversioned.
func (idx *BM25) WithRevision(rev string) *BM25 {
	idx.rev = rev
	return idx
}

// Revision implements Revisioned.
func (idx *BM25) Revision() string { return idx.rev }

// Revision implements Revisioned. The ranking also depends on the embedding
// index and the candidate cut, so those are part of the identity; it is ""
// when the BM25 side is unversioned. Computed once on first use.
func (h *Hybrid) Revision() string {
	h.revOnce.Do(func() {
		base := RevisionOf(h.BM25)
		if base == "" || h.Vectors == nil {
			return
		}
		hh := sha256.New()
		writeField(hh, "hybrid/v1")
		writeField(hh, base)
		writeField(hh, h.Vectors.Model)
		var n [8]byte
		binary.LittleEndian.PutUint64(n[:], uint64(h.Vectors.Dims))
		hh.Write(n[:])
		binary.LittleEndian.PutUint64(n[:], uint64(h.Candidates))
		hh.Write(n[:])
		ids := make([]string, 0, len(h.Vectors.byID))
		for id := range h.Vectors.byID {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			writeField(hh, id)
			for _, f := range h.Vectors.byID[id] {
				binary.LittleEndian.PutUint32(n[:4], math.Float32bits(f))
				hh.Write(n[:4])
			}
		}
		h.rev = hex.EncodeToString(hh.Sum(nil))
	})
	return h.rev
}

type hasher interface{ Write([]byte) (int, error) }

func writeField(h hasher, s string) {
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(len(s)))
	h.Write(n[:])
	h.Write([]byte(s))
}
