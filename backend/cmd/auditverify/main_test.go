package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
)

func exportOf(t *testing.T) string {
	t.Helper()
	ps, err := audit.NewPseudonymizer("1", []byte(strings.Repeat("k", 40)))
	if err != nil {
		t.Fatal(err)
	}
	svc := audit.NewService(audit.NewMemory(), ps, audit.Options{Now: func() time.Time { return time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC) }})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := svc.Record(ctx, audit.Draft{Event: audit.Event{Tenant: "ws:ws_0123456789ab", ActorKind: audit.ActorSystem,
			Action: audit.CaseDelete, Phase: audit.PhaseSingle, Outcome: audit.Allowed, Detail: "retention", Object: "case:case_0123456789ab"}}); err != nil {
			t.Fatal(err)
		}
	}
	h, _ := svc.ExportSnapshot(ctx, "ws:ws_0123456789ab")
	var b bytes.Buffer
	x := audit.NewExportWriter(&b)
	_ = x.Header(h)
	_, complete, _ := svc.ExportEvents(ctx, h, 0, x.Event)
	_, _ = x.Summary(complete)
	return b.String()
}

func TestExitCodes(t *testing.T) {
	good := exportOf(t)
	if code := run([]string{"-"}, strings.NewReader(good), &bytes.Buffer{}, &bytes.Buffer{}); code != 0 {
		t.Fatalf("intact export = %d", code)
	}
	bad := strings.Replace(good, `"detail":"retention"`, `"detail":""`, 1)
	if code := run([]string{"-"}, strings.NewReader(bad), &bytes.Buffer{}, &bytes.Buffer{}); code != 1 {
		t.Fatalf("edited export = %d", code)
	}
	if code := run([]string{"-"}, strings.NewReader("not json\n"), &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatalf("garbage = %d", code)
	}
	if code := run(nil, nil, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
		t.Fatalf("no args = %d", code)
	}
}
