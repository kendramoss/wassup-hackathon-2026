package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type originIn struct {
	Product     map[string]any   `json:"product,omitempty"`
	GoodHTS     string           `json:"good_hts,omitempty"`
	Country     string           `json:"country,omitempty"`
	Materials   []map[string]any `json:"materials,omitempty"`
	Row         string           `json:"row,omitempty"`
	Answers     map[string]bool  `json:"answers,omitempty"`
	MinimalOnly *bool            `json:"minimal_operations_only,omitempty"`
}

// originTool is what differs between check_eu_origin and check_uk_origin.
type originTool struct {
	name, title, path, op, market, rules, scope, broker string
}

var (
	euOriginTool = originTool{
		name: "check_eu_origin", title: "Check EU non-preferential origin", path: "/v1/rules/eu-origin", op: "checkEUOrigin", market: "EU",
		rules:  "the EU's non-preferential origin rules (Union Customs Code Art. 60; Delegated Regulation (EU) 2015/2446 Art. 31-36 and the list rules of its Annex 22-01)",
		scope:  "Annex 22-01 has list rules for some goods in 37 HS chapters only; every other good returns needs_judgement (Art. 60(2) UCC: last substantial, economically justified processing), which you must not answer yourself. These are common EU rules; each Member State's customs authority applies them and issues Binding Origin Information. Binding Origin Information and EBTI are not read.",
		broker: "Not a ruling: confirm with an EU customs adviser, or apply for Binding Origin Information in the Member State.",
	}
	ukOriginTool = originTool{
		name: "check_uk_origin", title: "Check UK non-preferential origin", path: "/v1/rules/uk-origin", op: "checkUKOrigin", market: "UK",
		rules:  "the UK's non-preferential origin rules (Taxation (Cross-border Trade) Act 2018 s. 17; SI 2020/1433; the product-specific rules in Part Four of HMRC's reference document v1.2)",
		scope:  "Part Four has rules for some goods in 37 HS chapters only; every other good returns needs_judgement (s. 17(3): last substantial, economically justified processing), which you must not answer yourself. HMRC publishes no origin rulings.",
		broker: "Not a ruling: confirm with a UK customs adviser, or ask HMRC for an Advance Origin Ruling.",
	}
)

// registerOrigin adds check_eu_origin or check_uk_origin (D760-D766),
// listed only when the server serves the route.
func (t tools) registerOrigin(s *mcp.Server, o originTool) {
	code := func(desc string) *jsonschema.Schema { return pattern(desc, `^[0-9]{4}(?:[. ]?[0-9]{2}){0,3}$`, 14) }
	share := func(desc string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "number", Description: desc, Minimum: ptr(0.0), Maximum: ptr(1.0)}
	}
	material := object("A material of the good.", []string{"hts", "origin"}, map[string]*jsonschema.Schema{
		"name":         str("", 1, 200),
		"hts":          code("HS heading or subheading of the material, e.g. 8534.00."),
		"origin":       country("Country the material is from."),
		"value_share":  share("Share of the good's ex-works price (0 to 1): for the 10% tolerance, value rules and the residual rule."),
		"weight_share": share("Share of the good's weight (0 to 1): for residual rules by weight and mixtures."),
	})
	add(s, &mcp.Tool{
		Name: o.name,
		Description: fmt.Sprintf(`Check %s for a good, from its HS code and its materials' codes and origins. No model call; nothing is saved.
Use it when the user imports into the %s and asks the good's non-preferential country of origin (tariff and trade measures). Not for preferential (trade-agreement) origin.
Input: either product (as for predict_origin, with hts on the product and its parts), or good_hts, country (where the good is made) and materials. Optional: row (pick a list row when result.needs asks for one, or "none"), answers (yes/no facts keyed as result.needs names them; leave a key out when unknown, false is an answer), minimal_operations_only.
Returns: status decided, undetermined or needs_judgement; origin; the list row applied and each alternative's outcome; plain reasons; result.needs listing exactly what would resolve an undetermined answer (fact keys for answers, row options, codes, shares, or review by a person). %s A rules check only: there is no accuracy figure, because no published origin rulings exist to test it against. Say so, and that it is not a ruling. REST: POST %s (operationId %s).`, o.rules, o.market, o.scope, o.path, o.op),
		InputSchema: &jsonschema.Schema{
			Type: "object", AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
			Properties: map[string]*jsonschema.Schema{
				"product":   t.product(),
				"good_hts":  code("HS heading or subheading of the good, e.g. 8527.13."),
				"country":   country("Country where the good is produced."),
				"materials": array("", material, 0, 50),
				"row":       str("Key of the list row the good is (from result.needs options), or \"none\".", 1, 40),
				"answers": {Type: "object", Description: "Yes/no facts keyed as result.needs names them (for example \"8473x.2\": true).",
					AdditionalProperties: &jsonschema.Schema{Type: "boolean"}},
				"minimal_operations_only": {Type: "boolean", Description: "The last work, where the good was made, was only minimal operations (packing, sorting, simple assembly, labelling)."},
			},
		},
		Annotations: readOnly(o.title, false),
	}, func(ctx context.Context, hdr http.Header, in originIn) (*mcp.CallToolResult, any, error) {
		return t.origin(ctx, hdr, in, o)
	})
}

