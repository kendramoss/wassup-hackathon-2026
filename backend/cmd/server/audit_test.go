package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

func auditEnv(extra map[string]string) func(string) string {
	m := map[string]string{"GOOGLE_CLOUD_PROJECT": "p", "CASES_STORE": "memory"}
	for k, v := range extra {
		m[k] = v
	}
	return func(k string) string { return m[k] }
}

func TestNewAuditIsOffWithoutTheSecretAndSaysSo(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	cfg, err := config.FromEnv(auditEnv(map[string]string{"K_SERVICE": "svc", "CASES_STORE": "memory"}))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := newAudit(context.Background(), cfg, store.NewMemory(), logger)
	if err != nil || svc != nil {
		t.Fatalf("no key: svc=%v err=%v, want nil and no error (existing deployments are unchanged)", svc, err)
	}
	if out := buf.String(); !strings.Contains(out, "audit trail NOT enabled") || strings.Contains(out, "no key") {
		t.Errorf("the log must say plainly that the trail is not enabled: %s", out)
	}
}

func TestNewAuditBuildsEachStoreWithTheConfiguredRetention(t *testing.T) {
	key := strings.Repeat("k", 40)
	for _, c := range []map[string]string{
		{"AUDIT_ACTOR_KEY": key, "CASES_STORE": "memory"},
		{"AUDIT_ACTOR_KEY": key, "CASES_STORE": "file", "AUDIT_DIR": t.TempDir()},
	} {
		cfg, err := config.FromEnv(auditEnv(c))
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		var buf bytes.Buffer
		svc, err := newAudit(ctx, cfg, store.NewMemory(), slog.New(slog.NewTextHandler(&buf, nil)))
		cancel()
		if err != nil || svc == nil || svc.Retention().Hours() != 90*24 {
			t.Fatalf("%v: svc=%v err=%v", c, svc, err)
		}
		if strings.Contains(buf.String(), key) {
			t.Error("the log contains the actor key")
		}
	}
}
