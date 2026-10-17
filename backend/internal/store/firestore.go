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
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
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
	Pinned    bool      `firestore:"pinned"` // queried by ListRecent (D58)
	// Workspace is queried by ListWorkspace (D68); absent for anonymous cases.
	Workspace string `firestore:"workspace,omitempty"`
	Case      []byte `firestore:"case_json_gz"`
}

func docOf(c Case, gz []byte) firestoreDoc {
	return firestoreDoc{ID: c.ID, UpdatedAt: c.UpdatedAt, Pinned: c.Pinned, Workspace: c.Workspace, Case: gz}
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
	if _, err := f.col.Doc(c.ID).Set(ctx, docOf(c, b)); err != nil {
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
		return tx.Set(f.col.Doc(id), docOf(c, b))
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

// ListWorkspace implements WorkspaceLister with a single-field equality
// query (no composite index needed); it reads all of the workspace's cases.
func (f *Firestore) ListWorkspace(ctx context.Context, workspace string) ([]Case, error) {
	it := f.col.Where("workspace", "==", workspace).Documents(ctx)
	defer it.Stop()
	var out []Case
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("store: list workspace: %w", err)
		}
		c, err := decodeSnapshot(snap)
		if err != nil {
			continue
		}
		out = append(out, c)
	}
	sortNewest(out)
	return out, nil
}

// ListRecent implements RecentLister: every pinned case plus the n most
// recently updated others, reading only those documents (D58). A pinned case
// saved before the pinned field existed is found by the second query only
// while it is among the newest; re-pinning it (PUT /pin) writes the field.
func (f *Firestore) ListRecent(ctx context.Context, n int) ([]Case, error) {
	seen := map[string]bool{}
	var out []Case
	add := func(q firestore.Query) error {
		it := q.Documents(ctx)
		defer it.Stop()
		for {
			snap, err := it.Next()
			if errors.Is(err, iterator.Done) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("store: list: %w", err)
			}
			if seen[snap.Ref.ID] {
				continue
			}
			c, err := decodeSnapshot(snap)
			if err != nil {
				continue
			}
			seen[snap.Ref.ID] = true
			out = append(out, c)
		}
	}
	if err := add(f.col.Where("pinned", "==", true)); err != nil {
		return nil, err
	}
	if err := add(f.col.OrderBy("updated_at", firestore.Desc).Limit(n + len(out))); err != nil {
		return nil, err
	}
	return CapRecent(out, n), nil
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

// SpendCollection holds one document per UTC day with the model spend so
// far (spend.Store, D59).
const SpendCollection = "model_spend"

// Spend is the daily spend counter on the same client.
func (f *Firestore) Spend() *FirestoreSpend {
	return &FirestoreSpend{col: f.client.Collection(SpendCollection)}
}

// FirestoreSpend implements spend.Store with an atomic increment per day.
type FirestoreSpend struct{ col *firestore.CollectionRef }

// Add implements spend.Store.
func (s *FirestoreSpend) Add(ctx context.Context, day string, usd float64) (float64, error) {
	ref := s.col.Doc(day)
	if _, err := ref.Set(ctx, map[string]any{"usd": firestore.Increment(usd), "updated_at": firestore.ServerTimestamp}, firestore.MergeAll); err != nil {
		return 0, fmt.Errorf("store: add spend: %w", err)
	}
	return s.Total(ctx, day)
}

// Total implements spend.Store.
func (s *FirestoreSpend) Total(ctx context.Context, day string) (float64, error) {
	snap, err := s.col.Doc(day).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: read spend: %w", err)
	}
	v, err := snap.DataAt("usd")
	if err != nil {
		return 0, nil
	}
	switch n := v.(type) {
	case float64:
		return n, nil
	case int64:
		return float64(n), nil
	}
	return 0, nil
}

