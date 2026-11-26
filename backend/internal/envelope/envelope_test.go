package envelope

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
)

func key(b byte) []byte { return bytes.Repeat([]byte{b}, 32) }

type mem map[string][]byte

func (m mem) Get(_ context.Context, k string) ([]byte, bool, error) { v, ok := m[k]; return v, ok, nil }
func (m mem) Put(_ context.Context, k string, v []byte) error       { m[k] = v; return nil }

func TestRoundTripAndAtRest(t *testing.T) {
	ctx := context.Background()
	p, _ := NewLocal(key(1))
	m := mem{}
	b := NewBlobs(m, p)
	secret := []byte(`{"origin":"VN","facts":"stamped steel housing"}`)
	if err := b.Put(ctx, "k1", secret); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(m["k1"], []byte("steel")) {
		t.Fatal("plaintext is visible at rest")
	}
	got, ok, err := b.Get(ctx, "k1")
	if err != nil || !ok || !bytes.Equal(got, secret) {
		t.Fatalf("got %q %v %v", got, ok, err)
	}
	// A fresh process (new data key) reads the old blob through its wrapped key.
	got, _, err = NewBlobs(m, p).Get(ctx, "k1")
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("second process: %q %v", got, err)
	}
}

func TestLegacyPlaintextIsReadAsStored(t *testing.T) {
	p, _ := NewLocal(key(1))
	m := mem{"old": []byte(`{"origin":"CN"}`)}
	got, ok, err := NewBlobs(m, p).Get(context.Background(), "old")
	if err != nil || !ok || string(got) != `{"origin":"CN"}` {
		t.Fatalf("got %q %v %v", got, ok, err)
	}
}

func TestWrongKeyOrSwappedBlobIsAMiss(t *testing.T) {
	ctx := context.Background()
	p1, _ := NewLocal(key(1))
	p2, _ := NewLocal(key(2))
	m := mem{}
	_ = NewBlobs(m, p1).Put(ctx, "a", []byte("x"))
	if v, ok, err := NewBlobs(m, p2).Get(ctx, "a"); ok || err != nil || v != nil {
		t.Fatalf("another key-encryption key: %q %v %v, want a miss", v, ok, err)
	}
	m["b"] = m["a"] // a blob moved to another key must not open
	if _, ok, err := NewBlobs(m, p1).Get(ctx, "b"); ok || err != nil {
		t.Fatalf("a blob copied under another key: %v %v, want a miss", ok, err)
	}
}

func TestWrapOffIsIdentity(t *testing.T) {
	m := blob.Dir{Dir: t.TempDir()}
	if Wrap(m, nil) != blob.Store(m) {
		t.Fatal("off must return the store unchanged")
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	k64 := base64.StdEncoding.EncodeToString(key(3))
	if c, err := ConfigFromEnv(env(nil)); err != nil || c.Mode != Off {
		t.Fatalf("default: %+v %v", c, err)
	}
	for _, m := range []map[string]string{
		{"BLOB_ENCRYPTION": "local"},
		{"BLOB_ENCRYPTION": "local", "BLOB_ENCRYPTION_KEY": k64, "K_SERVICE": "backend"},
		{"BLOB_ENCRYPTION": "kms", "BLOB_ENCRYPTION_KMS_KEY": "my-key"},
		{"BLOB_ENCRYPTION": "aes"},
	} {
		if _, err := ConfigFromEnv(env(m)); err == nil {
			t.Errorf("%v: want an error", m)
		}
	}
	if c, err := ConfigFromEnv(env(map[string]string{"BLOB_ENCRYPTION": "kms",
		"BLOB_ENCRYPTION_KMS_KEY": "projects/p/locations/us/keyRings/r/cryptoKeys/k"})); err != nil || c.Mode != KMSMode {
		t.Fatalf("kms: %+v %v", c, err)
	}
}

// fakeKMS mimics cryptoKeys.encrypt/decrypt by XOR, enough to test the wire.
func TestKMSProviderWire(t *testing.T) {
	const name = "projects/p/locations/us/keyRings/r/cryptoKeys/k"
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		field, out := "plaintext", "ciphertext"
		if strings.HasSuffix(r.URL.Path, ":decrypt") {
			field, out = "ciphertext", "plaintext"
		}
		b, _ := base64.StdEncoding.DecodeString(in[field])
		for i := range b {
			b[i] ^= 0x5a
		}
		_ = json.NewEncoder(w).Encode(map[string]string{out: base64.StdEncoding.EncodeToString(b)})
	}))
	defer srv.Close()
	k := &KMS{KeyName: name, Client: srv.Client(), BaseURL: srv.URL}
	ctx := context.Background()
	m := mem{}
	_ = NewBlobs(m, k).Put(ctx, "x", []byte("hello"))
	got, _, err := NewBlobs(m, k).Get(ctx, "x")
	if err != nil || string(got) != "hello" {
		t.Fatalf("got %q %v", got, err)
	}
	if len(paths) != 2 || paths[0] != "/"+name+":encrypt" || paths[1] != "/"+name+":decrypt" {
		t.Fatalf("calls %v", paths)
	}
}
