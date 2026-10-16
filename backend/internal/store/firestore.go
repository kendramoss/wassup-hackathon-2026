package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// FirestoreCollection holds one document per case.
const FirestoreCollection = "cases"

// Firestore stores cases in Cloud Firestore so they survive Cloud Run
// restarts and are shared between instances (D25). Each document holds the
// case as gzipped JSON plus updated_at for ordering: a case with full plan
// and target searches can be large, and the gzip keeps it well under
// Firestore's 1 MiB document limit.
type Firestore struct {
	client *firestore.Client
	col    *firestore.CollectionRef
}

// firestoreDoc is the stored shape of a case.
type firestoreDoc struct {
	ID        string    `firestore:"id"`
	UpdatedAt time.Time `firestore:"updated_at"`
	Case      []byte    `firestore:"case_json_gz"`
}

// NewFirestore connects to database (empty means "(default)") in project
// with Application Default Credentials.
func NewFirestore(ctx context.Context, project, database string) (*Firestore, error) {
	if database == "" {
		database = firestore.DefaultDatabaseID
	}
	c, err := firestore.NewClientWithDatabase(ctx, project, database)
	if err != nil {
		return nil, fmt.Errorf("store: firestore client (project %q, database %q): %w", project, database, err)
	}
	return NewFirestoreFromClient(c), nil
}

// NewFirestoreFromClient wraps an existing client (tests use the emulator).
func NewFirestoreFromClient(c *firestore.Client) *Firestore {
	return &Firestore{client: c, col: c.Collection(FirestoreCollection)}
}

// Close releases the client.
func (f *Firestore) Close() error { return f.client.Close() }

// Put implements Cases.
func (f *Firestore) Put(ctx context.Context, c Case) error {
	if !caseIDPattern.MatchString(c.ID) {
		return fmt.Errorf("store: invalid case id %q", c.ID)
	}
	b, err := gzipJSON(c)
	if err != nil {
		return fmt.Errorf("store: encode %s: %w", c.ID, err)
	}
	if _, err := f.col.Doc(c.ID).Set(ctx, firestoreDoc{ID: c.ID, UpdatedAt: c.UpdatedAt, Case: b}); err != nil {
		return fmt.Errorf("store: write %s: %w", c.ID, err)
	}
	return nil
}

// Update implements Cases. fn runs inside a transaction, so two writers
// each apply their own change to the latest case instead of one Set
// replacing the other.
func (f *Firestore) Update(ctx context.Context, id string, fn func(*Case) error) (Case, error) {
	if !caseIDPattern.MatchString(id) {
		return Case{}, ErrNotFound
	}
	var out Case
	err := f.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(f.col.Doc(id))
		if status.Code(err) == codes.NotFound {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		c, err := decodeSnapshot(snap)
		if err != nil {
			return err
		}
		if err := fn(&c); err != nil {
			return err
		}
		b, err := gzipJSON(c)
		if err != nil {
			return fmt.Errorf("store: encode %s: %w", id, err)
		}
		out = c
		return tx.Set(f.col.Doc(id), firestoreDoc{ID: c.ID, UpdatedAt: c.UpdatedAt, Case: b})
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Case{}, ErrNotFound
		}
		return Case{}, fmt.Errorf("store: update %s: %w", id, err)
	}
	return out, nil
}

// Get implements Cases.
func (f *Firestore) Get(ctx context.Context, id string) (Case, error) {
	if !caseIDPattern.MatchString(id) {
		return Case{}, ErrNotFound
	}
	snap, err := f.col.Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return Case{}, ErrNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("store: read %s: %w", id, err)
	}
	return decodeSnapshot(snap)
}

// List implements Cases.
func (f *Firestore) List(ctx context.Context) ([]Case, error) {
	it := f.col.Documents(ctx)
	defer it.Stop()
	var out []Case
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("store: list: %w", err)
		}
		c, err := decodeSnapshot(snap)
		if err != nil {
			continue // skip a corrupt document rather than failing the list
		}
		out = append(out, c)
	}
	sortNewest(out)
	return out, nil
}

// Delete implements Cases.
func (f *Firestore) Delete(ctx context.Context, id string) error {
	if !caseIDPattern.MatchString(id) {
		return ErrNotFound
	}
	if _, err := f.col.Doc(id).Get(ctx); status.Code(err) == codes.NotFound {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("store: read %s: %w", id, err)
	}
	if _, err := f.col.Doc(id).Delete(ctx); err != nil {
		return fmt.Errorf("store: delete %s: %w", id, err)
	}
	return nil
}

func decodeSnapshot(snap *firestore.DocumentSnapshot) (Case, error) {
	var d firestoreDoc
	if err := snap.DataTo(&d); err != nil {
		return Case{}, fmt.Errorf("store: decode %s: %w", snap.Ref.ID, err)
	}
	var c Case
	if err := gunzipJSON(d.Case, &c); err != nil {
		return Case{}, fmt.Errorf("store: decode %s: %w", snap.Ref.ID, err)
	}
	return c, nil
}

func gzipJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(v); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipJSON(b []byte, v any) error {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, v)
}

var _ Cases = (*Firestore)(nil)

// VerdictCollection holds cached verifier verdicts, one document per key.
const VerdictCollection = "verifier_cache"

// FirestoreBlobs stores cached verifier verdicts in Firestore (verifier.Blobs),
// so every Cloud Run instance reuses the same verdict and a restart keeps it
// (D50). Without it, /tmp caches differ per instance and a replayed plan
// search could land on an instance that verifies afresh.
type FirestoreBlobs struct{ col *firestore.CollectionRef }

type verdictDoc struct {
	Verdict []byte    `firestore:"verdict_json"`
	At      time.Time `firestore:"cached_at"`
}

// EngineCollection holds cached engine predictions (D54).
const EngineCollection = "engine_cache"

// Verdicts returns the verdict cache on the same client.
func (f *Firestore) Verdicts() *FirestoreBlobs { return f.Blobs(VerdictCollection) }

// Blobs returns a blob store in collection on the same client.
func (f *Firestore) Blobs(collection string) *FirestoreBlobs {
	return &FirestoreBlobs{col: f.client.Collection(collection)}
}

// Get implements verifier.Blobs.
func (b *FirestoreBlobs) Get(ctx context.Context, key string) ([]byte, bool, error) {
	snap, err := b.col.Doc(key).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("store: read verdict %s: %w", key, err)
	}
	var d verdictDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, false, fmt.Errorf("store: decode verdict %s: %w", key, err)
	}
	return d.Verdict, true, nil
}

// Put implements verifier.Blobs. The first verdict stored for a key wins:
// two instances verifying the same move at once keep one answer.
func (b *FirestoreBlobs) Put(ctx context.Context, key string, v []byte) error {
	_, err := b.col.Doc(key).Create(ctx, verdictDoc{Verdict: v, At: time.Now().UTC()})
	if status.Code(err) == codes.AlreadyExists {
		return nil
	}
	return err
}
