package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// barrierStore forces the interleaving Put(A), Put(B), Get(A), Delete(A),
// Get(B) over a real store. With one shared probe ID, Get(B) would then miss.
type barrierStore struct {
	store.Cases
	putArrived sync.WaitGroup
	putGate    chan struct{}
	gets       atomic.Int32
	deleted    chan struct{}
	delOnce    sync.Once
	mu         sync.Mutex
	putIDs     []string
}

func newBarrierStore(inner store.Cases) *barrierStore {
	b := &barrierStore{Cases: inner, putGate: make(chan struct{}), deleted: make(chan struct{})}
	b.putArrived.Add(2)
	go func() { b.putArrived.Wait(); close(b.putGate) }()
	return b
}

func (b *barrierStore) Put(ctx context.Context, c store.Case) error {
	if err := b.Cases.Put(ctx, c); err != nil {
		return err
	}
	b.mu.Lock()
	b.putIDs = append(b.putIDs, c.ID)
	b.mu.Unlock()
	b.putArrived.Done()
	select {
	case <-b.putGate:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("barrier: second probe never reached Put")
	}
}

func (b *barrierStore) Get(ctx context.Context, id string) (store.Case, error) {
	if b.gets.Add(1) == 2 { // the second reader waits for the first probe's Delete
		select {
		case <-b.deleted:
		case <-time.After(5 * time.Second):
			return store.Case{}, errors.New("barrier: first probe never deleted")
		}
	}
	return b.Cases.Get(ctx, id)
}

func (b *barrierStore) Delete(ctx context.Context, id string) error {
	err := b.Cases.Delete(ctx, id)
	b.delOnce.Do(func() { close(b.deleted) })
	return err
}

func runTwoProbes(t *testing.T, cs store.Cases) {
	t.Helper()
	b := newBarrierStore(cs)
	check := StoreCheck(b)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { errs <- check.Run(context.Background()) }()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent probe %d: %v", i, err)
		}
	}
	if len(b.putIDs) != 2 || b.putIDs[0] == b.putIDs[1] {
		t.Errorf("probes did not use their own IDs: %v", b.putIDs)
	}
	if left, err := cs.List(context.Background()); err != nil || len(left) != 0 {
		t.Errorf("probes left cases behind: %v %v", left, err)
	}
}

func TestStoreCheckConcurrentProbesDoNotCollide(t *testing.T) {
	t.Run("memory", func(t *testing.T) { runTwoProbes(t, store.NewMemory()) })
	t.Run("file", func(t *testing.T) {
		fs, err := store.NewFile(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		runTwoProbes(t, fs)
	})
}

func TestDeepHealthConcurrentRequestsBothHealthy(t *testing.T) {
	inner := store.NewMemory()
	b := newBarrierStore(inner)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Checks, d.Cases = []HealthCheck{StoreCheck(b)}, inner
	})
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		go func() {
			code, _ := hs.do(t, "GET", "/health?deep=1", "")
			codes <- code
		}()
	}
	for i := 0; i < 2; i++ {
		if code := <-codes; code != http.StatusOK {
			t.Errorf("deep health = %d, want 200", code)
		}
	}
}

