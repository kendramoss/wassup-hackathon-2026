package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type atigaMaterial struct {
	Name    string   `json:"name,omitempty"`
	HS      string   `json:"hs,omitempty"`
	Value   *float64 `json:"value,omitempty"`
	Country string   `json:"country,omitempty"`
	Status  string   `json:"status,omitempty"`
	RVCPct  *float64 `json:"rvc_pct,omitempty"`
}

type atigaIn struct {
	HS             string          `json:"hs"`
	ProducedIn     string          `json:"produced_in"`
	FOB            *float64        `json:"fob,omitempty"`
	Materials      []atigaMaterial `json:"materials,omitempty"`
	Costs          *rcepCosts      `json:"costs,omitempty"`
	Method         string          `json:"method,omitempty"`
	WhollyObtained bool            `json:"wholly_obtained,omitempty"`
	Operations     []string        `json:"operations,omitempty"`
	Processes      map[string]bool `json:"processes,omitempty"`
	Variant        string          `json:"variant,omitempty"`
}

func atigaSchema() *jsonschema.Schema {
	rvcPct := number("Art. 30.2 partial cumulation: the material's own ASEAN value content, per cent (counts from 20 to under 40, Member State materials only).", 0)
	rvcPct.Maximum = ptr(100.0)
	material := object("One input of the bill of materials.", nil, map[string]*jsonschema.Schema{
		"name":    str("", 0, 200),
		"hs":      pattern("The material's HS code, 2 to 10 digits (the rule needs 2, 4 or 6).", hsPattern, 14),
		"value":   number("CIF at import, or the earliest price paid in the Member State; same currency as fob.", 0),
		"country": country("Where the material originates."),
		"status":  enum("originating only when the supplier's declaration or proof of origin says it is ATIGA originating; unknown counts as non-originating.", "originating", "non_originating", "unknown"),
		"rvc_pct": rvcPct,
	})
	costs := object("Direct method terms (Art. 29.1(a)), same currency as fob.", nil, map[string]*jsonschema.Schema{
		"labour": number("", 0), "overhead": number("", 0), "profit": number("", 0), "other": number("", 0),
	})
	processes := object("Annex 3 conditions written in words; state each that applies (true) or does not (false). Left out means not stated.", nil, map[string]*jsonschema.Schema{
		"textile_process":       boolean("The Attachment 1 textile working or processing was done in a Member State."),
		"cut_and_sewn":          boolean("The good is both cut and sewn in a Member State."),
		"refining":              boolean("The good is produced by refining."),
		"wet_to_dry":            boolean("There is a change from the wet state to the dry state."),
		"recording":             boolean("Recording onto blank or unrecorded media (8523)."),
		"parts_same_subheading": boolean("Made from parts produced solely for goods of the same subheading."),
	})
	return object("", []string{"hs", "produced_in"}, map[string]*jsonschema.Schema{
		"hs":              pattern("The good's HS code, 4 to 10 digits (HS 2022).", `^[0-9]{4}(?:[. ]?[0-9]{2}){0,3}$`, 14),
		"produced_in":     country("ASEAN Member State where the good is produced."),
		"fob":             number("FOB value of the good (needed for RVC and de minimis).", 0),
		"materials":       array("", material, 0, 200),
		"costs":           costs,
		"method":          enum("The RVC method the exporting Member State uses (Art. 29.3); leave out to try both.", "direct", "indirect"),
		"wholly_obtained": boolean("The producer states the good is wholly obtained or produced in the Member State (Art. 27)."),
		"operations":      array("What is done to the non-originating materials: Art. 31 keys (preservation, shipment, packaging) or any other word for real production.", str("", 1, 60), 0, 50),
		"processes":       processes,
		"variant":         str("Branch id from rule.variants, for rows whose rule depends on the product.", 1, 60),
	})
}

// registerATIGA adds check_atiga_preference (POST /v1/atiga/check).
func (t tools) registerATIGA(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name: "check_atiga_preference",
		Description: `Check whether a good made in an ASEAN Member State qualifies as ATIGA originating (preferential origin under the ASEAN Trade in Goods Agreement, Chapter 3 and Annex 3, HS 2022), from its HS code, where it is made and its bill of materials. Deterministic rules, no model call, nothing saved.
Use it when the user asks whether a good can claim the ATIGA (AFTA, Form D) preferential tariff on import into another ASEAN Member State (Brunei, Cambodia, Indonesia, Laos, Malaysia, Myanmar, Philippines, Singapore, Thailand, Vietnam). It is NOT the country-of-origin or marking answer predict_origin gives; never present one as the other.
Input: hs and produced_in; materials with hs, value, country and status (originating only with a supplier's declaration or proof of origin); fob for value content and de minimis; costs and method for the direct method; processes for rules written in words (textile process, cut and sewn).
Returns: outcome yes / no / undetermined (missing facts are undetermined, never no), the Annex 3 rule or the Art. 28.1 general rule, the alternative that decided, RVC, de minimis, the failing materials and plain-English reasons. Not a ruling or a Certificate of Origin. REST: POST /v1/atiga/check (operationId checkAtigaPreference).`,
		InputSchema: atigaSchema(),
		Annotations: readOnly("Check ATIGA preference", false),
	}, t.atiga)
}

func (t tools) atiga(ctx context.Context, hdr http.Header, in atigaIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: "/v1/atiga/check", body: in})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	var b strings.Builder
	word := map[string]string{"yes": "Qualifies", "no": "Does not qualify", "undetermined": "Undetermined"}[str0(out["outcome"], "")]
	fmt.Fprintf(&b, "ATIGA preferential origin: %s", word)
	if a := str0(out["applied"], ""); a != "" {
		fmt.Fprintf(&b, " (%s)", a)
	}
	b.WriteString(".")
	if r := obj(out["rule"]); len(r) > 0 {
		fmt.Fprintf(&b, "\nRule (%v): %v.", r["code"], r["concise"])
	}
	for _, x := range list(out["failing"]) {
		f := obj(x)
		fmt.Fprintf(&b, "\nFails: %s (%s) %s.", str0(f["name"], "material"), str0(f["hs"], "?"), str0(f["reason"], ""))
	}
	for _, x := range list(out["reasons"]) {
		fmt.Fprintf(&b, "\n- %v", x)
	}
	fmt.Fprintf(&b, "\n%s", str0(out["note"], ""))
	return done(b.String(), out)
}
