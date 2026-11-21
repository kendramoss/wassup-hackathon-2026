package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// slowCases is a case store whose Get takes a store round trip and counts
// the lookups of unknown review-token cases.
type slowCases struct {
	*store.Memory
	probes atomic.Int64
}

func (s *slowCases) Get(ctx context.Context, id string) (store.Case, error) {
	if strings.HasPrefix(id, "case_") {
		s.probes.Add(1)
		time.Sleep(50 * time.Millisecond)
	}
	return s.Memory.Get(ctx, id)
}

// wellFormedReviewToken has the shape of a review link token but names a
// case that does not exist, so it reaches the case store.
func wellFormedReviewToken(t *testing.T) string {
	t.Helper()
	id, secret := make([]byte, 6), make([]byte, 32)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	return "ubr_" + hex.EncodeToString(id) + "_" + base64.RawURLEncoding.EncodeToString(secret)
}

// Follow-up to #489: concurrent bad review tokens from one client reach the
// case store at most AuthFailuresPerMin times; the rest are refused with 429
// before any lookup.
func TestBrokerReviewBadTokensThrottledUnderConcurrency(t *testing.T) {
	const perMin, n = 3, 30
	cs := &slowCases{Memory: store.NewMemory()}
	hs, _ := reviewHarness(t, func(d *Deps) {
		d.Cases = cs
		d.Limits.AuthFailuresPerMin = perMin
	})
	toks := make([]string, n)
	for i := range toks {
		toks[i] = wellFormedReviewToken(t)
	}
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			codes[i] = serve(hs, "GET", "/v1/review", "", with(map[string]string{"X-Client-IP": "203.0.113.60"}, tokenHeader(toks[i]))).Code
		})
	}
	wg.Wait()
	count := map[int]int{}
	for _, c := range codes {
		count[c]++
	}
	if got := cs.probes.Load(); got > perMin {
		t.Errorf("%d case lookups for bad tokens from one client, want at most %d", got, perMin)
	}
	if count[http.StatusNotFound] > perMin || count[http.StatusNotFound]+count[http.StatusTooManyRequests] != n {
		t.Errorf("codes %v, want at most %d x 404 and the rest 429", count, perMin)
	}
	// Another client is not affected.
	if c := serve(hs, "GET", "/v1/review", "", with(map[string]string{"X-Client-IP": "203.0.113.61"}, tokenHeader(wellFormedReviewToken(t)))).Code; c != http.StatusNotFound {
		t.Errorf("bad token from another client = %d, want 404", c)
	}
}
