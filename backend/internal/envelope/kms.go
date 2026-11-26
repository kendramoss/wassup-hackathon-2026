package envelope

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"golang.org/x/oauth2/google"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
)

// KMS wraps data keys with a Cloud KMS symmetric key through the KMS REST
// API (cryptoKeys.encrypt and decrypt). The service account needs
// roles/cloudkms.cryptoKeyEncrypterDecrypter on the key, which the customer
// can revoke to make every sealed blob unreadable.
type KMS struct {
	KeyName string       // projects/P/locations/L/keyRings/R/cryptoKeys/K
	Client  *http.Client // authorised for the cloudkms scope
	BaseURL string       // https://cloudkms.googleapis.com/v1 unless set
}

var kmsKeyRE = regexp.MustCompile(`^projects/[^/]+/locations/[^/]+/keyRings/[^/]+/cryptoKeys/[^/]+$`)

// NewKMS returns a KMS provider using application-default credentials.
func NewKMS(ctx context.Context, keyName string) (*KMS, error) {
	if !kmsKeyRE.MatchString(keyName) {
		return nil, fmt.Errorf("envelope: %q is not a Cloud KMS crypto key name", keyName)
	}
	c, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/cloudkms")
	if err != nil {
		return nil, fmt.Errorf("envelope: KMS credentials: %w", err)
	}
	return &KMS{KeyName: keyName, Client: c}, nil
}

func (k *KMS) Name() string { return k.KeyName }

func (k *KMS) Wrap(ctx context.Context, dek []byte) ([]byte, error) {
	var out struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := k.call(ctx, "encrypt", map[string]string{"plaintext": base64.StdEncoding.EncodeToString(dek)}, &out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Ciphertext)
}

func (k *KMS) Unwrap(ctx context.Context, w []byte) ([]byte, error) {
	var out struct {
		Plaintext string `json:"plaintext"`
	}
	if err := k.call(ctx, "decrypt", map[string]string{"ciphertext": base64.StdEncoding.EncodeToString(w)}, &out); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(out.Plaintext)
}

func (k *KMS) call(ctx context.Context, verb string, in, out any) error {
	base := k.BaseURL
	if base == "" {
		base = "https://cloudkms.googleapis.com/v1"
	}
	body, _ := json.Marshal(in)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/"+k.KeyName+":"+verb, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := k.Client.Do(req)
	if err != nil {
		return fmt.Errorf("envelope: KMS %s: %w", verb, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("envelope: KMS %s: HTTP %d", verb, resp.StatusCode)
	}
	return json.Unmarshal(b, out)
}

// Mode is BLOB_ENCRYPTION.
type Mode string

const (
	Off       Mode = "off"
	LocalMode Mode = "local"
	KMSMode   Mode = "kms"
)

// Config is the blob-encryption setting.
type Config struct {
	Mode     Mode
	LocalKey []byte // BLOB_ENCRYPTION_KEY, base64, local mode only
	KMSKey   string // BLOB_ENCRYPTION_KMS_KEY, kms mode only
}

// ConfigFromEnv reads BLOB_ENCRYPTION (off, local or kms; default off),
// BLOB_ENCRYPTION_KEY and BLOB_ENCRYPTION_KMS_KEY. Local mode is refused on
// Cloud Run (K_SERVICE set): a key in the environment is not
// customer-managed.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	c := Config{Mode: Mode(strings.ToLower(strings.TrimSpace(getenv("BLOB_ENCRYPTION"))))}
	if c.Mode == "" {
		c.Mode = Off
	}
	switch c.Mode {
	case Off:
		return c, nil
	case LocalMode:
		if getenv("K_SERVICE") != "" {
			return c, errors.New("BLOB_ENCRYPTION=local is for development; use kms on Cloud Run")
		}
		k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(getenv("BLOB_ENCRYPTION_KEY")))
		if err != nil || len(k) != 32 {
			return c, errors.New("BLOB_ENCRYPTION=local needs BLOB_ENCRYPTION_KEY: 32 bytes, base64")
		}
		c.LocalKey = k
	case KMSMode:
		c.KMSKey = strings.TrimSpace(getenv("BLOB_ENCRYPTION_KMS_KEY"))
		if !kmsKeyRE.MatchString(c.KMSKey) {
			return c, errors.New("BLOB_ENCRYPTION=kms needs BLOB_ENCRYPTION_KMS_KEY = projects/P/locations/L/keyRings/R/cryptoKeys/K")
		}
	default:
		return c, fmt.Errorf("BLOB_ENCRYPTION must be off, local or kms, not %q", c.Mode)
	}
	return c, nil
}

// Provider builds the configured provider; nil when off.
func (c Config) Provider(ctx context.Context) (Provider, error) {
	switch c.Mode {
	case LocalMode:
		return NewLocal(c.LocalKey)
	case KMSMode:
		return NewKMS(ctx, c.KMSKey)
	}
	return nil, nil
}

// Wrap returns inner encrypted with p, or inner unchanged when p is nil.
func Wrap(inner blob.Store, p Provider) blob.Store {
	if p == nil || inner == nil {
		return inner
	}
	return NewBlobs(inner, p)
}
