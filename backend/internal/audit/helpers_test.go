package audit

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

func pseud(t *testing.T) *Pseudonymizer {
	t.Helper()
	p, err := NewPseudonymizer("1", []byte(strings.Repeat("k", 40)))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func newTestService(t *testing.T, st Store, now func() time.Time) *Service {
	t.Helper()
	return NewService(st, pseud(t), Options{Now: now})
}

// clock is a settable clock.
type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

// userDraft is a minimal valid completion draft for tenant by user "u".
func userDraft(tenant string, a Action) Draft {
	return Draft{Event: Event{Tenant: tenant, ActorKind: ActorUser, Action: a, Phase: PhaseCompletion, Outcome: Allowed,
		Object: "case:case_0123456789ab", RequestID: "0123456789abcdef"}, ActorID: "alice@example.com"}
}

func mustRecord(t *testing.T, s *Service, d Draft) Event {
	t.Helper()
	e, err := s.Record(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func jsonMarshal(e Event) ([]byte, error) { return json.Marshal(e) }

func appendRaw(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}
