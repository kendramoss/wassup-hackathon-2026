package audit

import (
	"strings"
	"testing"
)

func TestPseudonymIsKeyedStableAndOneWay(t *testing.T) {
	p := pseud(t)
	a := p.Actor(ActorUser, "alice@example.com")
	if !actorPattern.MatchString(a) || !strings.HasPrefix(a, "a1_") {
		t.Fatalf("pseudonym = %q", a)
	}
	if strings.Contains(a, "alice") || a != p.Actor(ActorUser, "alice@example.com") {
		t.Error("pseudonym leaks or is unstable")
	}
	if a == p.Actor(ActorUser, "bob@example.com") {
		t.Error("two users share a pseudonym")
	}
	if a == p.Actor(ActorAPIKey, "alice@example.com") {
		t.Error("kind is not domain-separated")
	}
	other, _ := NewPseudonymizer("1", []byte(strings.Repeat("z", 40)))
	if a == other.Actor(ActorUser, "alice@example.com") {
		t.Error("different keys give the same pseudonym: it is not keyed")
	}
	rot, _ := NewPseudonymizer("2", []byte(strings.Repeat("k", 40)))
	if !strings.HasPrefix(rot.Actor(ActorUser, "alice@example.com"), "a2_") {
		t.Error("key id missing from pseudonym")
	}
}

func TestPseudonymizerRefusesWeakKeysAndBadIDs(t *testing.T) {
	if _, err := NewPseudonymizer("1", []byte("short")); err == nil {
		t.Error("short key accepted")
	}
	if _, err := NewPseudonymizer("", []byte(strings.Repeat("k", 40))); err == nil {
		t.Error("empty key id accepted")
	}
	if _, err := NewPseudonymizer("Key 1!", []byte(strings.Repeat("k", 40))); err == nil {
		t.Error("bad key id accepted")
	}
}

func TestCursorIsTenantKindAndFilterBound(t *testing.T) {
	c := pseud(t)
	const tn, other = "ws:ws_0123456789ab", "ws:ws_ba9876543210"
	tok := c.EncodeCursor(tn, Cursor{Kind: "list", Desc: true, Seq: 42, Hash: strings.Repeat("a", 64), Filter: "f1"})
	got, err := c.DecodeCursor(tn, "list", "f1", tok)
	if err != nil || got.Seq != 42 || !got.Desc || got.Hash != strings.Repeat("a", 64) {
		t.Fatalf("round trip = %+v %v", got, err)
	}
	if _, err := c.DecodeCursor(other, "list", "f1", tok); err == nil {
		t.Error("cursor accepted for another tenant")
	}
	if _, err := c.DecodeCursor(tn, "verify", "f1", tok); err == nil {
		t.Error("list cursor accepted by verify")
	}
	if _, err := c.DecodeCursor(tn, "list", "f2", tok); err == nil {
		t.Error("cursor accepted under different filters/time bounds: a client could skip or reuse a filter")
	}
	if _, err := c.DecodeCursor(tn, "list", "f1", tok[:len(tok)-2]+"AA"); err == nil {
		t.Error("tampered cursor accepted")
	}
	if _, err := c.DecodeCursor(tn, "list", "f1", "!!not base64"); err == nil {
		t.Error("garbage cursor accepted")
	}
	if _, err := c.DecodeCursor(tn, "list", "f1", strings.Repeat("A", 2100)); err == nil {
		t.Error("oversized cursor accepted")
	}
	otherKey, _ := NewPseudonymizer("1", []byte(strings.Repeat("z", 40)))
	if _, err := otherKey.DecodeCursor(tn, "list", "f1", tok); err == nil {
		t.Error("cursor from another key accepted")
	}
}

func TestOpaqueHidesUntrustedIDs(t *testing.T) {
	p := pseud(t)
	o := p.Opaque("case", "alice-secret-supplier-facts")
	if !strings.HasPrefix(o, "h_") || len(o) != 18 || strings.Contains(o, "alice") {
		t.Fatalf("opaque = %q", o)
	}
	if o != p.Opaque("case", "alice-secret-supplier-facts") || o == p.Opaque("workspace", "alice-secret-supplier-facts") {
		t.Error("opaque is unstable or not domain separated")
	}
	other, _ := NewPseudonymizer("1", []byte(strings.Repeat("z", 40)))
	if o == other.Opaque("case", "alice-secret-supplier-facts") {
		t.Error("opaque is not keyed")
	}
}
