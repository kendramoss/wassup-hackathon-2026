package logsafe

import (
	"crypto/sha256"
	"encoding/hex"
)

// RequestID decides the request id to log and echo. A client-supplied id is
// UNTRUSTED whatever its shape (a token or an email can look like a valid id),
// so any non-empty incoming value is replaced by "h_" plus the first 16 hex
// characters of its SHA-256, the same form the #284 audit contract stores. It
// is stable (the client can correlate by hashing its own value) and the raw
// value is never logged or echoed. With no incoming value the server id from
// mint is used. No secret is involved; a hash of a guessable value is guessable,
// which is why this is a log-hygiene measure, not anonymisation.
func RequestID(incoming string, mint func() string) string {
	if incoming == "" {
		return mint()
	}
	sum := sha256.Sum256([]byte(incoming))
	return "h_" + hex.EncodeToString(sum[:])[:16]
}
