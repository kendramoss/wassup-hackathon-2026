package store

import "time"

// Broker workflow (BROKER_WORKFLOW): a review queue, collaboration on a case
// and a CBP eRulings-ready filing package. Like the broker review links
// (D200) everything lives inside the case document: it is stored, isolated,
// exported and deleted with the case, with no collection of its own.

// QueueItem is a case's place in the human review queue (#881).
type QueueItem struct {
	State    string    `json:"state"` // queued | assigned | in_review | decided
	Priority string    `json:"priority"`
	Reason   string    `json:"reason"`
	QueuedBy string    `json:"queued_by,omitempty"`
	QueuedAt time.Time `json:"queued_at"`
	// DueAt is QueuedAt plus the priority's review target.
	DueAt      time.Time      `json:"due_at"`
	Assignee   string         `json:"assignee,omitempty"`
	AssignedAt *time.Time     `json:"assigned_at,omitempty"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	Decision   *QueueDecision `json:"decision,omitempty"`
	// Log is every transition, oldest first.
	Log []QueueStep `json:"log,omitempty"`
}

// QueueDecision is the reviewer's outcome. Reason is required.
type QueueDecision struct {
	Outcome string    `json:"outcome"` // approve | reject | changes
	Reason  string    `json:"reason"`
	By      string    `json:"by"`
	At      time.Time `json:"at"`
	// OnTime reports whether it was decided by DueAt.
	OnTime bool `json:"on_time"`
}

// QueueStep is one transition of a queue item.
type QueueStep struct {
	Action string    `json:"action"`
	By     string    `json:"by"`
	At     time.Time `json:"at"`
	Detail string    `json:"detail,omitempty"`
}

// Comment is one message in a case's discussion (#882). A reply names its
// thread's first comment in ParentID.
type Comment struct {
	ID       string    `json:"id"`
	ParentID string    `json:"parent_id,omitempty"`
	Author   string    `json:"author"`
	Role     string    `json:"role"` // owner | editor | viewer | broker | reviewer | external_broker
	Body     string    `json:"body"`
	At       time.Time `json:"at"`
}

// Approval is one sign-off on the case's ruling request.
type Approval struct {
	Decision string    `json:"decision"` // approve | reject | changes
	Reason   string    `json:"reason,omitempty"`
	By       string    `json:"by"`
	Role     string    `json:"role"`
	At       time.Time `json:"at"`
}

// Collaborator gives a person a role on one case, beside their workspace role.
type Collaborator struct {
	Email   string    `json:"email"`
	Role    string    `json:"role"` // broker | reviewer
	AddedBy string    `json:"added_by"`
	AddedAt time.Time `json:"added_at"`
}

// Filing is the case's ruling-request filing package and its tracking (#879).
// Nothing is submitted to CBP from here: the broker files through eRulings
// and records the status.
type Filing struct {
	Status      string          `json:"status"` // prepared | submitted | acknowledged | ruling_issued | withdrawn
	PreparedAt  time.Time       `json:"prepared_at"`
	PreparedBy  string          `json:"prepared_by,omitempty"`
	Submitter   FilingSubmitter `json:"submitter"`
	Letter      string          `json:"letter"`
	Elements    []FilingElement `json:"elements"`
	Ready       bool            `json:"ready"`
	Attachments []FilingFile    `json:"attachments,omitempty"`
	// PackageSHA256 is the hash of the package JSON when it was prepared.
	PackageSHA256 string       `json:"package_sha256"`
	CBPReference  string       `json:"cbp_reference,omitempty"`
	RulingNumber  string       `json:"ruling_number,omitempty"`
	History       []FilingStep `json:"history,omitempty"`
}

// FilingSubmitter is what the requester supplies for 19 CFR 177.2(b)(1)-(2)
// and the certification; the case does not know it.
type FilingSubmitter struct {
	RequesterName    string `json:"requester_name"`
	RequesterAddress string `json:"requester_address"`
	RequesterEmail   string `json:"requester_email,omitempty"`
	RequesterPhone   string `json:"requester_phone,omitempty"`
	Interest         string `json:"interest"` // importer | exporter | manufacturer | agent | other
	ImporterOfRecord string `json:"importer_of_record"`
	PortOfEntry      string `json:"port_of_entry,omitempty"`
	// NoPriorConsideration confirms the 177.2(b)(2)(ii) statement.
	NoPriorConsideration bool `json:"no_prior_consideration"`
	// Confidential is "none" or the information to withhold under 177.2(b)(7).
	Confidential string `json:"confidential"`
	SignerName   string `json:"signer_name"`
	SignerTitle  string `json:"signer_title,omitempty"`
	// Manufacturers maps a plant name in the case to its legal name, address
	// and MID code.
	Manufacturers        map[string]string `json:"manufacturers,omitempty"`
	CountryOfExportation string            `json:"country_of_exportation,omitempty"`
	FirstShipment        string            `json:"first_shipment,omitempty"`
	ProductModel         string            `json:"product_model,omitempty"`
	ProductDescription   string            `json:"product_description,omitempty"`
	HTSReference         string            `json:"hts_reference,omitempty"`
	Marking              string            `json:"marking,omitempty"`
	LegalBasis           string            `json:"legal_basis,omitempty"`
	PriorRulingNote      string            `json:"prior_ruling_note,omitempty"`
	PriorAdvice          string            `json:"prior_advice,omitempty"`
	// Series is "is" or "is not": whether the transaction is one of a series.
	Series string `json:"series,omitempty"`
	Sample string `json:"sample,omitempty"`
}

// FilingElement is one required content of the request and whether the
// package has it.
type FilingElement struct {
	Key      string `json:"key"`
	Citation string `json:"citation"`
	Label    string `json:"label"`
	Status   string `json:"status"` // present | missing
	Detail   string `json:"detail,omitempty"`
	Required bool   `json:"required"`
}

// FilingFile is one file of the package: generated, or to be attached by
// the requester (no bytes stored for those, only the name and hash if given).
type FilingFile struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"` // generated | to_attach
	Description string `json:"description"`
	SHA256      string `json:"sha256,omitempty"`
	Bytes       int    `json:"bytes,omitempty"`
}

// FilingStep is one status change.
type FilingStep struct {
	Status    string    `json:"status"`
	By        string    `json:"by,omitempty"`
	At        time.Time `json:"at"`
	Reference string    `json:"reference,omitempty"`
	Note      string    `json:"note,omitempty"`
}
