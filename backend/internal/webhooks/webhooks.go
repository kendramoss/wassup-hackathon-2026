// Package webhooks delivers signed HTTPS callbacks for workspace events
// (#915, D936). A workspace admin registers endpoints; the server posts the
// events each endpoint subscribed to, signed with HMAC-SHA256, retries a
// failed delivery on a backoff schedule and keeps a delivery log. Delivery is
// at least once: a receiver deduplicates on the X-Webhook-Id header.
package webhooks

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Event types a workspace can subscribe to.
const (
	EventRulingWatchHit = "ruling_watch.hit"
	EventReviewDecision = "review.decision"
	EventFilingStatus   = "filing.status"
	EventBudgetAlert    = "budget.alert"
	EventBatchFinished  = "batch.finished"
	// EventEvalDrift is sent to every workspace when a monthly re-score shows
	// accuracy below the earlier months (#921, D948). It belongs to no workspace.
	EventEvalDrift = "eval.drift"
	// EventTest is sent by the test-ping route only; it is not subscribable.
	EventTest = "webhook.test"
)

// Events lists the subscribable event types.
var Events = []string{EventRulingWatchHit, EventReviewDecision, EventFilingStatus, EventBudgetAlert, EventBatchFinished, EventEvalDrift}

// Limits.
const (
	MaxEndpoints       = 10
	MaxURLLen          = 2000
	MaxDescriptionLen  = 200
	MaxDeliveriesKept  = 100 // per endpoint
	MaxPayloadBytes    = 16 << 10
	SecretPrefix       = "whsec_"
	SignatureHeader    = "X-Webhook-Signature"
	IDHeader           = "X-Webhook-Id"
	EventHeader        = "X-Webhook-Event"
	RotationGrace      = 24 * time.Hour
	SignatureTolerance = 5 * time.Minute
)

// Delivery statuses.
const (
	StatusPending   = "pending"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"
)

// Errors.
var (
	ErrNotFound = errors.New("no such webhook endpoint")
	ErrInvalid  = errors.New("invalid input")
	ErrFull     = errors.New("this workspace has the most webhook endpoints it can have")
)

// Endpoint is one registered callback URL of a workspace. Secret is what the
// signature is made with, so it is stored. The JSON tags are for the file
// store only: the API answers with its own view, never this struct.
type Endpoint struct {
	ID          string    `firestore:"id" json:"id"`
	Workspace   string    `firestore:"workspace" json:"workspace"`
	URL         string    `firestore:"url" json:"url"`
	Events      []string  `firestore:"events" json:"events"`
	Description string    `firestore:"description" json:"description"`
	Enabled     bool      `firestore:"enabled" json:"enabled"`
	Secret      string    `firestore:"secret" json:"secret"`
	PrevSecret  string    `firestore:"prev_secret,omitempty" json:"prev_secret,omitempty"`
	PrevUntil   time.Time `firestore:"prev_until,omitempty" json:"prev_until,omitempty"`
	CreatedBy   string    `firestore:"created_by" json:"created_by"`
	CreatedAt   time.Time `firestore:"created_at" json:"created_at"`
	UpdatedAt   time.Time `firestore:"updated_at" json:"updated_at"`
	RotatedAt   time.Time `firestore:"rotated_at,omitempty" json:"rotated_at,omitempty"`
}

// Subscribed reports whether the endpoint takes events of type typ.
func (e Endpoint) Subscribed(typ string) bool { return e.Enabled && slices.Contains(e.Events, typ) }

// SecretHint is the last four characters of the secret, to tell two apart.
func (e Endpoint) SecretHint() string { return hint(e.Secret) }

func hint(s string) string {
	if len(s) < 4 {
		return ""
	}
	return s[len(s)-4:]
}

// Attempt is one try to deliver. Error is a class, never response text.
type Attempt struct {
	At         time.Time `firestore:"at" json:"at"`
	StatusCode int       `firestore:"status_code" json:"status_code"`
	DurationMS int64     `firestore:"duration_ms" json:"duration_ms"`
	Error      string    `firestore:"error,omitempty" json:"error,omitempty"`
}

// Delivery is one event for one endpoint, with its attempts. Payload is kept
// to retry; the API's delivery log leaves it out.
type Delivery struct {
	ID          string    `firestore:"id" json:"id"`
	EndpointID  string    `firestore:"endpoint_id" json:"endpoint_id"`
	Workspace   string    `firestore:"workspace" json:"workspace"`
	EventID     string    `firestore:"event_id" json:"event_id"`
	Event       string    `firestore:"event" json:"event"`
	Test        bool      `firestore:"test" json:"test"`
	Status      string    `firestore:"status" json:"status"`
	Attempts    []Attempt `firestore:"attempts" json:"attempts"`
	NextAttempt time.Time `firestore:"next_attempt_at" json:"next_attempt_at,omitempty"`
	CreatedAt   time.Time `firestore:"created_at" json:"created_at"`
	Payload     string    `firestore:"payload" json:"payload"`
}

// Envelope is the JSON body of every callback.
type Envelope struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	Workspace string    `json:"workspace"`
	Data      any       `json:"data"`
}

// NewID returns a random ID with prefix: wh_, dl_ or evt_.
func NewID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b)
}

// NewSecret returns a fresh signing secret.
func NewSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return SecretPrefix + base64.RawURLEncoding.EncodeToString(b)
}

func mac(secret string, t int64, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(strconv.FormatInt(t, 10)))
	h.Write([]byte{'.'})
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// Sign returns the X-Webhook-Signature value for body at time at: "t=<unix>"
// and one "v1=<hex>" per secret, the HMAC-SHA256 of "<unix>.<body>". During a
// rotation it carries a signature for the new and the previous secret, so a
// receiver switches without dropping a delivery.
func Sign(at time.Time, body []byte, secrets ...string) string {
	t := at.Unix()
	parts := []string{"t=" + strconv.FormatInt(t, 10)}
	for _, s := range secrets {
		if s != "" {
			parts = append(parts, "v1="+mac(s, t, body))
		}
	}
	return strings.Join(parts, ",")
}

// Verify checks a signature header the way a receiver should: the timestamp
// is within tolerance of now and one v1 value matches the secret.
func Verify(header string, body []byte, secret string, tolerance time.Duration, now time.Time) error {
	var t int64
	var sigs []string
	for _, p := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(p), "=")
		switch k {
		case "t":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return errors.New("webhooks: bad timestamp")
			}
			t = n
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if t == 0 || len(sigs) == 0 {
		return errors.New("webhooks: malformed signature")
	}
	if d := now.Sub(time.Unix(t, 0)); d > tolerance || d < -tolerance {
		return errors.New("webhooks: timestamp outside tolerance")
	}
	want := mac(secret, t, body)
	for _, s := range sigs {
		if hmac.Equal([]byte(s), []byte(want)) {
			return nil
		}
	}
	return errors.New("webhooks: signature mismatch")
}

// ValidateEvents checks a subscription list and returns it sorted and
// without duplicates.
func ValidateEvents(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("%w: give at least one event; choose from %s", ErrInvalid, strings.Join(Events, ", "))
	}
	var out []string
	for _, e := range in {
		if !slices.Contains(Events, e) {
			return nil, fmt.Errorf("%w: unknown event %q; choose from %s", ErrInvalid, e, strings.Join(Events, ", "))
		}
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	slices.Sort(out)
	return out, nil
}

// ValidateDescription trims and bounds a description.
func ValidateDescription(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) > MaxDescriptionLen {
		return "", fmt.Errorf("%w: description is longer than %d characters", ErrInvalid, MaxDescriptionLen)
	}
	return s, nil
}