func TestHealthProbeIDsAreUniqueAndStoreValid(t *testing.T) {
	valid := regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`) // store caseIDPattern
	seen := map[string]bool{}
	for i := 0; i < 2000; i++ {
		id := newHealthProbeID()
		if !valid.MatchString(id) || !IsHealthProbeID(id) || id == HealthProbeID {
			t.Fatalf("bad probe ID %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate probe ID %q", id)
		}
		seen[id] = true
	}
	fs, err := store.NewFile(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := StoreCheck(fs).Run(context.Background()); err != nil {
		t.Fatalf("real file store rejects the probe ID: %v", err)
	}
}

func TestIsHealthProbeID(t *testing.T) {
	hex32 := strings.Repeat("ab12", 8)
	for id, want := range map[string]bool{
		"healthcheck_probe":                true,
		"healthcheck_probe_" + hex32:       true,
		"":                                 false,
		"healthcheck_probe_":               false,
		"healthcheck_probe_" + hex32 + "0": false,
		"healthcheck_probe_" + strings.ToUpper(hex32): false,
		"healthcheck_probe_zz" + hex32[2:]:            false,
		"healthcheck_probe2":                          false,
		"my_healthcheck_probe_" + hex32:               false,
		"c_12345":                                     false,
	} {
		if got := IsHealthProbeID(id); got != want {
			t.Errorf("IsHealthProbeID(%q) = %v, want %v", id, got, want)
		}
	}
}

// flakyStore wraps a real store and lets a test make one operation fail
// after (or instead of) really doing it.
type flakyStore struct {
	store.Cases
	putErr, getErr, delErr error
	onGet                  func()
}

func (f *flakyStore) Put(ctx context.Context, c store.Case) error {
	if err := f.Cases.Put(ctx, c); err != nil {
		return err
	}
	return f.putErr // the write landed but the caller saw an error (e.g. timeout)
}

func (f *flakyStore) Get(ctx context.Context, id string) (store.Case, error) {
	if f.onGet != nil {
		f.onGet()
	}
	if f.getErr != nil {
		return store.Case{}, f.getErr
	}
	return f.Cases.Get(ctx, id)
}

func (f *flakyStore) Delete(ctx context.Context, id string) error {
	if f.delErr != nil {
		return f.delErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.Cases.Delete(ctx, id)
}

func TestStoreCheckCleansUpAndReportsRealErrors(t *testing.T) {
	denied := errors.New("PermissionDenied")
	t.Run("get fails: error reported, probe removed", func(t *testing.T) {
		mem := store.NewMemory()
		err := StoreCheck(&flakyStore{Cases: mem, getErr: denied}).Run(context.Background())
		if !errors.Is(err, denied) {
			t.Errorf("err = %v, want the Get error", err)
		}
		assertEmpty(t, mem)
	})
	t.Run("put errors after writing: error reported, probe removed", func(t *testing.T) {
		mem := store.NewMemory()
		err := StoreCheck(&flakyStore{Cases: mem, putErr: denied}).Run(context.Background())
		if !errors.Is(err, denied) {
			t.Errorf("err = %v, want the Put error", err)
		}
		assertEmpty(t, mem)
	})
	t.Run("delete fails: that error is reported", func(t *testing.T) {
		err := StoreCheck(&flakyStore{Cases: store.NewMemory(), delErr: denied}).Run(context.Background())
		if !errors.Is(err, denied) {
			t.Errorf("err = %v, want the Delete error", err)
		}
	})
	t.Run("cancelled mid-probe: still reported, probe removed", func(t *testing.T) {
		mem := store.NewMemory()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		fs := &flakyStore{Cases: mem, onGet: cancel}
		err := StoreCheck(fs).Run(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
		assertEmpty(t, mem)
	})
	t.Run("healthy: removed", func(t *testing.T) {
		mem := store.NewMemory()
		if err := StoreCheck(mem).Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertEmpty(t, mem)
	})
}

func assertEmpty(t *testing.T, cs store.Cases) {
	t.Helper()
	if left, err := cs.List(context.Background()); err != nil || len(left) != 0 {
		t.Errorf("store not clean: %v %v", left, err)
	}
}

func TestDeepHealthFailingStoreStill503(t *testing.T) {
	denied := errors.New("rpc error: PermissionDenied")
	mem := store.NewMemory()
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		d.Checks, d.Cases = []HealthCheck{StoreCheck(&flakyStore{Cases: mem, getErr: denied})}, mem
	})
	code, out := hs.do(t, "GET", "/health?deep=1", "")
	if code != http.StatusServiceUnavailable || out["status"] != "degraded" {
		t.Errorf("= %d %v", code, out)
	}
	assertEmpty(t, mem)
}

func TestCaseListHidesProbeCases(t *testing.T) {
	mem := store.NewMemory()
	now := time.Now().UTC()
	put := func(id string, pinned bool, owner string) {
		t.Helper()
		if err := mem.Put(context.Background(), store.Case{ID: id, Pinned: pinned, Owner: owner, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	newID := newHealthProbeID()
	put("real_case", false, "")
	for _, id := range []string{HealthProbeID, newID} {
		put(id, false, "")
	}
	put(newHealthProbeID(), true, "") // pinned probes are hidden too
	put(newHealthProbeID(), false, "someone")
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) { d.Cases = mem })
	for name, h := range map[string]map[string]string{"visitor": nil, "admin": {"X-Admin-Token": testAdmin}} {
		code, out := hs.doAs(t, "GET", "/v1/cases", "", h)
		rows, _ := out["cases"].([]any)
		if code != http.StatusOK || len(rows) != 1 || rows[0].(map[string]any)["id"] != "real_case" {
			t.Errorf("%s list = %d %v", name, code, out)
		}
	}
}
