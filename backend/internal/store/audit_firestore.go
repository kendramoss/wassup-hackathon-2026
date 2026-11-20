package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
)

// Audit collections (#284). One document per tenant (the chain's head and its
// retention checkpoint) and, under it, one document per event whose ID is the
// zero-padded sequence. Nothing here needs a composite index: events are read
// with a range filter and an order on the single field seq.
//
// STATUS: written against the Firestore client API and inspected, NOT run
// against an emulator or a live database (none is available in this
// workspace; the emulator test below skips without FIRESTORE_EMULATOR_HOST).
// No TTL policy, IAM binding or index is created or assumed by this code.
const (
	AuditCollection = "audit_tenants"
	auditEventsSub  = "events"
	// auditMaxAttempts bounds the client library's retries of an aborted
	// (contended) transaction; the head document of a busy tenant is hot.
	auditMaxAttempts = 5
	// auditPurgeMax keeps a purge transaction (events deleted + the tenant
	// document write) below Firestore's 500-write transaction limit.
	auditPurgeMax = 400
)

// Audit returns the audit store on the same client.
func (f *Firestore) Audit() *FirestoreAudit {
	return &FirestoreAudit{client: f.client, tenants: f.client.Collection(AuditCollection)}
}

// FirestoreAudit implements audit.Store.
type FirestoreAudit struct {
	client  *firestore.Client
	tenants *firestore.CollectionRef
}

// auditTenantDoc is the tenant document: the head and the checkpoint.
type auditTenantDoc struct {
	Tenant   string    `firestore:"tenant"`
	HeadSeq  int64     `firestore:"head_seq"`
	HeadHash string    `firestore:"head_hash"`
	HeadTime time.Time `firestore:"head_time"`
	CPSeq    int64     `firestore:"cp_seq,omitempty"`
	CPHash   string    `firestore:"cp_hash,omitempty"`
	CPTime   time.Time `firestore:"cp_time,omitempty"`
	PurgedAt time.Time `firestore:"purged_at,omitempty"`
}

// auditEventDoc is one event: the JSON that was hashed, plus seq and time for
// ordering and range reads.
type auditEventDoc struct {
	Seq   int64     `firestore:"seq"`
	Time  time.Time `firestore:"time"`
	Event string    `firestore:"event"`
}

func tenantKey(tenant string) string {
	sum := sha256.Sum256([]byte(tenant))
	return hex.EncodeToString(sum[:16])
}

func eventID(seq uint64) string { return fmt.Sprintf("%012d", seq) }

func toEventDoc(e audit.Event) (auditEventDoc, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return auditEventDoc{}, err
	}
	return auditEventDoc{Seq: int64(e.Seq), Time: e.Time, Event: string(b)}, nil
}

func fromEventDoc(d auditEventDoc) (audit.Event, error) {
	var e audit.Event
	if err := json.Unmarshal([]byte(d.Event), &e); err != nil {
		return audit.Event{}, fmt.Errorf("store: audit event: %w", err)
	}
	return e, nil
}

func (d auditTenantDoc) head() audit.Head {
	return audit.Head{Seq: uint64(d.HeadSeq), Hash: d.HeadHash, Time: d.HeadTime.UTC()}
}

func (d auditTenantDoc) checkpoint() *audit.Checkpoint {
	if d.CPSeq == 0 {
		return nil
	}
	return &audit.Checkpoint{ThroughSeq: uint64(d.CPSeq), ThroughHash: d.CPHash, ThroughTime: d.CPTime.UTC(), PurgedAt: d.PurgedAt.UTC()}
}

// Append implements audit.Store: the head read, the event create and the head
// update are one transaction, so concurrent appends get consecutive sequence
// numbers. An exhausted retry is an error (a sensitive write then fails
// closed). The transaction function may run again, so seal must be pure.
func (s *FirestoreAudit) Append(ctx context.Context, tenant string, seal func(audit.Head) (audit.Event, error)) (audit.Event, error) {
	tdoc := s.tenants.Doc(tenantKey(tenant))
	var out audit.Event
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		var head audit.Head
		snap, err := tx.Get(tdoc)
		switch {
		case err == nil:
			var td auditTenantDoc
			if err := snap.DataTo(&td); err != nil {
				return fmt.Errorf("decode tenant: %w", err)
			}
			head = td.head()
		case status.Code(err) != codes.NotFound:
			return err
		}
		e, err := seal(head)
		if err != nil {
			return err
		}
		if e.Tenant != tenant || e.Seq != head.Seq+1 {
			return audit.ErrInvalid
		}
		doc, err := toEventDoc(e)
		if err != nil {
			return err
		}
		if err := tx.Create(tdoc.Collection(auditEventsSub).Doc(eventID(e.Seq)), doc); err != nil {
			return err
		}
		out = e
		return tx.Set(tdoc, map[string]any{"tenant": tenant, "head_seq": int64(e.Seq), "head_hash": e.Hash, "head_time": e.Time}, firestore.MergeAll)
	}, firestore.MaxAttempts(auditMaxAttempts))
	if err != nil {
		if errors.Is(err, audit.ErrInvalid) {
			return audit.Event{}, err
		}
		return audit.Event{}, fmt.Errorf("store: audit append: %w", err)
	}
	return out, nil
}

