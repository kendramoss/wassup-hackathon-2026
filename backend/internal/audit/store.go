package audit

import (
	"context"
	"time"
)

// Range selects events of one tenant.
//
// Ascending (Desc false): events with Seq > Cursor, oldest first.
// Descending: events with Seq < Cursor, newest first; Cursor 0 starts at the head.
// Limit caps the result (> 0).
type Range struct {
	Cursor uint64
	Desc   bool
	Limit  int
}

// PurgeResult is what a purge removed.
type PurgeResult struct {
	Deleted    int
	Checkpoint *Checkpoint // the tenant's checkpoint after the purge, nil if it has none
}

// Store keeps per-tenant chains. Implementations must make Append atomic per
// tenant: the head is read, the event sealed and written as one step, so
// concurrent appends get distinct, consecutive sequence numbers and a
// consistent chain. Memory is process-local, File is a single-process local
// store (two processes sharing a directory are not supported), and the
// Firestore adapter does this in one transaction on the tenant document. Only
// the memory and file stores are exercised by tests here.
type Store interface {
	// Append calls seal with the tenant's current head and stores the event it
	// returns. seal may be called more than once (a retried transaction) and
	// must be pure.
	Append(ctx context.Context, tenant string, seal func(Head) (Event, error)) (Event, error)
	// Range reads events (see Range).
	Range(ctx context.Context, tenant string, r Range) ([]Event, error)
	// State returns the tenant's head (Seq 0: no events yet) and its retention
	// checkpoint (nil: nothing was ever purged).
	State(ctx context.Context, tenant string) (Head, *Checkpoint, error)
	// Purge removes the oldest events whose Time is before cutoff, a
	// time-contiguous prefix of at most limit events (<= 0: store default),
	// after recording the checkpoint that covers them. It never changes the
	// head. now stamps Checkpoint.PurgedAt.
	Purge(ctx context.Context, tenant string, cutoff, now time.Time, limit int) (PurgeResult, error)
	// Tenants lists the tenants that have a chain.
	Tenants(ctx context.Context) ([]string, error)
}

// DefaultPurgeChunk bounds the events one purge step deletes; the Firestore
// adapter keeps a step within a transaction's write limit.
const DefaultPurgeChunk = 400
