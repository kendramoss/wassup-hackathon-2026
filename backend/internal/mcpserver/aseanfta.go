package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// aseanFTA names one of the two ASEAN plus-one agreements this server reads
// (D949); they share one input and one answer shape.
type aseanFTA struct {
	name, path, label, edition, parties string
	description                         string
	processes                           [][2]string
}

var akftaTool = aseanFTA{
	name: "check_akfta_preference", path: "/v1/akfta/check", label: "AKFTA", edition: "HS 2022",
	parties: "Brunei, Cambodia, Indonesia, Laos, Malaysia, Myanmar, the Philippines, Singapore, Thailand, Vietnam or South Korea",
	description: `Check whether a good made in an ASEAN Member State or Korea qualifies as AKFTA originating (preferential origin under the ASEAN-Korea Free Trade Agreement, Annex 3 and the Appendix 2 product-specific rules, HS 2022), from its HS code, where it is made and its bill of materials. Deterministic rules, no model call, nothing saved.
Use it when the user asks whether a good can claim the AKFTA (Form AK) preferential tariff between ASEAN and Korea. It is NOT the country-of-origin or marking answer predict_origin gives; never present one as the other.
Input: hs and produced_in; materials with hs, value, country, status (originating only with a supplier's declaration or proof of origin) and wholly_obtained where a rule asks for it; fob for value content and de minimis; weight (good and materials) for textiles of chapters 50 to 63; processes for rules written in words (cut and sewn, printing or dyeing, glazed, milk limit).
Returns: outcome yes / no / undetermined (missing facts are undetermined, never no), the Appendix 2 rule or the general rule (RVC 40 or CTH), the alternative that decided, RVC, de minimis, the failing materials and plain-English reasons. Not a ruling or a Certificate of Origin. REST: POST /v1/akfta/check (operationId checkAkftaPreference).`,
	processes: [][2]string{
		{"cut_and_sewn", "The good is both cut and sewn in the territory of a Party."},
		{"printing_dyeing", "Printing or dyeing is accompanied by at least two preparatory or finishing operations."},
		{"milk_limit", "The good has no milk (or material from milk) from a non-Party above the stated share by weight."},
		{"glazed", "The good is glazed from goods that are not glazed."},
		{"mco", "The good is a multi-component integrated circuit made from goods that are not."},
	},
}

var acftaTool = aseanFTA{
	name: "check_acfta_preference", path: "/v1/acfta/check", label: "ACFTA", edition: "HS 2017",
	parties: "Brunei, Cambodia, Indonesia, Laos, Malaysia, Myanmar, the Philippines, Singapore, Thailand, Vietnam or China",
	description: `Check whether a good made in an ASEAN Member State or China qualifies as ACFTA originating (preferential origin under the ASEAN-China Free Trade Agreement as upgraded in 2015, Annex 1 and Attachment B product-specific rules, HS 2017), from its HS code, where it is made and its bill of materials. Deterministic rules, no model call, nothing saved.
Use it when the user asks whether a good can claim the ACFTA (Form E) preferential tariff between ASEAN and China. The table is HS 2017: a code from a later edition may not match. It is NOT the country-of-origin or marking answer predict_origin gives; never present one as the other.
Input: hs and produced_in; materials with hs, value, country, status (originating only with a supplier's declaration or proof of origin) and wholly_obtained where a rule asks for it; fob for value content and de minimis (build-down method only); weight or value for textiles of chapters 50 to 63; processes for Process Rules 1 to 3.
Returns: outcome yes / no / undetermined (missing facts are undetermined, never no), the Attachment B rule or the Art. 4(1) general rule, the alternative that decided, RVC, de minimis, the failing materials and plain-English reasons. Not a ruling or a Certificate of Origin. REST: POST /v1/acfta/check (operationId checkAcftaPreference).`,
	processes: [][2]string{
		{"process_rule_1", "Process Rule 1: fibre-making, spinning, twisting, texturising or braiding."},
		{"process_rule_2", "Process Rule 2: non-woven, weaving or knitting, dyeing or printing and finishing, coating or lamination."},
		{"process_rule_3", "Process Rule 3: cutting and assembly into a complete article (apparel, tents)."},
		{"process_rule", "A Process Rule of Attachment B without saying which of 1 to 3."},
		{"animals_raised", "The goods come from animals raised in either Party."},
		{"fats_wo", "The goods are made from fats or oil wholly obtained in either Party."},
	},
}

