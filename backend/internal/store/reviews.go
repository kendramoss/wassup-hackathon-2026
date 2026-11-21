package store

import (
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// BrokerReview is one broker review link on a case (D200). It lives inside
// the case document, so it is stored, read, isolated and deleted with the
// case: no collection of its own, no query, no index. The link's secret is
// never stored; TokenHash is its SHA-256. Draft is a snapshot taken when the
// link was made, so the broker reviews exactly what the owner shared.
type BrokerReview struct {
	ID            string           `json:"id"`
	TokenHash     string           `json:"token_hash"`
	CreatedAt     time.Time        `json:"created_at"`
	ExpiresAt     time.Time        `json:"expires_at"`
	RevokedAt     *time.Time       `json:"revoked_at,omitempty"`
	FirstViewedAt *time.Time       `json:"first_viewed_at,omitempty"`
	Draft         ReviewDraft      `json:"draft"`
	Decisions     []ReviewDecision `json:"decisions,omitempty"`
}

// ReviewDraft is what the broker sees, frozen at link creation.
type ReviewDraft struct {
	Subject     string            `json:"subject"` // setup | plan
	Label       string            `json:"label"`
	Format      string            `json:"format"`
	ProductName string            `json:"product_name"`
	Product     *domain.Product   `json:"product,omitempty"`
	Prediction  domain.Prediction `json:"prediction"`
	Plan        *ReviewPlan       `json:"plan,omitempty"`
	Content     string            `json:"content"`
	Notes       []string          `json:"notes,omitempty"`
}

// ReviewPlan is the plan a draft argues for, when it is not the current setup.
type ReviewPlan struct {
	Description     string   `json:"description"`
	PredictedOrigin string   `json:"predicted_origin"`
	Supporting      string   `json:"supporting_ruling,omitempty"`
	Counter         string   `json:"counter_ruling,omitempty"`
	CheckConfidence *float64 `json:"check_confidence,omitempty"`
	BelowBar        bool     `json:"below_bar"`
}

// ReviewDecision is one answer from the broker. Comment is plain text.
type ReviewDecision struct {
	Decision string    `json:"decision"` // approve | changes | file_request
	Comment  string    `json:"comment,omitempty"`
	At       time.Time `json:"at"`
}
