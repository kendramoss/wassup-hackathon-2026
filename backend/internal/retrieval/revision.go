package retrieval

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"sort"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
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

// WithRevision records the corpus revision (corpus.Store.Revision(), the
// single canonical hash of every effective evidence field) of the store idx
// was built from. The index may cover only part of that store or carry
// reasoning attached afterwards, so the reported revision also folds in the
// indexed ruling ids and their Reasoning; the corpus hash itself is not
// recomputed. An empty storeRev leaves idx unversioned (Revision "").
func (idx *BM25) WithRevision(storeRev string) *BM25 {
	if storeRev == "" {
		idx.rev = ""
		return idx
	}
	docs := make([]domain.Ruling, len(idx.docs))
	copy(docs, idx.docs)
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	h := sha256.New()
	writeField(h, "bm25/v1")
	writeField(h, storeRev)
	for _, d := range docs {
		writeField(h, d.ID)
		writeField(h, d.Reasoning)
	}
	idx.rev = hex.EncodeToString(h.Sum(nil))
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
