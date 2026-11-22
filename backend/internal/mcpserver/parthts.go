package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type partsHTSIn struct {
	Product map[string]any `json:"product"`
}

// registerPartsHTS adds suggest_part_hts (POST /v1/products/parts-hts-suggest,
// D380), listed only when the server serves that route.
func (t tools) registerPartsHTS(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name: "suggest_part_hts",
		Description: `Suggest up to 3 U.S. HTS codes for each part of a product (at most 20 parts), and for the product itself when it has no hts, so the 19 CFR 102.20 rule check can run. Nothing is saved.
Use it when predict_origin or get_case shows a rule check waiting for part codes, or the user asks for the parts' tariff codes.
Input: product (name, description and components; send the parts still without a code). Each candidate says its source: "rulings" (an earlier CBP ruling states that code for a similar part, listed in supporting_rulings) or "model" (the model's own reading of the HTSUS, no supporting ruling).
Unverified suggestions: show them to the user and put a code on a part (components[].hts) only when the user accepts it, with the part's other suggested codes in components[].hts_alternatives; then call predict_origin with the product and its codes, and the case's rule_check reads them and says whether its answer is the same under every suggested code (rule_check.robustness). Not a classification: confirm with a licensed customs broker. REST: POST /v1/products/parts-hts-suggest (operationId suggestPartsHts).`,
		InputSchema: &jsonschema.Schema{
			Type: "object", AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}, Required: []string{"product"},
			Properties: map[string]*jsonschema.Schema{"product": t.product()},
		},
		Annotations: readOnly("Suggest HTS codes for parts", true),
	}, t.partsHTS)
}

func (t tools) partsHTS(ctx context.Context, hdr http.Header, in partsHTSIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: "/v1/products/parts-hts-suggest", body: map[string]any{"product": in.Product}})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	var b strings.Builder
	b.WriteString("Suggested HTS codes (unverified; confirm with a licensed customs broker):")
	if g := obj(out["good"]); len(g) > 0 {
		fmt.Fprintf(&b, "\nThe product: %s", candidateList(list(g["candidates"])))
	}
	for _, x := range list(out["parts"]) {
		p := obj(x)
		fmt.Fprintf(&b, "\n%s: %s", str0(p["name"], "part"), candidateList(list(p["candidates"])))
	}
	if out["reasons_unavailable"] == true {
		b.WriteString("\nThe model was unavailable: only codes from earlier rulings are listed, without reasons.")
	}
	b.WriteString("\nPut a code on a part (components[].hts) only when the user accepts it, and the part's other suggested codes in components[].hts_alternatives.")
	return done(b.String(), out)
}

// candidateList is "8501.31 (rulings: N123456), 8501.40 (model)" or "none".
func candidateList(cs []any) string {
	if len(cs) == 0 {
		return "none"
	}
	var parts []string
	for _, x := range cs {
		c := obj(x)
		s := fmt.Sprintf("%v (%s", c["hts"], str0(c["source"], "model"))
		if ids := list(c["cited"]); len(ids) > 0 {
			s += ": " + joinAny(ids)
		}
		parts = append(parts, s+")")
	}
	return strings.Join(parts, ", ")
}
