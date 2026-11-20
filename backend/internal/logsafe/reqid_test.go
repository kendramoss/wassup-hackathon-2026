package logsafe

import (
	"regexp"
	"testing"
)

var hashedID = regexp.MustCompile(`^h_[0-9a-f]{16}$`)

func TestRequestIDNormalisesEveryClientValue(t *testing.T) {
	mint := func() string { return "00112233445566ff" }
	if got := RequestID("", mint); got != "00112233445566ff" {
		t.Fatalf("empty header must use the server id, got %q", got)
	}
	for _, in := range []string{sentTokenRID, sentEmail, sentToken, "0123456789abcdef", "  ", "a", sentSupplier} {
		got := RequestID(in, mint)
		if !hashedID.MatchString(got) {
			t.Fatalf("RequestID(%q) = %q, want h_<16 hex>", in, got)
		}
		assertClean(t, got)
		if got != RequestID(in, mint) {
			t.Fatal("not deterministic")
		}
	}
	if RequestID("a", mint) == RequestID("b", mint) {
		t.Fatal("different inputs collided")
	}
}

func TestRequestIDHashMatchesAuditContractForm(t *testing.T) {
	// 284 contract: "h_" + first 16 hex of SHA-256 of the client value.
	if got := RequestID("abc", func() string { return "x" }); got != "h_ba7816bf8f01cfea" {
		t.Fatalf("got %q", got)
	}
}

func TestRequestIDMintedIsNotRevalidated(t *testing.T) {
	if got := RequestID("", func() string { return "feedfacefeedface" }); got != "feedfacefeedface" {
		t.Fatal(got)
	}
}
