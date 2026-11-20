package httpapi

import (
	"log/slog"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/logsafe"
)

// Log attributes for ids the SERVER created (#285). logsafe accepts an id under
// the case and key_id keys only as an opaque Ref, and a Ref is only as honest
// as its caller: each use below must be an id read from the store or made by
// the generator, never one taken from a path, header, query or body. An id
// that does not have the server's shape is dropped (an empty Attr is ignored by
// slog), never logged raw.

// caseAttr names a case read from the store (or just created by the server).
func caseAttr(id string) slog.Attr {
	ref, ok := logsafe.MintedRef(id)
	if !ok {
		return slog.Attr{}
	}
	return slog.Any("case", ref)
}

// keyAttr names an API key the key store returned or issued.
func keyAttr(id string) slog.Attr {
	ref, ok := logsafe.MintedRef(id)
	if !ok {
		return slog.Attr{}
	}
	return slog.Any("key_id", ref)
}
