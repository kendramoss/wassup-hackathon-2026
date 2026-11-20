package logsafe

import (
	"regexp"
	"strings"
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

func TestPlantedClientIDsNeverLoggedByteForByte(t *testing.T) {
	l, buf, _ := newTest(t, DefaultPolicy())
	for _, in := range []string{"0123456789abcdef", "h_0123456789abcdef", sentTokenRID, "deadbeefdeadbeefdeadbeef"} {
		buf.Reset()
		ref := RequestRef(in, func() string { return "ffffffffffffffff" })
		l.InfoContext(bg, "request", "request_id", ref)
		if strings.Contains(buf.String(), `"`+in+`"`) || ref.String() == in || !hashedID.MatchString(ref.String()) {
			t.Fatalf("client id %q logged raw: %s", in, buf.String())
		}
	}
}

func TestRequestRefMintGuards(t *testing.T) {
	for _, mint := range []func() string{nil, func() string { return "" }, func() string { return sentEmail }, func() string { return "ABCDEF12" }} {
		got := RequestRef("", mint).String()
		if len(got) != 16 || strings.Contains(got, "@") {
			t.Fatalf("bad fallback id %q", got)
		}
	}
	if RequestRef("", func() string { return "0123abcd" }).String() != "0123abcd" {
		t.Fatal("valid server id changed")
	}
}