func (t tools) origin(ctx context.Context, hdr http.Header, in originIn, o originTool) (*mcp.CallToolResult, any, error) {
	body := map[string]any{}
	if in.Product != nil {
		body["product"] = in.Product
	} else {
		body["good_hts"], body["country"], body["materials"] = in.GoodHTS, in.Country, in.Materials
	}
	if in.Row != "" {
		body["row"] = in.Row
	}
	if len(in.Answers) > 0 {
		body["answers"] = in.Answers
	}
	if in.MinimalOnly != nil {
		body["minimal_operations_only"] = *in.MinimalOnly
	}
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: o.path, body: body})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	r := obj(out["result"])
	var b strings.Builder
	fmt.Fprintf(&b, "%s.\n", str0(out["label"], ""))
	switch r["status"] {
	case "decided":
		fmt.Fprintf(&b, "%s non-preferential origin: %s (%s).", o.market, str0(r["origin"], ""), str0(r["basis"], ""))
	case "needs_judgement":
		b.WriteString("The rules do not decide this good by code: its origin needs a substantial-transformation judgement by a person.")
	default:
		b.WriteString("Not decided from what was given.")
	}
	if reason := str0(r["reason"], ""); reason != "" {
		fmt.Fprintf(&b, "\nWhy: %s.", strings.TrimSuffix(reason, "."))
	}
	if as := list(r["assumes"]); len(as) > 0 {
		b.WriteString("\nAssumes: ")
		for i, a := range as {
			if i > 0 {
				b.WriteString("; ")
			}
			fmt.Fprint(&b, a)
		}
		b.WriteString(".")
	}
	var ask, add, pick, review []string
	for _, x := range list(r["needs"]) {
		n := obj(x)
		label := str0(n["label"], str0(n["key"], ""))
		if ev := str0(n["evidence"], ""); ev != "" {
			label += " [settled by: " + ev + "]"
		}
		switch n["kind"] {
		case "fact":
			ask = append(ask, fmt.Sprintf("%s (send answers[%q] true or false)", strings.TrimSuffix(label, "?"), str0(n["key"], "")))
		case "row":
			var opts []string
			for _, y := range list(n["options"]) {
				op := obj(y)
				opts = append(opts, fmt.Sprintf("%s = %s", str0(op["key"], ""), str0(op["description"], "")))
			}
			pick = append(pick, label+" Send row: "+strings.Join(opts, "; "))
		case "review":
			review = append(review, label)
		default:
			add = append(add, label)
		}
	}
	if len(pick) > 0 {
		fmt.Fprintf(&b, "\nPick a row: %s.", strings.Join(pick, " "))
	}
	if len(ask) > 0 {
		fmt.Fprintf(&b, "\nWould resolve it: %s.", strings.Join(ask, "; "))
	}
	if len(add) > 0 {
		fmt.Fprintf(&b, "\nAdd: %s.", strings.Join(add, "; "))
	}
	if len(review) > 0 {
		fmt.Fprintf(&b, "\nNeeds human review (no field can answer it): %s.", strings.Join(review, "; "))
	}
	fmt.Fprintf(&b, "\n%s", o.broker)
	return done(b.String(), out)
}
