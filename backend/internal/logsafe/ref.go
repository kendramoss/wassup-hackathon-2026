package logsafe

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"regexp"
)

// Ref is an OPAQUE, already-normalised identifier that may be logged under
// the request_id, case and key_id keys. Its only fields are unexported, so
// code outside this package cannot build one from an arbitrary string: it must
// come from RequestRef (client header or server mint) or MintedRef (an id the
// server itself created). A raw string under those keys is always masked,
// because the shape of an id (16 hex characters, h_ plus 16 hex) cannot prove
// that it was not planted by a client or copied from a secret.
type Ref struct{ v string }

var (
	mintRE   = regexp.MustCompile(`^[0-9a-f]{8,32}$`)
	mintedRE = regexp.MustCompile(`^([0-9a-f]{16}|(case|ws)_[0-9a-f]{8,32})$`)
)

// RequestRef normalises the request id ONCE, at the top boundary. Any
// non-empty client value, whatever its shape, becomes "h_" + the first 16 hex
// of its SHA-256 (the form the #284 audit contract stores). With no client
// value the id comes from mint (server minted, kept as is, never re-hashed); a
// nil mint, or one returning something other than 8..32 lowercase hex, falls
// back to 16 random hex characters. The result is the one value to log, echo
// in the response header and hand to the audit; pass the Ref (or its String)
// on rather than re-reading the header.
func RequestRef(incoming string, mint func() string) Ref {
	if incoming != "" {
		sum := sha256.Sum256([]byte(incoming))
		return Ref{"h_" + hex.EncodeToString(sum[:])[:16]}
	}
	if mint != nil {
		if id := mint(); mintRE.MatchString(id) {
			return Ref{id}
		}
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	return Ref{hex.EncodeToString(b[:])}
}

// MintedRef wraps an id the SERVER created and read back from its own store or
// generator (a case id, workspace id or API-key id), never one taken from a
// request path, header, query or body. The shape check only rejects typos; the
// caller is the one asserting provenance, which is why this is a named
// constructor and not a rule on the string.
func MintedRef(id string) (Ref, bool) {
	if !mintedRE.MatchString(id) {
		return Ref{}, false
	}
	return Ref{id}, true
}

// String returns the normalised id (for the response header and the audit).
func (r Ref) String() string { return r.v }

// IsZero reports whether r was not built by a constructor.
func (r Ref) IsZero() bool { return r.v == "" }

// LogValue lets any slog handler print a Ref; the sanitising handler trusts
// only this type, never an arbitrary LogValuer.
func (r Ref) LogValue() slog.Value { return slog.StringValue(r.v) }

func refRule(v slog.Value) (slog.Value, bool) {
	if v.Kind() != slog.KindLogValuer {
		return v, false
	}
	r, ok := v.Any().(Ref)
	if !ok || r.IsZero() {
		return v, false
	}
	return slog.StringValue(r.v), true
}
