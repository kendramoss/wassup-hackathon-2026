package main

import (
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestInfraIsOffByDefault(t *testing.T) {
	s, err := newInfra(context.Background(), func(string) string { return "" }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil || s.placement.Active() || s.blobKey != nil || s.statusPage {
		t.Fatalf("defaults changed behaviour: %+v %v", s, err)
	}
}

func TestInfraSettings(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	s, err := newInfra(context.Background(), env(map[string]string{
		"TENANT_TIER": "dedicated", "TENANT_WORKSPACE": "acme", "DATA_REGION": "eu", "STATUS_PAGE": "on",
		"BLOB_ENCRYPTION": "local", "BLOB_ENCRYPTION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
	}), quiet)
	if err != nil || !s.placement.Active() || s.blobKey == nil || !s.statusPage {
		t.Fatalf("got %+v %v", s, err)
	}
	_, err = newInfra(context.Background(), env(map[string]string{"STATUS_PAGE": "yes", "TENANT_TIER": "x"}), quiet)
	if err == nil || !strings.Contains(err.Error(), "STATUS_PAGE") || !strings.Contains(err.Error(), "TENANT_TIER") {
		t.Fatalf("want both errors, got %v", err)
	}
}
