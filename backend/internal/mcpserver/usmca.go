package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	usmcaAmountPattern = `^(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`
	usmcaHSPattern     = `^[0-9]{2}(?:\.?[0-9]{2}){0,4}$`
)

// usmcaSchema is the input of check_usmca_origin_rule (POST /v1/usmca/check).
// Amounts are JSON strings, never numbers; an omitted amount is unknown, not
// zero.
func usmcaSchema() *jsonschema.Schema {
	amount := func(desc string) *jsonschema.Schema { return pattern(desc, usmcaAmountPattern, 64) }
	claim := func() *jsonschema.Schema { return enum("", "not_applicable", "applies") }
	material := object("One non-originating-or-unknown input material.", nil, map[string]*jsonschema.Schema{
		"id":          str("Your label for the material.", 0, 64),
		"hs":          pattern("The material's HS code, 2 to 10 digits, dots allowed. Omit when unknown.", usmcaHSPattern, 14),
		"originating": enum("Your claim about the material; not verified. Omitted means unknown.", "originating", "non_originating", "unknown"),
	})
	profile := object("Confirmations that the good is a simple good. Each unconfirmed provision keeps the rule from being applied; omitted never means not_applicable.", nil, map[string]*jsonschema.Schema{
		"simple_goods_confirmed":   boolean("The good is a simple good with none of the unsupported provisions below."),
		"materials_list_complete":  boolean("The materials list names every non-originating input."),
		"sets":                     claim(),
		"accessories_packing":      claim(),
		"fungibles":                claim(),
		"intermediate_materials":   claim(),
		"cost_averaging":           claim(),
		"de_minimis":               claim(),
		"outside_party_operations": claim(),
		"special_valuation":        claim(),
		"parts_route":              claim(),
	})
	return object("", []string{"destination", "regime", "good_hs"}, map[string]*jsonschema.Schema{
		"destination":       enum("Always US.", "US"),
		"regime":            enum("Always usmca.", "usmca"),
		"good_hs":           pattern("The good's HS code, 4 to 10 digits, dots allowed (8202.31.00 is read as 82023100).", `^[0-9]{4}(?:\.?[0-9]{2}){0,3}$`, 14),
		"rvc_method":        enum("The regional value content method you elect. Required when the rule's value-content alternative is used; never chosen for you.", "transaction_value", "net_cost"),
		"transaction_value": amount("Transaction value of the good as a decimal string. Omit when unknown; omitted is not zero."),
		"net_cost":          amount("Net cost of the good as a decimal string. Omit when unknown; omitted is not zero."),
		"vnm":               amount("Value of non-originating materials as a decimal string, including materials of undetermined origin. Omit when unknown; omitted is not zero, and 0 means known zero."),
		"materials":         array("", material, 0, 200),
		"profile":           profile,
	})
}

const usmcaDescription = `Check one selected USMCA product-specific origin rule for a good imported into the United States (HTSUS General Note 11(o), pilot goods 8202.31, 8207.13 and 9503.00 only), from its HS code, regional value content figures and bill of materials. Deterministic rules, no model call, nothing saved.
Reports the selected-rule status only: met, not_met, needs_information or not_supported. It is NOT a certification of origin, NOT a determination that the good is originating, NOT a tariff rate, duty saving or duty-free entitlement, and NOT for deciding a good's legal origin status. A not_met result concerns the rule as modeled, not a legal finding. It is separate from predict_origin (non-preferential origin), US marking, Canada marking and check_rcep_preference.
Input: destination US, regime usmca, good_hs; the regional value content method you elect (rvc_method) with exact decimal strings for transaction_value or net_cost and vnm (never JSON numbers); materials with hs and your originating claim; profile confirmations. Unknown versus zero: an omitted amount, HS code or claim is unknown and gives needs_information when a decision depends on it; "0" is a known zero. Unconfirmed special provisions (parts route, de minimis, sets, packing, fungibles, and others) are not assumed away.
Returns: status, the alternatives and conditions evaluated, the separate exact RVC arithmetic (a zero denominator is undefined, a negative RVC is not clamped), missing facts, unsupported reasons, the source snapshot and the limits. Originating claims and figures are yours and are not verified. REST: POST /v1/usmca/check (operationId checkUsmcaOriginRule).`

// registerUSMCA adds check_usmca_origin_rule (POST /v1/usmca/check).
func (t tools) registerUSMCA(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name:        "check_usmca_origin_rule",
		Description: usmcaDescription,
		InputSchema: usmcaSchema(),
		Annotations: readOnly("Check USMCA origin rule", false),
	}, t.usmca)
}

func (t tools) usmca(ctx context.Context, hdr http.Header, in map[string]any) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: "/v1/usmca/check", body: in})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	var b strings.Builder
	fmt.Fprintf(&b, "USMCA selected-rule check for %v: %s", out["good_hs"], str0(out["status"], "unknown"))
	b.WriteString(". This is the selected rule only, not a certification of origin, tariff rate or duty entitlement.")
	for _, x := range list(out["missing_facts"]) {
		f := obj(x)
		fmt.Fprintf(&b, "\nMissing: %s", str0(f["message"], str0(f["field"], "a fact")))
	}
	for _, x := range list(out["unsupported_reasons"]) {
		fmt.Fprintf(&b, "\nNot supported: %v", x)
	}
	for _, x := range list(out["limits"]) {
		fmt.Fprintf(&b, "\n- %v", x)
	}
	return done(b.String(), out)
}
