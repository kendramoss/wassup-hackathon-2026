package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type canadaIn struct {
	Product        map[string]any   `json:"product,omitempty"`
	GoodHTS        string           `json:"good_hts,omitempty"`
	Country        string           `json:"country,omitempty"`
	Materials      []map[string]any `json:"materials,omitempty"`
	SimpleAssembly *bool            `json:"simple_assembly,omitempty"`
	GRI2a          *bool            `json:"gri_2a,omitempty"`
}

// registerCanada adds check_canada_marking (POST /v1/rules/canada-marking,
// D720), listed only when the server serves that route.
func (t tools) registerCanada(s *mcp.Server) {
	code := func(desc string) *jsonschema.Schema { return pattern(desc, `^[0-9]{4}(?:[. ]?[0-9]{2}){0,3}$`, 14) }
	share := func(desc string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "number", Description: desc, Minimum: ptr(0.0), Maximum: ptr(1.0)}
	}
	material := object("A material of the good.", []string{"hts", "origin"}, map[string]*jsonschema.Schema{
		"name":         str("", 1, 200),
		"hts":          code("HS heading or subheading of the material, e.g. 5205.11."),
		"origin":       country("Country the material is from."),
		"value_share":  share("Share of the good's value (0 to 1), for de minimis."),
		"weight_share": share("Share of the good's weight (0 to 1), for de minimis in chapters 50 to 63."),
	})
	add(s, &mcp.Tool{
		Name: "check_canada_marking",
		Description: `Check Canada's marking-origin rules (SOR/94-23, goods imported into Canada from the United States or Mexico) for a good, from its HS code and its materials' codes and origins. No model call; nothing is saved.
Use it when the user ships to Canada and asks what country of origin the goods must be marked with.
Input: either product (as for predict_origin, with hts on the product and its parts), or good_hts, country (where the good is made) and materials. Optional simple_assembly and gri_2a settle rules that turn on them.
Returns: status (decided, undetermined or out_of_scope), origin, the Schedule III row applied, which materials do not shift, and plain reasons. A rules check only: there is no accuracy figure, because Canada publishes too few origin rulings to test it. Say so, and that it is not a ruling. REST: POST /v1/rules/canada-marking (operationId checkCanadaMarking).`,
		InputSchema: &jsonschema.Schema{
			Type: "object", AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
			Properties: map[string]*jsonschema.Schema{
				"product":         t.product(),
				"good_hts":        code("HS heading or subheading of the good, e.g. 6301.30."),
				"country":         country("Country where the good is produced."),
				"materials":       array("", material, 0, 50),
				"simple_assembly": {Type: "boolean", Description: "The work is a simple assembly (five or fewer foreign parts, minor processing only)."},
				"gri_2a":          {Type: "boolean", Description: "The change comes from collecting parts classified as the finished good under GRI 2(a)."},
			},
		},
		Annotations: readOnly("Check Canada marking rules", false),
	}, t.canada)
}

func (t tools) canada(ctx context.Context, hdr http.Header, in canadaIn) (*mcp.CallToolResult, any, error) {
	body := map[string]any{}
	if in.Product != nil {
		body["product"] = in.Product
	} else {
		body["good_hts"], body["country"], body["materials"] = in.GoodHTS, in.Country, in.Materials
	}
	if in.SimpleAssembly != nil {
		body["simple_assembly"] = *in.SimpleAssembly
	}
	if in.GRI2a != nil {
		body["gri_2a"] = *in.GRI2a
	}
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: "/v1/rules/canada-marking", body: body})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	r := obj(out["result"])
	var b strings.Builder
	fmt.Fprintf(&b, "%s.\n", str0(out["label"], ""))
	switch r["status"] {
	case "decided":
		fmt.Fprintf(&b, "Canada marking origin: %s (%s).", str0(r["origin"], ""), str0(r["basis"], ""))
	case "out_of_scope":
		b.WriteString("Not covered by the Canadian rule table.")
	default:
		b.WriteString("Not decided from the codes given.")
	}
	if reason := str0(r["reason"], ""); reason != "" {
		fmt.Fprintf(&b, "\nWhy: %s.", reason)
	}
	for _, x := range list(r["materials"]) {
		m := obj(x)
		if m["status"] == "no_shift" && m["foreign"] == true {
			fmt.Fprintf(&b, "\nDoes not shift: %s (%s, %s).", str0(m["name"], "a material"), str0(m["hts"], ""), str0(m["origin"], "origin unknown"))
		}
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
	b.WriteString("\nNot a ruling: confirm with a licensed customs broker in Canada.")
	return done(b.String(), out)
}
