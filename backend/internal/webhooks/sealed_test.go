package webhooks

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/envelope"
)

func sealerWith(t *testing.T, b byte) *envelope.Sealer {
	t.Helper()
	s, err := envelope.NewSealer(bytes.Repeat([]byte{b}, 32), SealPurpose)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSealedStoreKeepsNoSecretInTheClear(t *testing.T) {
	ctx := context.Background()
	inner := NewMemory()
	st := NewSealedStore(inner, sealerWith(t, 1))
	e := Endpoint{ID: "wh_1", Workspace: "ws_a", URL: "https://x.example.com", Events: []string{EventBatchFinished}, Enabled: true,
		Secret: NewSecret(), PrevSecret: NewSecret(), PrevUntil: time.Unix(1_800_000_000, 0).UTC()}
	secret, prev := e.Secret, e.PrevSecret
	if err := st.PutEndpoint(ctx, e); err != nil {
		t.Fatal(err)
	}
	if e.Secret != secret || e.PrevSecret != prev {
		t.Fatal("PutEndpoint changed the caller's endpoint")
	}
	raw, _ := inner.GetEndpoint(ctx, "wh_1")
	for _, v := range []string{raw.Secret, raw.PrevSecret} {
		if v == secret || v == prev || strings.Contains(v, "whsec_") || !strings.HasPrefix(v, envelope.SealedPrefix) {
			t.Errorf("stored value %q is not sealed", v)
		}
	}
	if raw.Secret == raw.PrevSecret {
		t.Error("two secrets sealed to one value")
	}
	got, err := st.GetEndpoint(ctx, "wh_1")
	if err != nil || got.Secret != secret || got.PrevSecret != prev {
		t.Fatalf("get = %+v %v", got, err)
	}
	list, err := st.ListEndpoints(ctx, "ws_a")
	if err != nil || len(list) != 1 || list[0].Secret != secret || list[0].PrevSecret != prev {
		t.Fatalf("list = %+v %v", list, err)
	}
	e.PrevSecret = ""
	_ = st.PutEndpoint(ctx, e)
	if raw, _ = inner.GetEndpoint(ctx, "wh_1"); raw.PrevSecret != "" {
		t.Errorf("empty previous secret stored as %q", raw.PrevSecret)
	}
}

func TestSealedStoreFileHoldsNoPlaintext(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	f, err := NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	st := NewSealedStore(f, sealerWith(t, 1))
	e := Endpoint{ID: "wh_1", Workspace: "ws_a", Secret: NewSecret(), PrevSecret: NewSecret()}
	if err := st.PutEndpoint(ctx, e); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "webhooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(e.Secret)) || bytes.Contains(b, []byte(e.PrevSecret)) || bytes.Contains(b, []byte("whsec_")) {
		t.Error("the store file holds a secret in the clear")
	}
}

func TestSealedStoreFailsClosed(t *testing.T) {
	ctx := context.Background()
	inner := NewMemory()
	st := NewSealedStore(inner, sealerWith(t, 1))
	for _, id := range []string{"wh_1", "wh_2"} {
		if err := st.PutEndpoint(ctx, Endpoint{ID: id, Workspace: "ws_a", Secret: NewSecret(), PrevSecret: NewSecret()}); err != nil {
			t.Fatal(err)
		}
	}

	wrong := NewSealedStore(inner, sealerWith(t, 2))
	if g, err := wrong.GetEndpoint(ctx, "wh_1"); err == nil || g.Secret != "" || g.PrevSecret != "" || errors.Is(err, ErrNotFound) {
		t.Errorf("wrong key get = %+v %v", g, err)
	}
	if l, err := wrong.ListEndpoints(ctx, "ws_a"); err == nil || len(l) != 0 {
		t.Errorf("wrong key list = %+v %v", l, err)
	}

	// A sealed secret moved to another endpoint, or into the other field,
	// does not open.
	a, _ := inner.GetEndpoint(ctx, "wh_1")
	b, _ := inner.GetEndpoint(ctx, "wh_2")
	b.Secret = a.Secret
	_ = inner.PutEndpoint(ctx, b)
	if _, err := st.GetEndpoint(ctx, "wh_2"); err == nil {
		t.Error("a secret copied from another endpoint opens")
	}
	c, _ := inner.GetEndpoint(ctx, "wh_1")
	c.Secret, c.PrevSecret = c.PrevSecret, c.Secret
	_ = inner.PutEndpoint(ctx, c)
	if _, err := st.GetEndpoint(ctx, "wh_1"); err == nil {
		t.Error("swapped secret fields open")
	}
}

