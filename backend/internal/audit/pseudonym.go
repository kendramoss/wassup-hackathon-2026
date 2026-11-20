package audit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// MinKeyBytes is the shortest AUDIT_ACTOR_KEY accepted.
const MinKeyBytes = 32

// Pseudonymizer turns verified identities into keyed pseudonyms and MACs
// cursors. The key comes from the operator's secret store (AUDIT_ACTOR_KEY);
// it is never generated here, never logged and never stored in the trail. A
// pseudonym is not anonymisation: whoever holds the key can confirm a guessed
// identity. Rotation is a new key and a new key ID: old events keep their
// pseudonyms (the hash chain covers them), correlating across the change needs
// the retained old key.
type Pseudonymizer struct {
	keyID string
	key   []byte
}

// NewPseudonymizer checks the key ID and key strength.
func NewPseudonymizer(keyID string, key []byte) (*Pseudonymizer, error) {
	if !keyIDPattern.MatchString(keyID) {
		return nil, errors.New("audit: the actor key id must be 1 to 8 lower-case letters or digits")
	}
	if len(key) < MinKeyBytes {
		return nil, fmt.Errorf("audit: the actor key must be at least %d bytes", MinKeyBytes)
	}
	return &Pseudonymizer{keyID: keyID, key: append([]byte(nil), key...)}, nil
}

func (p *Pseudonymizer) mac(domain string, parts ...string) []byte {
	m := hmac.New(sha256.New, p.key)
	m.Write([]byte(domain))
	for _, s := range parts {
		m.Write([]byte{0})
		m.Write([]byte(s))
	}
	return m.Sum(nil)
}

// Actor is the pseudonym of a verified identity: a<keyid>_<32 hex>. id is an
// email (user) or an API key ID (apikey) and is not retained.
func (p *Pseudonymizer) Actor(kind ActorKind, id string) string {
	return "a" + p.keyID + "_" + hex.EncodeToString(p.mac("actor", string(kind), id)[:16])
}

// Opaque is the keyed stand-in for an ID the requester supplied and the server
// did not confirm: h_<16 hex>. Such an ID may carry any text a client chose,
// so it is never stored raw.
func (p *Pseudonymizer) Opaque(kind, id string) string {
	return "h_" + hex.EncodeToString(p.mac("object", kind, id)[:8])
}

// maxCursor bounds the opaque input we are willing to decode.
const maxCursor = 2048

// Cursor is a position in a listing or a verification. It is opaque to
// clients and MAC-bound to the tenant, the kind, and a digest of the filters
// and time bounds, so it cannot be moved to another tenant or reused under
// different filters.
type Cursor struct {
	Kind   string `json:"k"`
	Desc   bool   `json:"d,omitempty"`
	Seq    uint64 `json:"s"`
	Hash   string `json:"h,omitempty"`
	TimeUS int64  `json:"t,omitempty"`
	Filter string `json:"f,omitempty"`
	// Open are the intents of a verification still without a completion, and
	// OpenOverflow says there were too many to carry (see Unresolved).
	Open         []uint64 `json:"o,omitempty"`
	OpenOverflow bool     `json:"x,omitempty"`
	// Upper and UpperHash freeze the head a verification started from.
	Upper     uint64 `json:"u,omitempty"`
	UpperHash string `json:"uh,omitempty"`
	// AsOfUS freezes the clock used to age intents.
	AsOfUS int64 `json:"a,omitempty"`
}

var errCursor = errors.New("audit: invalid cursor")

// EncodeCursor returns the opaque, MAC'd form of c for tenant.
func (p *Pseudonymizer) EncodeCursor(tenant string, c Cursor) string {
	body, _ := json.Marshal(c)
	b := base64.RawURLEncoding.EncodeToString(body)
	return b + "." + hex.EncodeToString(p.mac("cursor", tenant, b)[:12])
}

// DecodeCursor checks the MAC, tenant, kind and filter digest.
func (p *Pseudonymizer) DecodeCursor(tenant, kind, filter, tok string) (Cursor, error) {
	if len(tok) > maxCursor {
		return Cursor{}, errCursor
	}
	b, sig, ok := cutLast(tok, '.')
	if !ok {
		return Cursor{}, errCursor
	}
	want := hex.EncodeToString(p.mac("cursor", tenant, b)[:12])
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return Cursor{}, errCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(b)
	if err != nil {
		return Cursor{}, errCursor
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil || c.Kind != kind || c.Filter != filter {
		return Cursor{}, errCursor
	}
	return c, nil
}

func cutLast(s string, sep byte) (before, after string, ok bool) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

// ErrCursor is returned for any bad cursor.
var ErrCursor = errCursor
