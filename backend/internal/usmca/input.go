package usmca

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Bounds.
const (
	MaxMaterials  = 200
	MaxMaterialID = 64
	DestinationUS = "US"
	RegimeUSMCA   = "usmca"
)

// ErrInput is the sentinel for malformed input (HTTP 400 at the API layer).
var ErrInput = errors.New("invalid usmca input")

// InputError names the offending field and wraps ErrInput.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return "usmca input: " + e.Field + ": " + e.Message }

// Unwrap lets errors.Is(err, ErrInput) match.
func (e *InputError) Unwrap() error { return ErrInput }

func inputErr(field, format string, a ...any) error {
	return &InputError{Field: field, Message: fmt.Sprintf(format, a...)}
}

// OriginClaim is a caller-supplied originating-status claim for a material.
// The tool does not certify it. The empty value means unknown.
type OriginClaim string

// Origin claims.
const (
	ClaimOriginating    OriginClaim = "originating"
	ClaimNonOriginating OriginClaim = "non_originating"
	ClaimUnknown        OriginClaim = "unknown"
)

// Material is one input material. An empty HS means the code is unknown.
type Material struct {
	ID          string      `json:"id,omitempty"`
	HS          string      `json:"hs,omitempty"`
	Originating OriginClaim `json:"originating,omitempty"`
}

// ProvisionClaim is the caller's statement about one unsupported special
// provision. The empty value means unconfirmed, never "not applicable".
type ProvisionClaim string

// Provision claims.
const (
	ProvisionNotApplicable ProvisionClaim = "not_applicable"
	ProvisionApplies       ProvisionClaim = "applies"
)

// Profile holds the explicit simple-goods confirmations.
type Profile struct {
	SimpleGoods            bool           `json:"simple_goods_confirmed"`
	MaterialsComplete      bool           `json:"materials_list_complete"`
	Sets                   ProvisionClaim `json:"sets,omitempty"`
	AccessoriesPacking     ProvisionClaim `json:"accessories_packing,omitempty"`
	Fungibles              ProvisionClaim `json:"fungibles,omitempty"`
	IntermediateMaterials  ProvisionClaim `json:"intermediate_materials,omitempty"`
	CostAveraging          ProvisionClaim `json:"cost_averaging,omitempty"`
	DeMinimis              ProvisionClaim `json:"de_minimis,omitempty"`
	OutsidePartyOperations ProvisionClaim `json:"outside_party_operations,omitempty"`
	SpecialValuation       ProvisionClaim `json:"special_valuation,omitempty"`
}

type provisionRef struct {
	name string
	get  func(Profile) ProvisionClaim
}

var provisions = []provisionRef{
	{"sets", func(p Profile) ProvisionClaim { return p.Sets }},
	{"accessories_packing", func(p Profile) ProvisionClaim { return p.AccessoriesPacking }},
	{"fungibles", func(p Profile) ProvisionClaim { return p.Fungibles }},
	{"intermediate_materials", func(p Profile) ProvisionClaim { return p.IntermediateMaterials }},
	{"cost_averaging", func(p Profile) ProvisionClaim { return p.CostAveraging }},
	{"de_minimis", func(p Profile) ProvisionClaim { return p.DeMinimis }},
	{"outside_party_operations", func(p Profile) ProvisionClaim { return p.OutsidePartyOperations }},
	{"special_valuation", func(p Profile) ProvisionClaim { return p.SpecialValuation }},
}

// Input is one stateless check request. Amounts are exact decimal strings in
// one currency and valuation basis; omitted amounts are unknown, never zero.
type Input struct {
	Destination      string     `json:"destination"`
	Regime           string     `json:"regime"`
	GoodHS           string     `json:"good_hs"`
	RVCMethod        string     `json:"rvc_method,omitempty"`
	TransactionValue Amount     `json:"transaction_value"`
	NetCost          Amount     `json:"net_cost"`
	VNM              Amount     `json:"vnm"`
	Materials        []Material `json:"materials,omitempty"`
	Profile          Profile    `json:"profile"`
}

// DecodeInput strictly decodes a JSON request body. Unknown fields, trailing
// data, JSON numbers for amounts and malformed decimals are ErrInput.
func DecodeInput(data []byte) (Input, error) {
	var in Input
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		return Input{}, inputErr("body", "%v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Input{}, inputErr("body", "unexpected trailing data")
	}
	return in, nil
}

// Validate checks structure and bounds. It never coerces.
func (in Input) Validate() error {
	if in.Destination != DestinationUS {
		return inputErr("destination", "must be exactly %q", DestinationUS)
	}
	if in.Regime != RegimeUSMCA {
		return inputErr("regime", "must be exactly %q", RegimeUSMCA)
	}
	if !isDigits(in.GoodHS) || (len(in.GoodHS) != 4 && len(in.GoodHS) != 6 && len(in.GoodHS) != 8 && len(in.GoodHS) != 10) {
		return inputErr("good_hs", "must be 4, 6, 8 or 10 ASCII digits without dots or spaces")
	}
	if in.RVCMethod != "" {
		if _, err := ParseMethod(in.RVCMethod); err != nil {
			return inputErr("rvc_method", "%v", err)
		}
	}
	if len(in.Materials) > MaxMaterials {
		return inputErr("materials", "at most %d materials are accepted", MaxMaterials)
	}
	for i, m := range in.Materials {
		f := fmt.Sprintf("materials[%d]", i)
		if len(m.ID) > MaxMaterialID {
			return inputErr(f+".id", "longer than %d bytes", MaxMaterialID)
		}
		if m.HS != "" && (!isDigits(m.HS) || len(m.HS) > 10 || len(m.HS)%2 != 0) {
			return inputErr(f+".hs", "must be empty (unknown) or 2, 4, 6, 8 or 10 ASCII digits")
		}
		switch m.Originating {
		case "", ClaimOriginating, ClaimNonOriginating, ClaimUnknown:
		default:
			return inputErr(f+".originating", "must be originating, non_originating or unknown")
		}
	}
	for _, p := range provisions {
		switch p.get(in.Profile) {
		case "", ProvisionNotApplicable, ProvisionApplies:
		default:
			return inputErr("profile."+p.name, "must be not_applicable or applies")
		}
	}
	return nil
}