// APIKeyCollection holds one document per API key, by key ID, with the
// key's SHA-256 (never the key itself); APIKeyUsageCollection holds one
// document per key and UTC day with its request count (apikey.Store, D69).
const (
	APIKeyCollection      = "api_keys"
	APIKeyUsageCollection = "api_key_usage"
)

// APIKeys is the API key store on the same client.
func (f *Firestore) APIKeys() *FirestoreAPIKeys {
	return &FirestoreAPIKeys{client: f.client, keys: f.client.Collection(APIKeyCollection), usage: f.client.Collection(APIKeyUsageCollection)}
}

// FirestoreAPIKeys implements apikey.Store.
type FirestoreAPIKeys struct {
	client      *firestore.Client
	keys, usage *firestore.CollectionRef
}

type apiKeyDoc struct {
	ID         string    `firestore:"id"`
	Name       string    `firestore:"name"`
	Hash       string    `firestore:"sha256"`
	CreatedAt  time.Time `firestore:"created_at"`
	DailyQuota int       `firestore:"daily_quota"`
	Disabled   bool      `firestore:"disabled"`
}

func (d apiKeyDoc) key() apikey.Key {
	return apikey.Key{ID: d.ID, Name: d.Name, Hash: d.Hash, CreatedAt: d.CreatedAt, DailyQuota: d.DailyQuota, Disabled: d.Disabled}
}

// Create implements apikey.Store.
func (s *FirestoreAPIKeys) Create(ctx context.Context, k apikey.Key) error {
	d := apiKeyDoc{ID: k.ID, Name: k.Name, Hash: k.Hash, CreatedAt: k.CreatedAt, DailyQuota: k.DailyQuota, Disabled: k.Disabled}
	if _, err := s.keys.Doc(k.ID).Create(ctx, d); err != nil {
		return fmt.Errorf("store: create api key: %w", err)
	}
	return nil
}

// Get implements apikey.Store.
func (s *FirestoreAPIKeys) Get(ctx context.Context, id string) (apikey.Key, error) {
	snap, err := s.keys.Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return apikey.Key{}, apikey.ErrNotFound
	}
	if err != nil {
		return apikey.Key{}, fmt.Errorf("store: read api key: %w", err)
	}
	var d apiKeyDoc
	if err := snap.DataTo(&d); err != nil {
		return apikey.Key{}, fmt.Errorf("store: decode api key: %w", err)
	}
	return d.key(), nil
}

// List implements apikey.Store.
func (s *FirestoreAPIKeys) List(ctx context.Context) ([]apikey.Key, error) {
	it := s.keys.Documents(ctx)
	defer it.Stop()
	var out []apikey.Key
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("store: list api keys: %w", err)
		}
		var d apiKeyDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, fmt.Errorf("store: decode api key: %w", err)
		}
		out = append(out, d.key())
	}
}

// Disable implements apikey.Store.
func (s *FirestoreAPIKeys) Disable(ctx context.Context, id string) error {
	_, err := s.keys.Doc(id).Update(ctx, []firestore.Update{{Path: "disabled", Value: true}})
	if status.Code(err) == codes.NotFound {
		return apikey.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("store: disable api key: %w", err)
	}
	return nil
}

// Use implements apikey.Store. A transaction reads and bumps the day's
// count, so two instances counting at once each get their own number.
func (s *FirestoreAPIKeys) Use(ctx context.Context, id, day string) (int64, error) {
	ref := s.usage.Doc(id + "_" + day)
	var n int64
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		n = 0
		snap, err := tx.Get(ref)
		switch {
		case status.Code(err) == codes.NotFound:
		case err != nil:
			return err
		default:
			if v, err := snap.DataAt("requests"); err == nil {
				n, _ = v.(int64)
			}
		}
		n++
		return tx.Set(ref, map[string]any{"key_id": id, "day": day, "requests": n, "updated_at": time.Now().UTC()})
	})
	if err != nil {
		return 0, fmt.Errorf("store: count api key use: %w", err)
	}
	return n, nil
}
