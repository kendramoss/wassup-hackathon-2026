package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIssueValidates(t *testing.T) {
	k := New(NewMemory())
	ctx := context.Background()
	tests := []struct {
		name  string
		key   string
		quota int
		ok    bool
	}{
		{"ok", "  Acme   Customs Brokers ", 100, true},
		{"empty name", "   ", 100, false},
		{"long name", strings.Repeat("a", MaxName+1), 100, false},
		{"zero quota", "Acme", 0, false},
		{"huge quota", "Acme", MaxQuota + 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret, key, err := k.Issue(ctx, tt.key, tt.quota)
			if !tt.ok {
				if !errors.Is(err, ErrBadRequest) {
					t.Fatalf("err = %v, want ErrBadRequest", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !keyPattern.MatchString(secret) || !strings.HasPrefix(secret, Prefix) {
				t.Errorf("secret %q has the wrong shape", secret)
			}
			if key.Name != "Acme Customs Brokers" || key.DailyQuota != tt.quota || !ValidID(key.ID) || key.Hash != hashOf(secret) {
				t.Errorf("key = %+v", key)
			}
			if strings.Contains(key.Hash, secret) || key.Hash == secret {
				t.Error("the secret is stored")
			}
		})
	}
}

func TestAuthenticate(t *testing.T) {
	k := New(NewMemory())
	ctx := context.Background()
	secret, key, err := k.Issue(ctx, "Acme", 10)
	if err != nil {
		t.Fatal(err)
	}
	other, _, _ := k.Issue(ctx, "Beta", 10)
	if got, err := k.Authenticate(ctx, secret); err != nil || got.ID != key.ID {
		t.Fatalf("Authenticate = %+v %v", got, err)
	}
	// Same ID prefix, different secret: the full hash must match too.
	tampered := secret[:len(secret)-1] + map[bool]string{true: "A", false: "B"}[secret[len(secret)-1] != 'A']
	for _, bad := range []string{"", "op_short", "sk_" + secret[3:], tampered, secret + "x"} {
		if _, err := k.Authenticate(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("Authenticate(%q) = %v, want ErrInvalid", bad, err)
		}
	}
	if err := k.Disable(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := k.Authenticate(ctx, secret); !errors.Is(err, ErrInvalid) {
		t.Errorf("disabled key = %v, want ErrInvalid", err)
	}
	if _, err := k.Authenticate(ctx, other); err != nil {
		t.Errorf("disabling one key affected another: %v", err)
	}
	for _, id := range []string{"0000000000000000", "../etc", ""} {
		if err := k.Disable(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Disable(%q) = %v, want ErrNotFound", id, err)
		}
	}
	ks, err := k.List(ctx)
	if err != nil || len(ks) != 2 {
		t.Fatalf("List = %d keys, %v", len(ks), err)
	}
}

func TestUseCountsPerUTCDay(t *testing.T) {
	k := New(NewMemory())
	now := time.Date(2026, 10, 17, 22, 30, 0, 0, time.UTC)
	k.now = func() time.Time { return now }
	ctx := context.Background()
	_, key, _ := k.Issue(ctx, "Acme", 2)
	for i := 1; i <= 2; i++ {
		if ok, n, _, err := k.Use(ctx, key); !ok || n != int64(i) || err != nil {
			t.Fatalf("use %d = %v %d %v", i, ok, n, err)
		}
	}
	ok, n, reset, _ := k.Use(ctx, key)
	if ok || n != 3 {
		t.Errorf("third use = %v %d, want refused", ok, n)
	}
	if reset != 90*time.Minute {
		t.Errorf("reset in %v, want 1h30m (next UTC midnight)", reset)
	}
	now = now.Add(2 * time.Hour) // 00:30 the next UTC day
	if ok, n, _, _ := k.Use(ctx, key); !ok || n != 1 {
		t.Errorf("next day = %v %d, want a fresh count", ok, n)
	}
}
