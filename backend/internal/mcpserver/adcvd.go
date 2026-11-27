package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type adcvdPart struct {
	Name   string `json:"name,omitempty"`
	HTS    string `json:"hts,omitempty"`
	Origin string `json:"origin,omitempty"`
}

type adcvdIn struct {
	Parts []adcvdPart `json:"parts"`
}

func adcvdSchema() *jsonschema.Schema {
	part := object("One part to screen.", nil, map[string]*jsonschema.Schema{
		"name":   str("", 0, 200),
		"hts":    pattern("The part's HTS heading or subheading, 4 to 10 digits.", `^[0-9]{4}(?:[. ]?[0-9]{2}){0,3}$`, 14),
		"origin": country("Where the part is made."),
	})
	return object("", []string{"parts"}, map[string]*jsonschema.Schema{
		"parts": array("The parts to screen, at most 200.", part, 1, 200),
	})
}

// registerADCVD adds check_adcvd_exposure (POST /v1/adcvd/check).
func (t tools) registerADCVD(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name: "check_adcvd_exposure",
		Description: `Screen parts for possible exposure to a U.S. antidumping (AD) or countervailing duty (CVD) order, from each part's HTS code and country of origin. Nothing saved. Without the scope reader the screen is deterministic with no model call. When the server runs the scope-first check (D959, ADCVD_SCOPE_READER) a model screens the orders of the part's country that its name could fit and reads each remaining order's whole written scope against the name; then the part name and origin matter most and the HTS code is only a hint.
Use it when the user asks whether a part or product from a country could be under an AD/CVD order. It never changes the origin answer or any duty figure; do not present a result as a duty calculation or as a Commerce scope ruling.
Input: parts, each with origin, a name (the description the scope is read against) and an optional hts (4 to 10 digits; required without the scope reader).
Returns, per part: possible_exposure (with the matching orders: case numbers, product, country, order date, the notice they came from and, from the scope-first check, the scope sentence relied on; a reading of unclear means a person must check), person_must_check (the reader could not settle an order and leaned away: a person must read the scope), no_match, or not_checked (with what was missing). Always repeat the notice: HTS numbers in a scope are for convenience, and the written scope of the order governs. It cannot see goods finished in a third country (circumvention) or an order newer than the snapshot date it reports. REST: POST /v1/adcvd/check (operationId checkAdcvdExposure).`,
		InputSchema: adcvdSchema(),
		Annotations: readOnly("Screen parts for AD/CVD orders", false),
	}, t.adcvd)
}

func (t tools) adcvd(ctx context.Context, hdr http.Header, in adcvdIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: "/v1/adcvd/check", body: in})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	var b strings.Builder
	fmt.Fprintf(&b, "AD/CVD screen against orders to %v.", out["snapshot"])
	for _, x := range list(out["parts"]) {
		p := obj(x)
		name := str0(p["name"], str0(p["hts"], "part"))
		switch p["status"] {
		case "possible_exposure":
			fmt.Fprintf(&b, "\n- %s: Possible exposure.", name)
			for _, y := range list(p["matches"]) {
				m := obj(y)
				cs := []string{}
				for _, c := range list(m["cases"]) {
					cs = append(cs, str0(c, ""))
				}
				fmt.Fprintf(&b, " %s (%s, %s).", str0(m["product"], ""), strings.Join(cs, " and "), str0(m["country"], ""))
				if m["reading"] == "unclear" {
					b.WriteString(" The reader could not settle it: a person must check.")
				}
			}
		case "person_must_check":
			fmt.Fprintf(&b, "\n- %s: a person must check the written scope. %s", name, str0(p["reason"], ""))
		case "no_match":
			fmt.Fprintf(&b, "\n- %s: no match.", name)
		default:
			fmt.Fprintf(&b, "\n- %s: not checked. %s", name, str0(p["reason"], ""))
		}
	}
	fmt.Fprintf(&b, "\n%s", str0(out["notice"], ""))
	return done(b.String(), out)
}
