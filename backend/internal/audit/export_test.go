package audit

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

const exTenant = "ws:ws_0123456789ab"

func exportAll(t *testing.T, s *Service, from, to *time.Time) []byte {
	t.Helper()
	ctx := context.Background()
	h, err := s.ExportSnapshot(ctx, exTenant)
	if err != nil {
		t.Fatal(err)
	}
	h.From, h.To = from, to
	var buf bytes.Buffer
	x := NewExportWriter(&buf)
	if err := x.Header(h); err != nil {
		t.Fatal(err)
	}
	_, complete, err := s.ExportEvents(ctx, h, 0, x.Event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x.Summary(complete); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func seeded(t *testing.T, n int) (*Service, *clock) {
	t.Helper()
	c := &clock{t: t0}
	s := newTestService(t, NewMemory(), c.now)
	for i := 0; i < n; i++ {
		mustRecord(t, s, userDraft(exTenant, CaseCreate))
		c.advance(time.Minute)
	}
	return s, c
}

func TestExportRoundTripVerifies(t *testing.T) {
	s, _ := seeded(t, 7)
	b := exportAll(t, s, nil, nil)
	if got := strings.Count(string(b), "\n"); got != 9 {
		t.Fatalf("lines = %d, want header + 7 events + summary", got)
	}
	res, err := VerifyExport(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || !res.Whole || res.Events != 7 {
		t.Fatalf("got %+v", res)
	}
}

func TestExportDetectsEditsAndDrops(t *testing.T) {
	s, _ := seeded(t, 5)
	b := string(exportAll(t, s, nil, nil))
	lines := strings.Split(strings.TrimSuffix(b, "\n"), "\n")

	edited := strings.Replace(b, `"action":"case.create"`, `"action":"case.delete"`, 1)
	if r, _ := VerifyExport(strings.NewReader(edited)); r.OK || r.DigestOK {
		t.Fatalf("an edited event verified: %+v", r)
	}
	// Drop an event and recompute nothing: digest and chain both fail.
	dropped := strings.Join(append(append([]string{}, lines[:2]...), lines[3:]...), "\n") + "\n"
	if r, _ := VerifyExport(strings.NewReader(dropped)); r.OK || len(r.Findings) == 0 {
		t.Fatalf("a dropped event verified: %+v", r)
	}
	// Truncating the tail (keep the summary) breaks the head check.
	trunc := strings.Join(append(append([]string{}, lines[:len(lines)-2]...), lines[len(lines)-1]), "\n") + "\n"
	if r, _ := VerifyExport(strings.NewReader(trunc)); r.OK {
		t.Fatalf("a truncated export verified: %+v", r)
	}
}

func TestExportIsASnapshot(t *testing.T) {
	s, c := seeded(t, 3)
	ctx := context.Background()
	h, err := s.ExportSnapshot(ctx, exTenant)
	if err != nil {
		t.Fatal(err)
	}
	c.advance(time.Minute)
	mustRecord(t, s, userDraft(exTenant, CaseCreate)) // appended after the snapshot
	n, complete, err := s.ExportEvents(ctx, h, 0, func(Event) error { return nil })
	if err != nil || n != 3 || !complete {
		t.Fatalf("n=%d complete=%v err=%v", n, complete, err)
	}
}

func TestExportWindowAndCap(t *testing.T) {
	s, _ := seeded(t, 6)
	from, to := t0.Add(2*time.Minute), t0.Add(4*time.Minute)
	b := exportAll(t, s, &from, &to)
	r, err := VerifyExport(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK || r.Whole || r.Events != 2 {
		t.Fatalf("window: %+v", r)
	}
	ctx := context.Background()
	h, _ := s.ExportSnapshot(ctx, exTenant)
	n, complete, err := s.ExportEvents(ctx, h, 4, func(Event) error { return nil })
	if err != nil || n != 4 || complete {
		t.Fatalf("cap: n=%d complete=%v err=%v", n, complete, err)
	}
}

func TestExportAfterPurgeAnchorsOnCheckpoint(t *testing.T) {
	c := &clock{t: t0}
	s := NewService(NewMemory(), pseud(t), Options{Now: c.now, Retention: 24 * time.Hour})
	for i := 0; i < 5; i++ {
		mustRecord(t, s, userDraft(exTenant, CaseCreate))
		c.advance(12 * time.Hour)
	}
	if _, err := s.PurgeAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	r, err := VerifyExport(bytes.NewReader(exportAll(t, s, nil, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK || r.Events >= 5 {
		t.Fatalf("after purge: %+v", r)
	}
}

func TestVerifyExportRejectsMalformed(t *testing.T) {
	for _, in := range []string{"", `{"tenant":"x"}` + "\n", `{"type":"header"}` + "\n", `{"type":"nope"}` + "\n"} {
		if _, err := VerifyExport(strings.NewReader(in)); err == nil {
			t.Fatalf("accepted %q", in)
		}
	}
}
