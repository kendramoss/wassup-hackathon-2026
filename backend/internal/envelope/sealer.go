package envelope

import (
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// MinSealKeyBytes is the shortest master key NewSealer accepts.
const MinSealKeyBytes = 32

// SealedPrefix starts every value Sealer.Seal returns.
const SealedPrefix = "sv1:"

// Sealer seals short secrets (a signing secret, a client secret) held in a
// database record, with AES-256-GCM. The sealing key is derived from the
// master key and a purpose label, so two features given one master key still
// get different keys. The context passed to Seal and Open (the record and
// field the value belongs to) is authenticated, so a sealed value copied to
// another record or field does not open. Unlike Blobs there is no key
// wrapping: the master key comes from the process's configuration.
type Sealer struct{ aead cipher.AEAD }

// NewSealer derives the sealing key for purpose from master.
func NewSealer(master []byte, purpose string) (*Sealer, error) {
	if len(master) < MinSealKeyBytes {
		return nil, fmt.Errorf("envelope: the sealing key must be at least %d bytes", MinSealKeyBytes)
	}
	if purpose == "" {
		return nil, errors.New("envelope: a sealer needs a purpose")
	}
	m := hmac.New(sha256.New, master)
	m.Write([]byte("origin-proof-seal-v1:" + purpose))
	a, err := gcm(m.Sum(nil))
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: a}, nil
}

func (s *Sealer) aad(context string) []byte { return []byte(SealedPrefix + context) }

// Seal encrypts plain, bound to context.
func (s *Sealer) Seal(plain, context string) (string, error) {
	ct, err := seal(s.aead, []byte(plain), s.aad(context))
	if err != nil {
		return "", err
	}
	return SealedPrefix + base64.RawStdEncoding.EncodeToString(ct), nil
}

// Open decrypts a value Seal returned for the same context. Its errors never
// carry the value.
func (s *Sealer) Open(sealed, context string) (string, error) {
	raw, ok := strings.CutPrefix(sealed, SealedPrefix)
	if !ok {
		return "", errors.New("envelope: not a sealed value")
	}
	ct, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		return "", errors.New("envelope: sealed value is not valid base64")
	}
	plain, err := open(s.aead, ct, s.aad(context))
	if err != nil {
		return "", errors.New("envelope: sealed value does not open with this key")
	}
	return string(plain), nil
}
