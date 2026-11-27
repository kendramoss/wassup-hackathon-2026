package sso

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// MinKeyBytes is the shortest master key accepted (SSO_SESSION_KEY).
const MinKeyBytes = 32

// Keys derives the session signing key and the secret sealing key from one
// master key, so neither is used for the other's purpose.
type Keys struct {
	sign []byte
	seal []byte
}

// NewKeys checks and derives from the master key.
func NewKeys(master []byte) (Keys, error) {
	if len(master) < MinKeyBytes {
		return Keys{}, fmt.Errorf("sso: the session key must be at least %d bytes", MinKeyBytes)
	}
	return Keys{sign: derive(master, "origin-proof-sso-session-v1"), seal: derive(master, "origin-proof-sso-secret-v1")}, nil
}

func derive(master []byte, label string) []byte {
	m := hmac.New(sha256.New, master)
	m.Write([]byte(label))
	return m.Sum(nil)
}

const sealPrefix = "v1:"

// Seal encrypts a secret for storage.
func (k Keys) Seal(plain string) (string, error) {
	block, err := aes.NewCipher(k.seal)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plain), []byte(sealPrefix))
	return sealPrefix + base64.RawStdEncoding.EncodeToString(ct), nil
}

// Open decrypts a sealed secret.
func (k Keys) Open(sealed string) (string, error) {
	raw, ok := strings.CutPrefix(sealed, sealPrefix)
	if !ok {
		return "", errors.New("sso: unknown sealed secret format")
	}
	ct, err := base64.RawStdEncoding.DecodeString(raw)
	if err != nil {
		return "", errors.New("sso: sealed secret is not valid base64")
	}
	block, err := aes.NewCipher(k.seal)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ct) < gcm.NonceSize() {
		return "", errors.New("sso: sealed secret is too short")
	}
	plain, err := gcm.Open(nil, ct[:gcm.NonceSize()], ct[gcm.NonceSize():], []byte(sealPrefix))
	if err != nil {
		return "", errors.New("sso: sealed secret does not open with this key")
	}
	return string(plain), nil
}
