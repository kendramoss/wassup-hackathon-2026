package store

import (
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
)

// Supplier origin declarations (SUPPLIER_DECLARATIONS, #916). Like the broker
// review links (D200) everything lives inside the case document: it is
// stored, isolated, exported and deleted with the case, with no collection of
// its own. The attached document's bytes are the one exception: they sit in
// the blob store under their hash, and only their description is kept here.

// SupplierRequest asks one supplier for the origin of one or more parts of a
// case. The link's token is never stored, only its hash.
type SupplierRequest struct {
	ID        string `json:"id"`
	TokenHash string `json:"token_hash"`
	// SupplierName and SupplierEmail are as the requester typed them. The
	// email is kept for the requester's records: no email is sent from here,
	// it is never logged and the supplier is never shown it.
	SupplierName  string     `json:"supplier_name"`
	SupplierEmail string     `json:"supplier_email,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	CreatedBy     string     `json:"created_by,omitempty"`
	ExpiresAt     time.Time  `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	FirstViewedAt *time.Time `json:"first_viewed_at,omitempty"`
	// Parts are the parts asked about, as they were named when asked.
	Parts []SupplierPart `json:"parts"`
	// Answers is every answer given on this link, oldest first. A later
	// answer for the same part replaces the earlier one in effect.
	Answers []SupplierAnswer `json:"answers,omitempty"`
}

// SupplierPart is one part asked about: its position in the product's parts
// list and its name then. When the parts list changes so the name no longer
// sits at that position, answers for it are not applied.
type SupplierPart struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
}

// SupplierAnswer is the supplier's declaration for one part.
type SupplierAnswer struct {
	Part     int    `json:"part"`
	PartName string `json:"part_name"`
	// Origin is the declared country of origin, normalised.
	Origin string `json:"origin"`
	// HTS is the supplier's tariff code for the part, optional.
	HTS     string `json:"hts,omitempty"`
	HowMade string `json:"how_made"`
	// Document is the optional attached evidence.
	Document    *SupplierDocument `json:"document,omitempty"`
	SignerName  string            `json:"signer_name"`
	SignerTitle string            `json:"signer_title"`
	// ValidUntil is the last day the declaration holds (YYYY-MM-DD).
	ValidUntil string    `json:"valid_until"`
	AnsweredAt time.Time `json:"answered_at"`
}

// SupplierDocument describes an attached file whose bytes are in the blob
// store under SHA256. It holds no content.
type SupplierDocument struct {
	Name     string `json:"name"`
	MIMEType string `json:"mime_type"`
	Bytes    int    `json:"bytes"`
	Pages    int    `json:"pages,omitempty"`
	SHA256   string `json:"sha256"`
}

// SupplierDocKey is the blob key of an attached document.
func SupplierDocKey(sha string) string { return "supplier-doc-" + sha }

// DerivedBlobs lists the blobs the case's own content points at, whether or
// not the cache index knows them: the supplier documents (#916).
func (c Case) DerivedBlobs() []blob.Ref {
	var out []blob.Ref
	for _, rq := range c.SupplierRequests {
		for _, a := range rq.Answers {
			if a.Document != nil && a.Document.SHA256 != "" {
				out = append(out, blob.Ref{Store: blob.StoreVerdicts, Key: SupplierDocKey(a.Document.SHA256)})
			}
		}
	}
	return out
}
