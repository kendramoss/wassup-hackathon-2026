package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type rcepMaterial struct {
	Name    string   `json:"name,omitempty"`
	HS      string   `json:"hs,omitempty"`
	Value   *float64 `json:"value,omitempty"`
	Weight  *float64 `json:"weight,omitempty"`
	Country string   `json:"country,omitempty"`
	Status  string   `json:"status,omitempty"`
}

type rcepCosts struct {
	Labour   float64 `json:"labour,omitempty"`
	Overhead float64 `json:"overhead,omitempty"`
	Profit   float64 `json:"profit,omitempty"`
	Other    float64 `json:"other,omitempty"`
}

type rcepIn struct {
	HS                             string         `json:"hs"`
	ProducedIn                     string         `json:"produced_in"`
	FOB                            *float64       `json:"fob,omitempty"`
	Weight                         *float64       `json:"weight,omitempty"`
	Materials                      []rcepMaterial `json:"materials,omitempty"`
	Costs                          *rcepCosts     `json:"costs,omitempty"`
	WhollyObtained                 bool           `json:"wholly_obtained,omitempty"`
	Operations                     []string       `json:"operations,omitempty"`
	ChemicalReaction               *bool          `json:"chemical_reaction,omitempty"`
	Variant                        string         `json:"variant,omitempty"`
	ActiveIngredientOriginatingPct *float64       `json:"active_ingredient_originating_pct,omitempty"`
}

func number(desc string, lo float64) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "number", Description: desc, Minimum: ptr(lo)}
}

func boolean(desc string) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "boolean", Description: desc}
}

const hsPattern = `^[0-9]{2}(?:[. ]?[0-9]{2}){0,4}$`

func rcepSchema() *jsonschema.Schema {
	material := object("One input of the bill of materials.", nil, map[string]*jsonschema.Schema{
		"name":    str("", 0, 200),
		"hs":      pattern("The material's HS code, 2 to 10 digits (the rule needs 2, 4 or 6).", hsPattern, 14),
		"value":   number("CIF at import, or the earliest price paid in the Party; same currency as fob.", 0),
		"weight":  number("Same unit as the good's weight.", 0),
		"country": country("Where the material originates."),
		"status":  enum("originating only when the supplier's declaration or proof of origin says it is RCEP originating; unknown counts as non-originating.", "originating", "non_originating", "unknown"),
	})
	costs := object("Build-up formula terms (Art. 3.5.1(b)), same currency as fob.", nil, map[string]*jsonschema.Schema{
		"labour": number("", 0), "overhead": number("", 0), "profit": number("", 0), "other": number("", 0),
	})
	pctSchema := number("Heading 38.08 only: per cent by weight of the active ingredients that is originating.", 0)
	pctSchema.Maximum = ptr(100.0)
	return object("", []string{"hs", "produced_in"}, map[string]*jsonschema.Schema{
		"hs":                                pattern("The good's HS code, 4 to 10 digits (HS 2022).", `^[0-9]{4}(?:[. ]?[0-9]{2}){0,3}$`, 14),
		"produced_in":                       country("Country where the good is produced."),
		"fob":                               number("FOB value of the good (needed for RVC and the value de minimis).", 0),
		"weight":                            number("Weight of the good (chapters 50-63 de minimis).", 0),
		"materials":                         array("", material, 0, 200),
		"costs":                             costs,
		"wholly_obtained":                   boolean("The producer states the good is wholly obtained in the Party (Art. 3.3)."),
		"operations":                        array("What is done to the non-originating materials: Art. 3.6 keys (preserving, packaging, simple_processes, marking, dilution, disassembly, slaughtering, simple_painting_polishing, simple_peeling, simple_mixing) or any other word for real production.", str("", 1, 60), 0, 50),
		"chemical_reaction":                 boolean("A chemical reaction took place in a Party (Annex 3A CR)."),
		"variant":                           str("Branch id from rule.variants, for the eight rows whose rule depends on the product.", 1, 40),
		"active_ingredient_originating_pct": pctSchema,
	})
}

// registerRCEP adds check_rcep_preference (POST /v1/rcep/check).
func (t tools) registerRCEP(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name: "check_rcep_preference",
		Description: `Check whether a good made in an RCEP Party qualifies as RCEP originating (preferential origin under the RCEP Agreement, Chapter 3 and Annex 3A, HS 2022), from its HS code, where it is made and its bill of materials. Deterministic rules, no model call, nothing saved.
Use it when the user asks whether a good can claim the RCEP preferential tariff on import into another RCEP Party (Australia, Brunei, Cambodia, China, Indonesia, Japan, South Korea, Laos, Malaysia, Myanmar, New Zealand, Philippines, Singapore, Thailand, Vietnam). It is NOT the country-of-origin or marking answer predict_origin gives; never present one as the other.
Input: hs and produced_in; materials with hs, value, country and status (originating only with a supplier's declaration or proof of origin); fob for value content and de minimis; costs for the build-up formula.
Returns: outcome yes / no / undetermined (missing facts are undetermined, never no), the Annex 3A rule and the alternative that decided, RVC both ways, de minimis, the failing materials and plain-English reasons. Not a ruling or a certificate of origin. REST: POST /v1/rcep/check (operationId checkRcepPreference).`,
		InputSchema: rcepSchema(),
		Annotations: readOnly("Check RCEP preference", false),
	}, t.rcep)
}

func (t tools) rcep(ctx context.Context, hdr http.Header, in rcepIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: "/v1/rcep/check", body: in})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	var b strings.Builder
	word := map[string]string{"yes": "Qualifies", "no": "Does not qualify", "undetermined": "Undetermined"}[str0(out["outcome"], "")]
	fmt.Fprintf(&b, "RCEP preferential origin: %s", word)
	if a := str0(out["applied"], ""); a != "" {
		fmt.Fprintf(&b, " (%s)", a)
	}
	b.WriteString(".")
	if r := obj(out["rule"]); len(r) > 0 {
		fmt.Fprintf(&b, "\nAnnex 3A rule for %v: %v.", r["code"], r["text"])
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
