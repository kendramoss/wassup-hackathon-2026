// Package envelope encrypts stored blobs with a data key that is itself
// wrapped by a key-encryption key the customer can hold (#874, CMEK).
//
// A Provider wraps and unwraps data keys: Local holds a key in memory (for
// development and tests only), KMS calls Cloud KMS. Blobs wraps a
// blob.Store: each Put is sealed with AES-256-GCM under the process's data
// key, bound to the blob's key, and records which wrapped data key sealed
// it. A Get of a blob written before encryption was on returns it as stored,
// so turning encryption on needs no migration; the caches it protects are
// disposable (docs/design/cmek.md).
//
// Off by default: with BLOB_ENCRYPTION unset nothing is wrapped.
package envelope

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
)

// Provider wraps and unwraps data keys with a key-encryption key.
type Provider interface {
	// Name identifies the key-encryption key (a KMS key name, or "local").
	Name() string
	Wrap(ctx context.Context, dek []byte) ([]byte, error)
	Unwrap(ctx context.Context, wrapped []byte) ([]byte, error)
}

// Local wraps data keys with an in-memory AES-256 key. Its key lives in the
// process's configuration, so it is not customer-managed: Config refuses it
// on Cloud Run.
type Local struct{ aead cipher.AEAD }

// NewLocal returns a Local provider for a 32-byte key.
func NewLocal(key []byte) (*Local, error) {
	a, err := gcm(key)
	if err != nil {
		return nil, err
	}
	return &Local{aead: a}, nil
}

func (*Local) Name() string { return "local" }

func (l *Local) Wrap(_ context.Context, dek []byte) ([]byte, error) { return seal(l.aead, dek, nil) }

func (l *Local) Unwrap(_ context.Context, w []byte) ([]byte, error) { return open(l.aead, w, nil) }

func gcm(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("envelope: key must be 32 bytes, got %d", len(key))
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

func seal(a cipher.AEAD, plain, aad []byte) ([]byte, error) {
	nonce := make([]byte, a.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, plain, aad), nil
}

func open(a cipher.AEAD, b, aad []byte) ([]byte, error) {
	if len(b) < a.NonceSize() {
		return nil, errors.New("envelope: ciphertext too short")
	}
	return a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], aad)
}

// sealed is a stored encrypted blob. Format "openv1": the marker field is
// what tells it apart from a plaintext blob written before encryption.
type sealed struct {
	Format  string `json:"openv"`
	KEK     string `json:"kek"`
	Wrapped string `json:"wrapped_dek"`
	Data    string `json:"data"`
}

const format = "openv1"

// Blobs is a blob.Store whose values are encrypted at rest.
type Blobs struct {
	inner blob.Store
	p     Provider

	mu      sync.Mutex
	dek     []byte
	wrapped []byte
	keys    map[string]cipher.AEAD // unwrapped data keys by wrapped form
}

// NewBlobs wraps inner.
func NewBlobs(inner blob.Store, p Provider) *Blobs {
	return &Blobs{inner: inner, p: p, keys: map[string]cipher.AEAD{}}
}

// maxKeys bounds the unwrapped-key cache: one data key per process start,
// so this holds the keys of the last few revisions.
const maxKeys = 64

func (b *Blobs) current(ctx context.Context) (cipher.AEAD, []byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dek == nil {
		dek := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, dek); err != nil {
			return nil, nil, err
		}
		w, err := b.p.Wrap(ctx, dek)
		if err != nil {
			return nil, nil, fmt.Errorf("envelope: wrap data key: %w", err)
		}
		a, err := gcm(dek)
		if err != nil {
			return nil, nil, err
		}
		b.dek, b.wrapped = dek, w
		b.remember(string(w), a)
	}
	return b.keys[string(b.wrapped)], b.wrapped, nil
}

func (b *Blobs) remember(w string, a cipher.AEAD) {
	if len(b.keys) >= maxKeys {
		for k := range b.keys {
			if k != string(b.wrapped) {
				delete(b.keys, k)
				break
			}
		}
	}
	b.keys[w] = a
}

func (b *Blobs) key(ctx context.Context, wrapped []byte) (cipher.AEAD, error) {
	b.mu.Lock()
	a, ok := b.keys[string(wrapped)]
	b.mu.Unlock()
	if ok {
		return a, nil
	}
	dek, err := b.p.Unwrap(ctx, wrapped)
	if err != nil {
		return nil, fmt.Errorf("envelope: unwrap data key: %w", err)
	}
	if a, err = gcm(dek); err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.remember(string(wrapped), a)
	b.mu.Unlock()
	return a, nil
}

// Put implements blob.Store.
func (b *Blobs) Put(ctx context.Context, key string, v []byte) error {
	a, w, err := b.current(ctx)
	if err != nil {
		return err
	}
	ct, err := seal(a, v, []byte(key))
	if err != nil {
		return err
	}
	out, err := json.Marshal(sealed{Format: format, KEK: b.p.Name(),
		Wrapped: base64.StdEncoding.EncodeToString(w), Data: base64.StdEncoding.EncodeToString(ct)})
	if err != nil {
		return err
	}
	return b.inner.Put(ctx, key, out)
}

// Get implements blob.Store. A blob stored before encryption was on is
// returned as stored; one that cannot be opened is a miss.
func (b *Blobs) Get(ctx context.Context, key string) ([]byte, bool, error) {
	raw, ok, err := b.inner.Get(ctx, key)
	if err != nil || !ok {
		return raw, ok, err
	}
	if !strings.Contains(string(raw), `"openv"`) {
		return raw, true, nil
	}
	var s sealed
	if json.Unmarshal(raw, &s) != nil || s.Format != format {
		return raw, true, nil
	}
	w, err1 := base64.StdEncoding.DecodeString(s.Wrapped)
	ct, err2 := base64.StdEncoding.DecodeString(s.Data)
	if err := errors.Join(err1, err2); err != nil {
		return nil, false, fmt.Errorf("envelope: decode %s: %w", key, err)
	}
	// A blob this process cannot open (key revoked or disabled, KMS
	// unreachable, blob copied under another key) is a cache miss, not an
	// error: the caller recomputes, and revoking the key never breaks a
	// prediction.
	a, err := b.key(ctx, w)
	if err != nil {
		return nil, false, nil
	}
	v, err := open(a, ct, []byte(key))
	if err != nil {
		return nil, false, nil
	}
	return v, true, nil
}

var _ blob.Store = (*Blobs)(nil)