type aseanFTAMaterial struct {
	Name           string   `json:"name,omitempty"`
	HS             string   `json:"hs,omitempty"`
	Value          *float64 `json:"value,omitempty"`
	Country        string   `json:"country,omitempty"`
	Status         string   `json:"status,omitempty"`
	WhollyObtained *bool    `json:"wholly_obtained,omitempty"`
	Weight         *float64 `json:"weight,omitempty"`
}

type aseanFTAIn struct {
	HS             string             `json:"hs"`
	ProducedIn     string             `json:"produced_in"`
	FOB            *float64           `json:"fob,omitempty"`
	Weight         *float64           `json:"weight,omitempty"`
	Materials      []aseanFTAMaterial `json:"materials,omitempty"`
	Costs          *rcepCosts         `json:"costs,omitempty"`
	Method         string             `json:"method,omitempty"`
	WhollyObtained bool               `json:"wholly_obtained,omitempty"`
	Operations     []string           `json:"operations,omitempty"`
	Processes      map[string]bool    `json:"processes,omitempty"`
	Variant        string             `json:"variant,omitempty"`
}

func (a aseanFTA) schema() *jsonschema.Schema {
	material := object("One input of the bill of materials.", nil, map[string]*jsonschema.Schema{
		"name":            str("", 0, 200),
		"hs":              pattern("The material's HS code, 2 to 10 digits (the rule needs 2, 4 or 6).", hsPattern, 14),
		"value":           number("CIF at import, or the earliest price paid; same currency as fob.", 0),
		"country":         country("Where the material originates."),
		"status":          enum("originating only when the supplier's declaration or proof of origin says it is "+a.label+" originating; unknown counts as non-originating.", "originating", "non_originating", "unknown"),
		"wholly_obtained": boolean("The material was wholly obtained or produced in a Party; some rules ask for it."),
		"weight":          number("For the textile de minimis (chapters 50 to 63); same unit as the good's weight.", 0),
	})
	props := map[string]*jsonschema.Schema{}
	for _, p := range a.processes {
		props[p[0]] = boolean(p[1])
	}
	processes := object("Rule conditions written in words; state each that applies (true) or does not (false). Left out means not stated.", nil, props)
	method := "The RVC method to use; leave out to try every one the agreement gives."
	if a.label == "ACFTA" {
		method = "Only the build-down formula exists for ACFTA: leave out or give indirect."
	}
	return object("", []string{"hs", "produced_in"}, map[string]*jsonschema.Schema{
		"hs":          pattern("The good's HS code, 4 to 10 digits ("+a.edition+").", `^[0-9]{4}(?:[. ]?[0-9]{2}){0,3}$`, 14),
		"produced_in": country("Where the good is produced: " + a.parties + "."),
		"fob":         number("FOB value of the good (needed for RVC and de minimis).", 0),
		"weight":      number("Weight of the good, for the textile de minimis (chapters 50 to 63).", 0),
		"materials":   array("", material, 0, 200),
		"costs": object("Direct method terms, same currency as fob.", nil, map[string]*jsonschema.Schema{
			"labour": number("", 0), "overhead": number("", 0), "profit": number("", 0), "other": number("", 0),
		}),
		"method":          enum(method, "direct", "indirect"),
		"wholly_obtained": boolean("The producer states the good is wholly obtained or produced in a Party."),
		"operations":      array("What is done to the non-originating materials: minimal-operation keys or any other word for real production.", str("", 1, 60), 0, 50),
		"processes":       processes,
		"variant":         str("Branch id from rule.variants, for rows whose rule depends on the product.", 1, 60),
	})
}

// registerASEANFTA adds check_akfta_preference or check_acfta_preference.
func (t tools) registerASEANFTA(s *mcp.Server, a aseanFTA) {
	add(s, &mcp.Tool{
		Name:        a.name,
		Description: a.description,
		InputSchema: a.schema(),
		Annotations: readOnly("Check "+a.label+" preference", false),
	}, func(ctx context.Context, hdr http.Header, in aseanFTAIn) (*mcp.CallToolResult, any, error) {
		return t.aseanFTA(ctx, hdr, a, in)
	})
}

func (t tools) aseanFTA(ctx context.Context, hdr http.Header, a aseanFTA, in aseanFTAIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: a.path, body: in})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	var b strings.Builder
	word := map[string]string{"yes": "Qualifies", "no": "Does not qualify", "undetermined": "Undetermined"}[str0(out["outcome"], "")]
	fmt.Fprintf(&b, "%s preferential origin: %s", a.label, word)
	if x := str0(out["applied"], ""); x != "" {
		fmt.Fprintf(&b, " (%s)", x)
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