func TestSealedStoreRejectsRecordsWrittenBeforeSealing(t *testing.T) {
	ctx := context.Background()
	inner := NewMemory()
	plain := NewSecret()
	_ = inner.PutEndpoint(ctx, Endpoint{ID: "wh_old", Workspace: "ws_a", Secret: plain})
	st := NewSealedStore(inner, sealerWith(t, 1))
	g, err := st.GetEndpoint(ctx, "wh_old")
	if err == nil || g.Secret != "" || errors.Is(err, ErrNotFound) {
		t.Fatalf("plaintext record = %+v %v", g, err)
	}
	if strings.Contains(err.Error(), plain) || !strings.Contains(err.Error(), "wh_old") {
		t.Errorf("error = %q", err)
	}
	if _, err := st.ListEndpoints(ctx, "ws_a"); err == nil {
		t.Error("plaintext record listed")
	}
}

func TestSealedStoreRotationKeepsBothSecretsSealedAndSigningVerifies(t *testing.T) {
	c := &clock{t: time.Unix(1_800_000_000, 0).UTC()}
	inner := NewMemory()
	st := NewSealedStore(inner, sealerWith(t, 1))
	s := New(st, Options{AllowInsecure: true, Now: c.now, Client: http.DefaultClient})
	srv, got, mu := receiver(t)
	e := endpoint(t, st, c, "ws_a", srv.URL, EventBatchFinished)
	old := e.Secret
	ctx := context.Background()

	e2, err := s.Rotate(ctx, e.ID)
	if err != nil || e2.PrevSecret != old || e2.Secret == old {
		t.Fatalf("rotate = %+v %v", e2, err)
	}
	raw, _ := inner.GetEndpoint(ctx, e.ID)
	for _, v := range []string{raw.Secret, raw.PrevSecret} {
		if v == old || v == e2.Secret || !strings.HasPrefix(v, envelope.SealedPrefix) {
			t.Errorf("after rotation the store holds %q", v)
		}
	}
	s.Emit(ctx, "ws_a", EventBatchFinished, map[string]any{})
	s.Wait()
	mu.Lock()
	r := (*got)[0]
	mu.Unlock()
	for _, sec := range []string{old, e2.Secret} {
		if err := Verify(r.header.Get(SignatureHeader), r.body, sec, time.Minute, c.now()); err != nil {
			t.Errorf("signature does not verify with %s...: %v", sec[:8], err)
		}
	}
	if g, err := st.GetEndpoint(ctx, e.ID); err != nil || g.Secret != e2.Secret || g.PrevSecret != old {
		t.Errorf("reread = %+v %v", g, err)
	}
}

func TestSealedStores(t *testing.T) {
	storeContract(t, "sealed-memory", func(*testing.T) (Store, func() Store) {
		return NewSealedStore(NewMemory(), sealerWith(t, 1)), nil
	})
	storeContract(t, "sealed-file", func(t *testing.T) (Store, func() Store) {
		dir := t.TempDir()
		open := func() Store {
			f, err := NewFile(dir)
			if err != nil {
				t.Fatal(err)
			}
			return NewSealedStore(f, sealerWith(t, 1))
		}
		return open(), open
	})
}
