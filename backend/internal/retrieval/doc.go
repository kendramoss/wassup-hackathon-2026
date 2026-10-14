// Package retrieval finds earlier CBP rulings similar to a case. Retriever is
// the seam: BM25 now, Vertex embeddings can be added behind the same
// interface. Every search takes a filter so callers enforce the precedent
// rule (strictly earlier than the case, never the case itself).
package retrieval