// Range implements audit.Store.
func (s *FirestoreAudit) Range(ctx context.Context, tenant string, r audit.Range) ([]audit.Event, error) {
	q := s.tenants.Doc(tenantKey(tenant)).Collection(auditEventsSub).Query
	if r.Desc {
		if r.Cursor > 0 {
			q = q.Where("seq", "<", int64(r.Cursor))
		}
		q = q.OrderBy("seq", firestore.Desc)
	} else {
		q = q.Where("seq", ">", int64(r.Cursor)).OrderBy("seq", firestore.Asc)
	}
	it := q.Limit(r.Limit).Documents(ctx)
	defer it.Stop()
	var out []audit.Event
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("store: audit range: %w", err)
		}
		var d auditEventDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, fmt.Errorf("store: audit range: %w", err)
		}
		e, err := fromEventDoc(d)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
}

// State implements audit.Store.
func (s *FirestoreAudit) State(ctx context.Context, tenant string) (audit.Head, *audit.Checkpoint, error) {
	snap, err := s.tenants.Doc(tenantKey(tenant)).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return audit.Head{}, nil, nil
	}
	if err != nil {
		return audit.Head{}, nil, fmt.Errorf("store: audit state: %w", err)
	}
	var td auditTenantDoc
	if err := snap.DataTo(&td); err != nil {
		return audit.Head{}, nil, fmt.Errorf("store: audit state: %w", err)
	}
	return td.head(), td.checkpoint(), nil
}

// Purge implements audit.Store. One transaction reads the oldest events, then
// records the checkpoint on the tenant document and deletes those events, so
// there is no window in which events are gone and the checkpoint is not
// written. limit is clamped to auditPurgeMax.
func (s *FirestoreAudit) Purge(ctx context.Context, tenant string, cutoff, now time.Time, limit int) (audit.PurgeResult, error) {
	if limit <= 0 || limit > auditPurgeMax {
		limit = auditPurgeMax
	}
	tdoc := s.tenants.Doc(tenantKey(tenant))
	var res audit.PurgeResult
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		res = audit.PurgeResult{}
		snap, err := tx.Get(tdoc)
		if status.Code(err) == codes.NotFound {
			return nil
		}
		if err != nil {
			return err
		}
		var td auditTenantDoc
		if err := snap.DataTo(&td); err != nil {
			return err
		}
		res.Checkpoint = td.checkpoint()
		snaps, err := tx.Documents(tdoc.Collection(auditEventsSub).OrderBy("seq", firestore.Asc).Limit(limit)).GetAll()
		if err != nil {
			return err
		}
		var del []*firestore.DocumentRef
		var last audit.Event
		for _, sn := range snaps {
			var d auditEventDoc
			if err := sn.DataTo(&d); err != nil {
				return err
			}
			e, err := fromEventDoc(d)
			if err != nil {
				return err
			}
			if !e.Time.Before(cutoff) {
				break
			}
			del, last = append(del, sn.Ref), e
		}
		if len(del) == 0 {
			return nil
		}
		cp := audit.Checkpoint{ThroughSeq: last.Seq, ThroughHash: last.Hash, ThroughTime: last.Time, PurgedAt: now.UTC()}
		if err := tx.Set(tdoc, map[string]any{"cp_seq": int64(cp.ThroughSeq), "cp_hash": cp.ThroughHash, "cp_time": cp.ThroughTime, "purged_at": cp.PurgedAt}, firestore.MergeAll); err != nil {
			return err
		}
		for _, ref := range del {
			if err := tx.Delete(ref); err != nil {
				return err
			}
		}
		res = audit.PurgeResult{Deleted: len(del), Checkpoint: &cp}
		return nil
	}, firestore.MaxAttempts(auditMaxAttempts))
	if err != nil {
		return audit.PurgeResult{}, fmt.Errorf("store: audit purge: %w", err)
	}
	return res, nil
}

// Tenants implements audit.Store.
func (s *FirestoreAudit) Tenants(ctx context.Context) ([]string, error) {
	it := s.tenants.Documents(ctx)
	defer it.Stop()
	var out []string
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("store: audit tenants: %w", err)
		}
		var td auditTenantDoc
		if snap.DataTo(&td) == nil && audit.ValidTenant(td.Tenant) {
			out = append(out, td.Tenant)
		}
	}
}

var _ audit.Store = (*FirestoreAudit)(nil)
