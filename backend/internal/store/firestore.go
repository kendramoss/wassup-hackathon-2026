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
