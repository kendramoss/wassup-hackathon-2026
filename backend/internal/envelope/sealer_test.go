package envelope

import (
	"strings"
	"testing"
)

func TestSealerRoundTripBindsKeyPurposeAndContext(t *testing.T) {
	s, err := NewSealer(key(1), "test-purpose")
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Seal("whsec_plain", "ctx-a")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.Seal("whsec_plain", "ctx-a")
	if a == b {
		t.Error("two seals of one value are equal: nonce reused")
	}
	if strings.Contains(a, "whsec_plain") || !strings.HasPrefix(a, SealedPrefix) {
		t.Errorf("sealed = %q", a)
	}
	if got, err := s.Open(a, "ctx-a"); err != nil || got != "whsec_plain" {
		t.Fatalf("open = %q %v", got, err)
	}
	if _, err := s.Open(a, "ctx-b"); err == nil {
		t.Error("opens under another context")
	}
	other, _ := NewSealer(key(2), "test-purpose")
	if _, err := other.Open(a, "ctx-a"); err == nil {
		t.Error("opens under another key")
	}
	diff, _ := NewSealer(key(1), "another-purpose")
	if _, err := diff.Open(a, "ctx-a"); err == nil {
		t.Error("opens under another purpose")
	}
	for _, bad := range []string{"", "whsec_plain", SealedPrefix, SealedPrefix + "!!", SealedPrefix + "AAAA"} {
		if _, err := s.Open(bad, "ctx-a"); err == nil {
			t.Errorf("opened %q", bad)
		}
	}
	if _, err := NewSealer(key(1)[:31], "p"); err == nil || strings.Contains(err.Error(), "\x01") {
		t.Errorf("short key = %v", err)
	}
	if _, err := NewSealer(key(1), ""); err == nil {
		t.Error("empty purpose accepted")
	}
}
