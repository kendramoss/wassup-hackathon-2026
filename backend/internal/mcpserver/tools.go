package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Disclaimer goes with every tool result (domain.Disclaimer, repeated so the
// package stays free of the API's internals).
const Disclaimer = "Not a ruling. Confirm with a licensed customs broker or a CBP binding ruling."

type tools struct {
	api      http.Handler
	families []string
}

// handler is what each tool implements: build one API call from the input,
// then summarise the answer. hdr is the MCP request's header (mapped by guard).
type handler[In any] func(ctx context.Context, hdr http.Header, in In) (*mcp.CallToolResult, any, error)

// add registers one tool. Adding a tool is one call here plus its handler,
// e.g. extract_product_from_media once POST /v1/products/extract-media exists.
func add[In any](s *mcp.Server, tool *mcp.Tool, h handler[In]) {
	mcp.AddTool(s, tool, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		var hdr http.Header
		if req != nil && req.Extra != nil {
			hdr = req.Extra.Header
		}
		return h(ctx, hdr, in)
	})
}

// Annotations. Lookups are read-only; case tools write to the caller's own
// case only (never delete); tools that may fetch from CBP or USITC are
// open-world.
func readOnly(title string, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, ReadOnlyHint: true, IdempotentHint: true, DestructiveHint: ptr(false), OpenWorldHint: ptr(openWorld)}
}

func writes(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{Title: title, DestructiveHint: ptr(false), OpenWorldHint: ptr(false)}
}

// Tool names map to the OpenAPI operationIds (docs/api/openapi.yaml).
func (t tools) register(s *mcp.Server, search bool) {
	add(s, &mcp.Tool{
		Name: "predict_origin",
		Description: `Predict the country of origin U.S. Customs (CBP) would likely assign to a product (substantial-transformation test), citing earlier CBP rulings. Creates a case and returns its case_id.
Use it when the user asks "what origin would CBP give this product" or before searching production moves.
Input: facts (plain-language description of parts, where each is made and where it is assembled) and/or a structured product (extract_product drafts one); family helps plan search. 
Returns: predicted origin, confidence (0-1; calibrated_confidence is the calibrated probability when one applies, else only the model's stated value), decisive step, cited CBP rulings, triage route (auto, needs_review, needs_info) and the case_id. Without an API key it also returns a case_access_key to pass to later calls on the case.
Not a ruling: confirm with a licensed customs broker. REST: POST /v1/cases/predict (operationId predictCase).`,
		InputSchema: &jsonschema.Schema{
			// "facts or product" is checked in predict, not with a top-level
			// anyOf: an OpenAI-compatible gateway to Gemini refused the whole
			// tool list over that anyOf (D390), so no input schema has
			// anyOf, oneOf or allOf at its root (TestInputSchemasHaveNoRootComposition).
			Type: "object", AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
			Properties: map[string]*jsonschema.Schema{
				"facts":   str("The product facts in plain language: what it is, each part and the country it is made in, and where it is assembled or processed. Up to 12,000 characters.", 20, 12000),
				"product": t.product(),
				"family":  t.family(),
				"date":    pattern("Case date (YYYY-MM-DD); only CBP rulings dated before it are used. Default: today (UTC).", datePattern, 10),
				"current_origin": object("CBP's actual holding on the current setup, if the user has one.", []string{"country"}, map[string]*jsonschema.Schema{
					"country": country("Country CBP held."),
					"ruling":  pattern("The ruling number, e.g. N328954.", `^[NH][0-9]{6}$`, 7),
				}),
				"display_name":    str("Optional name for the case.", 1, 200),
				"case_access_key": accessKey(),
				"jurisdiction":    market("Market the goods enter: us (default), eu, uk, ca, jp, kr, au, mx or other. Omitted: read from the facts (an authority such as HMRC, \"imported into <place>\"), else us. Only us has an origin engine; any other market returns no origin and goes to review."),
			},
		},
		Annotations: writes("Predict country of origin"),
	}, t.predict)

	add(s, &mcp.Tool{
		Name: "extract_product",
		Description: `Draft a structured product (parts, plants, assembly operations) from free text. Nothing is saved.
Use it when the user pastes a supplier note, BOM text or description and you want structured input for predict_origin or plan search.
Input: facts (up to 12,000 characters), optional family and name. Parts with no stated country are listed in warnings, never guessed.
Returns: product, plants, warnings and follow-up questions. Takes about 15 s. REST: POST /v1/products/extract (operationId extractProduct).`,
		InputSchema: object("", []string{"facts"}, map[string]*jsonschema.Schema{
			"facts":  str("The text to read.", 20, 12000),
			"family": t.family(),
			"name":   str("Product name; defaults to the name in the text.", 1, 200),
		}),
		Annotations: readOnly("Extract a structured product", false),
	}, t.extract)

	add(s, &mcp.Tool{
		Name: "plan_production_moves",
		Description: `Search production moves (which operation groups to move to plants in a target country) that earlier CBP rulings support for changing the product's origin to that country. Heavy: about 30 s or more, and a stricter rate limit.
Use it after predict_origin when the user asks how to get a different origin (for example to lower duties).
Input: case_id and target_country. The case needs a structured product with a family, and plants in the target country with capabilities.
If a call times out, call again with the same input: the server joins the search still running instead of starting another. Pass the same idempotency_key on a retry to also get a search that has already finished (kept 15 minutes).
Returns: precedent-supported plans (move, predicted origin, supporting ruling, check confidence, yearly figures when the case has economics), and rejected moves with reasons. Each plan has a plan_id (its move key) and, for shown plans, a plan_index counted from 0; pass the plan_id to export_ruling_request_draft. When no plan is shown, no move is supported by an earlier ruling: do not say that a move changes the origin. Plans are precedent-supported at most, never approved.
Not a ruling: confirm with a licensed customs broker. REST: POST /v1/cases/{id}/plan (operationId planCase).`,
		InputSchema: object("", []string{"case_id", "target_country"}, map[string]*jsonschema.Schema{
			"case_id":         caseID(),
			"target_country":  country("Country of origin to aim for, e.g. Thailand."),
			"case_access_key": accessKey(),
			"idempotency_key": pattern("Optional. Any string you choose, the same on a retry; a new one for a new search.", `^[!-~]{1,200}$`, 200),
			"jurisdiction":    market("Optional. Must be the case's market; plan search works on U.S. cases only."),
		}),
		Annotations: writes("Plan production moves"),
	}, t.plan)

	add(s, &mcp.Tool{
		Name: "compare_target_countries",
		Description: `Run the plan search for 2 to 4 target countries in parallel and rank them (shown plan first, then below the confidence bar, then no supported plan, then no capable plant). Heavy: about as long as one plan search.
Use it when the user asks which country to move production to.
Input: case_id and targets. Returns the ranked countries with each one's best plan.
Not a ruling: confirm with a licensed customs broker. REST: POST /v1/cases/{id}/compare-targets (operationId compareTargets).`,
		InputSchema: object("", []string{"case_id", "targets"}, map[string]*jsonschema.Schema{
			"case_id":         caseID(),
			"targets":         uniq(array("Target countries.", country(""), 2, 4)),
			"case_access_key": accessKey(),
			"jurisdiction":    market("Optional. Must be the case's market; comparison works on U.S. cases only."),
		}),
		Annotations: writes("Compare target countries"),
	}, t.compare)

	if search {
		add(s, &mcp.Tool{
			Name: "search_rulings",
			Description: `Search Unde's corpus of earlier CBP country-of-origin rulings by text (keyword ranking over the facts sections).
Use it to find precedents for a product or an operation, e.g. "printed circuit board assembly motor housing".
Input: query, optional before date and limit. Returns ruling numbers, dates, subjects, CBP's held origin, a facts snippet and a link, plus "superseded" when a later CBP ruling or a final Customs Bulletin notice revoked or modified one, and "proposed" when CBP has only proposed to (information, not a bar). Read one in full with get_ruling.
REST: GET /v1/rulings (operationId searchRulings).`,
			InputSchema: object("", []string{"query"}, map[string]*jsonschema.Schema{
				"query":  str("Words to search for (at least one word of 3 or more letters).", 3, 300),
				"before": pattern("Only rulings dated before this date (YYYY-MM-DD).", datePattern, 10),
				"limit":  integer("How many results (default 10).", 1, 20),
			}),
			Annotations: readOnly("Search CBP rulings", false),
		}, t.search)
	}

	add(s, &mcp.Tool{
		Name: "get_ruling",
		Description: `Read one CBP ruling's text with its date, subject, CBP's held origin and public link on rulings.cbp.gov (and "superseded" when a later CBP ruling or a final Customs Bulletin notice revoked or modified it, "proposed" when CBP has only proposed to).
Use it to read a ruling that a prediction, plan or search cited. Input: ruling_id like N357771 or H301234.
REST: GET /v1/rulings/{id} (operationId getRuling).`,
		InputSchema: object("", []string{"ruling_id"}, map[string]*jsonschema.Schema{
			"ruling_id": pattern("CBP ruling number: N or H and 6 digits.", `^[NH][0-9]{6}$`, 7),
		}),
		Annotations: readOnly("Get a CBP ruling", true),
	}, t.ruling)

	add(s, &mcp.Tool{
		Name: "get_duty_rates",
		Description: `Look up sourced U.S. duty rates for an HTS subheading by country of origin, with every component (MFN, Section 301 and others) and its source.
Use it when the user asks what duty a product pays from a given country. Input: hts (8 or 10 digits, e.g. 8419.89.95) and optional country to narrow the answer.
Returns: the line's description and, per origin, the total ad valorem rate (0.25 = 25%) and its components. Subheadings outside the sourced table may be answered with the live MFN rate only.
REST: GET /v1/duty-rates (operationId getDutyRates).`,
		InputSchema: object("", []string{"hts"}, map[string]*jsonschema.Schema{
			"hts":     pattern("HTS subheading, 8 digits or 10 with a statistical suffix.", `^[0-9][0-9. -]{7,13}$`, 14),
			"country": country("Only this origin's rate."),
		}),
		Annotations: readOnly("Get duty rates", true),
	}, t.duty)

	add(s, &mcp.Tool{
		Name: "get_case",
		Description: `Fetch one of the caller's cases (or a demo case) with its product, prediction, cited rulings and latest plan.
Use it to resume work on a case. Only cases created with the same API key (or case_access_key) and the demo cases are visible; any other id answers not found.
REST: GET /v1/cases/{id} (operationId getCase).`,
		InputSchema: object("", []string{"case_id"}, map[string]*jsonschema.Schema{
			"case_id":         caseID(),
			"case_access_key": accessKey(),
		}),
		Annotations: readOnly("Get a case", false),
	}, t.getCase)

	add(s, &mcp.Tool{
		Name: "list_cases",
		Description: `List the caller's cases and the demo cases (pinned first, then most recently updated): id, name, predicted origin, confidence, triage route and workflow step.
Use it to find a case_id. Only the caller's own cases (same API key or case_access_key) and the demos are listed.
REST: GET /v1/cases (operationId listCases).`,
		InputSchema: object("", nil, map[string]*jsonschema.Schema{
			"case_access_key": accessKey(),
		}),
		Annotations: readOnly("List cases", false),
	}, t.listCases)

	add(s, &mcp.Tool{
		Name: "export_ruling_request_draft",
		Description: `Draft a CBP binding-ruling request letter (Markdown) for a case, for a customs broker to review and complete. A preview: nothing is recorded on the case.
Use it when the user wants to ask CBP for a binding ruling on the current setup or on a plan.
Input: case_id; to draft a plan, its plan_id from plan_production_moves (or plan_index, counted from 0: 0 is the first shown plan), or target with plan_id for a plan from compare_target_countries; format markdown (short) or cfr177 (19 C.F.R. 177.2 layout with [FILL IN] gaps, when the server enables it). With no plan_id, plan_index or move_key the draft is for the current setup: the right call when the plan search showed no plan.
Returns the letter text and reviewer notes. It is a draft: a licensed customs broker must review it. REST: POST /v1/cases/{id}/export (operationId exportCase).`,
		InputSchema: object("", []string{"case_id"}, map[string]*jsonschema.Schema{
			"case_id":         caseID(),
			"format":          enum("markdown (default) or cfr177.", "markdown", "cfr177"),
			"plan_id":         str("A plan's plan_id as plan_production_moves or compare_target_countries returned it, e.g. motor_fan_assembly@th+pcba@th. Omit plan_id, plan_index and move_key for the current setup.", 1, 300),
			"plan_index":      integer("0-based position in the latest plan search's shown plans: 0 is the first plan. Prefer plan_id. Not with plan_id.", 0, 100),
			"target":          country("With plan_id (or move_key): the country of a compare_target_countries search."),
			"move_key":        str("The same value as plan_id (the older name).", 1, 300),
			"case_access_key": accessKey(),
		}),
		Annotations: readOnly("Draft a ruling request", false),
	}, t.export)

	add(s, &mcp.Tool{
		Name: "create_broker_review_link",
		Description: `Make a link a licensed customs broker opens without an account to review one ruling-request draft of a case, and approve it, request changes or mark it to file as a binding ruling request.
Use it when the user wants their broker to check the draft. Input: case_id; optional plan_id (or plan_index, counted from 0), or target with plan_id; none of them for the current setup; format; expires_in_days (default 14, 1 to 30).
Returns review_path (/review#<token> on the Unde web app; the token is the URL fragment, never sent to a server) and the token, shown once: Unde stores only a hash, so it cannot be shown again. Anyone holding the link can read that one draft until it expires or is revoked; share it with the broker only. The draft is frozen when the link is made. Follow up with get_broker_review_status.
Not a ruling: the broker's answer is their professional view, not CBP's. REST: POST /v1/cases/{id}/reviews (operationId createBrokerReviewLink).`,
		InputSchema: object("", []string{"case_id"}, map[string]*jsonschema.Schema{
			"case_id":         caseID(),
			"format":          enum("markdown (default) or cfr177.", "markdown", "cfr177"),
			"plan_id":         str("A plan's plan_id as plan_production_moves or compare_target_countries returned it, e.g. motor_fan_assembly@th+pcba@th. Omit plan_id, plan_index and move_key for the current setup.", 1, 300),
			"plan_index":      integer("0-based position in the latest plan search's shown plans: 0 is the first plan. Prefer plan_id. Not with plan_id.", 0, 100),
			"target":          country("With plan_id (or move_key): the country of a compare_target_countries search."),
			"move_key":        str("The same value as plan_id (the older name).", 1, 300),
			"expires_in_days": integer("Days until the link stops working (default 14).", 1, 30),
			"case_access_key": accessKey(),
		}),
		Annotations: writes("Create a broker review link"),
	}, t.createReview)

	add(s, &mcp.Tool{
		Name: "get_broker_review_status",
		Description: `Read the case's broker review links: each link's status (pending, viewed, approved, changes_requested, file_requested, revoked, expired), what draft it shares, when it expires, and the broker's answers with their comments.
Use it after create_broker_review_link to see whether the broker answered. Input: case_id. Never returns a link's token.
REST: GET /v1/cases/{id}/reviews (operationId listBrokerReviewLinks).`,
		InputSchema: object("", []string{"case_id"}, map[string]*jsonschema.Schema{
			"case_id":         caseID(),
			"case_access_key": accessKey(),
		}),
		Annotations: readOnly("Get broker review status", false),
	}, t.reviewStatus)
}

func uniq(s *jsonschema.Schema) *jsonschema.Schema { s.UniqueItems = true; return s }

// ---- inputs ----

type knownOrigin struct {
	Country string `json:"country"`
	Ruling  string `json:"ruling,omitempty"`
}

type predictIn struct {
	Facts         string         `json:"facts,omitempty"`
	Product       map[string]any `json:"product,omitempty"`
	Family        string         `json:"family,omitempty"`
	Date          string         `json:"date,omitempty"`
	CurrentOrigin *knownOrigin   `json:"current_origin,omitempty"`
	DisplayName   string         `json:"display_name,omitempty"`
	AccessKey     string         `json:"case_access_key,omitempty"`
	Jurisdiction  string         `json:"jurisdiction,omitempty"`
}

type extractIn struct {
	Facts  string `json:"facts"`
	Family string `json:"family,omitempty"`
	Name   string `json:"name,omitempty"`
}

type planIn struct {
	CaseID       string `json:"case_id"`
	Target       string `json:"target_country"`
	AccessKey    string `json:"case_access_key,omitempty"`
	Idempotency  string `json:"idempotency_key,omitempty"`
	Jurisdiction string `json:"jurisdiction,omitempty"`
}

type compareIn struct {
	CaseID       string   `json:"case_id"`
	Targets      []string `json:"targets"`
	AccessKey    string   `json:"case_access_key,omitempty"`
	Jurisdiction string   `json:"jurisdiction,omitempty"`
}

type searchIn struct {
	Query  string `json:"query"`
	Before string `json:"before,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type rulingIn struct {
	RulingID string `json:"ruling_id"`
}

type dutyIn struct {
	HTS     string `json:"hts"`
	Country string `json:"country,omitempty"`
}

type caseIn struct {
	CaseID    string `json:"case_id"`
	AccessKey string `json:"case_access_key,omitempty"`
}

type listIn struct {
	AccessKey string `json:"case_access_key,omitempty"`
}

type exportIn struct {
	CaseID    string `json:"case_id"`
	Format    string `json:"format,omitempty"`
	PlanIndex *int   `json:"plan_index,omitempty"`
	PlanID    string `json:"plan_id,omitempty"`
	Target    string `json:"target,omitempty"`
	MoveKey   string `json:"move_key,omitempty"`
	AccessKey string `json:"case_access_key,omitempty"`
}

type createReviewIn struct {
	CaseID        string `json:"case_id"`
	Format        string `json:"format,omitempty"`
	PlanIndex     *int   `json:"plan_index,omitempty"`
	PlanID        string `json:"plan_id,omitempty"`
	Target        string `json:"target,omitempty"`
	MoveKey       string `json:"move_key,omitempty"`
	ExpiresInDays *int   `json:"expires_in_days,omitempty"`
	AccessKey     string `json:"case_access_key,omitempty"`
}

// ---- handlers ----

func (t tools) predict(ctx context.Context, hdr http.Header, in predictIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Facts) == "" && in.Product == nil {
		return toolError("Give facts (a plain-language description) or a structured product.")
	}
	date := in.Date
	if date == "" {
		date = time.Now().UTC().Format(time.DateOnly)
	}
	body := map[string]any{"date": date}
	if in.Facts != "" {
		body["facts"] = in.Facts
	}
	if in.Product != nil {
		body["product"] = in.Product
	}
	if in.Family != "" {
		body["family"] = in.Family
	}
	if in.CurrentOrigin != nil {
		body["current_origin"] = in.CurrentOrigin
	}
	if in.DisplayName != "" {
		body["display_name"] = in.DisplayName
	}
	if in.Jurisdiction != "" {
		body["jurisdiction"] = in.Jurisdiction
	}
	// An anonymous caller gets a case access key: without one, a case made
	// with CASE_OWNERSHIP on could not be read back (nothing else ties two
	// stateless MCP requests to the same caller).
	access := in.AccessKey
	minted := false
	if !hasKey(hdr) && access == "" {
		access, minted = newAccessKey(), true
	}
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: "/v1/cases/predict", body: body, access: access})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	var b strings.Builder
	p := obj(out["prediction"])
	if md := obj(out["jurisdiction_detail"]); md["supported"] == false {
		// No engine ran for this market (D680): say so, never an origin.
		fmt.Fprintf(&b, "Market: %s. %s. No origin was predicted.", str0(md["name"], "unknown"), str0(obj(out["triage"])["reason"], "Not supported yet"))
		fmt.Fprintf(&b, "\nIf the goods enter the United States, call predict_origin again with jurisdiction \"us\".\nCase id: %v.", out["id"])
		if access != "" && !hasKey(hdr) {
			out["case_access_key"] = access
		}
		return done(b.String(), out)
	}
	fmt.Fprintf(&b, "Likely origin: %s (%s).", str0(p["origin_country"], "unknown"), confPhrase(p))
	if d := str0(p["decisive_step"], ""); d != "" {
		fmt.Fprintf(&b, " Decisive step: %s.", d)
	}
	if tr := obj(out["triage"]); tr["route"] != nil {
		fmt.Fprintf(&b, "\nTriage: %v. %s", tr["route"], str0(tr["reason"], ""))
	}
	if cited := list(p["cited_rulings"]); len(cited) > 0 {
		fmt.Fprintf(&b, "\nCited CBP rulings: %s.", joinAny(cited))
	}
	if cl := obj(out["clarify"]); len(list(cl["questions"])) > 0 {
		fmt.Fprintf(&b, "\nOpen questions: %d (see clarify.questions).", len(list(cl["questions"])))
	}
	if l := ruleCheckLine(out); l != "" {
		b.WriteString("\n" + l)
	}
	fmt.Fprintf(&b, "\nCase id: %v.", out["id"])
	if access != "" && !hasKey(hdr) {
		out["case_access_key"] = access
		if minted {
			b.WriteString("\ncase_access_key returned: pass it with the case_id to later calls on this case. Keep it private; anyone holding it can read the case.")
		}
	}
	return done(b.String(), out)
}

func (t tools) extract(ctx context.Context, hdr http.Header, in extractIn) (*mcp.CallToolResult, any, error) {
	body := map[string]any{"facts": in.Facts}
	if in.Family != "" {
		body["family"] = in.Family
	}
	if in.Name != "" {
		body["name"] = in.Name
	}
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: "/v1/products/extract", body: body})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	pr := obj(out["product"])
	var b strings.Builder
	fmt.Fprintf(&b, "Draft product %q: %d parts, %d plants, %d operations.", str0(pr["name"], ""), len(list(pr["components"])), len(list(out["plants"])), len(list(pr["operations"])))
	for _, w := range list(out["warnings"]) {
		fmt.Fprintf(&b, "\nWarning: %v", w)
	}
	if q := list(out["questions"]); len(q) > 0 {
		fmt.Fprintf(&b, "\n%d follow-up questions (see questions).", len(q))
	}
	b.WriteString("\nReview it, then pass it as product to predict_origin.")
	return done(b.String(), out)
}

func (t tools) plan(ctx context.Context, hdr http.Header, in planIn) (*mcp.CallToolResult, any, error) {
	body := map[string]any{"target": in.Target}
	if in.Jurisdiction != "" {
		body["jurisdiction"] = in.Jurisdiction
	}
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: casePath(in.CaseID, "/plan"), body: body, access: in.AccessKey, idempotency: in.Idempotency})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	pl := obj(out["plan"])
	plans, rejected := list(pl["plans"]), list(pl["rejected"])
	var b strings.Builder
	fmt.Fprintf(&b, "Target %s: %d precedent-supported plans, %d rejected moves.", str0(pl["target"], in.Target), len(plans), len(rejected))
	for i, x := range plans {
		if i == 5 {
			fmt.Fprintf(&b, "\n... %d more in plan.plans.", len(plans)-5)
			break
		}
		p := obj(x)
		p["plan_index"] = i
		fmt.Fprintf(&b, "\n- plan_id %s (plan_index %d): %s", str0(p["plan_id"], "?"), i, planLine(p))
	}
	if len(plans) > 0 {
		b.WriteString("\nTo draft a plan, pass its plan_id to export_ruling_request_draft.")
	} else {
		b.WriteString("\n" + NoPlanLine)
		if len(rejected) > 0 {
			fmt.Fprintf(&b, "\nClosest rejected: %s", planLine(obj(rejected[0])))
		}
		b.WriteString("\nTo draft for the current setup, call export_ruling_request_draft without plan_id, plan_index or move_key.")
	}
	if rc := obj(obj(pl["baseline"])["rule_check"]); len(rc) > 0 {
		b.WriteString("\n" + planRuleNote(rc["applies"] == "part102"))
	}
	return done(b.String(), out)
}

// planRuleNote says what the per-move 19 CFR 102.20 check does to a plan
// list: on a Part 102 case it orders shown plans within each tier (D500);
// otherwise it is for information only (D441).
func planRuleNote(part102 bool) string {
	if part102 {
		return "This case is decided under Part 102, so within each tier shown plans rank by the 19 CFR 102.20 rule check (rule_check): moves it says shift first, then moves it cannot decide, then moves it gives to another country (flagged for a broker). It never changes which plans are shown or their verdicts."
	}
	return "The 19 CFR 102.20 rule check on each move (rule_check) is display only: it never changes which plans are shown or their order. This case is decided under substantial transformation, so it is for information."
}

// NoPlanLine is what plan_production_moves says when it shows no plan
// (D470): the agent evaluation (D451, a03) saw an agent claim a move
// would change the origin after a search that supported none.
const NoPlanLine = "No move is supported by an earlier ruling; do not claim a move changes origin."

func (t tools) compare(ctx context.Context, hdr http.Header, in compareIn) (*mcp.CallToolResult, any, error) {
	body := map[string]any{"targets": in.Targets}
	if in.Jurisdiction != "" {
		body["jurisdiction"] = in.Jurisdiction
	}
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: casePath(in.CaseID, "/compare-targets"), body: body, access: in.AccessKey})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	var b strings.Builder
	b.WriteString("Ranked targets:")
	for _, x := range list(out["targets"]) {
		ts := obj(x)
		fmt.Fprintf(&b, "\n%v. %v: %v", ts["rank"], ts["target"], ts["status"])
		if best := obj(ts["best"]); len(best) > 0 {
			fmt.Fprintf(&b, ". Best: plan_id %s: %s", str0(best["plan_id"], "?"), planLine(best))
		}
	}
	for _, x := range list(out["targets"]) {
		if rc := obj(obj(obj(x)["best"])["rule_check"]); len(rc) > 0 {
			b.WriteString("\n" + planRuleNote(rc["applies"] == "part102"))
			break
		}
	}
	return done(b.String(), out)
}

func (t tools) search(ctx context.Context, hdr http.Header, in searchIn) (*mcp.CallToolResult, any, error) {
	q := url.Values{"q": {in.Query}}
	if in.Before != "" {
		q.Set("before", in.Before)
	}
	if in.Limit > 0 {
		q.Set("limit", fmt.Sprint(in.Limit))
	}
	res, err := t.call(ctx, hdr, apiCall{method: "GET", path: "/v1/rulings", query: q})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	hits := list(out["rulings"])
	var b strings.Builder
	fmt.Fprintf(&b, "%d rulings found.", len(hits))
	for _, x := range hits {
		h := obj(x)
		fmt.Fprintf(&b, "\n%v (%v): %s. Held origin: %s.", h["id"], h["date"], str0(h["subject"], "no subject"), str0(h["origin"], "none stated"))
	}
	return done(b.String(), out)
}

func (t tools) ruling(ctx context.Context, hdr http.Header, in rulingIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "GET", path: "/v1/rulings/" + url.PathEscape(in.RulingID)})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	text := str0(out["text"], "")
	if r := []rune(text); len(r) > 6000 {
		text = string(r[:6000]) + "\n[... truncated; the full text is in structuredContent.text and at the url]"
	}
	return done(fmt.Sprintf("%v (%v): %s\nHeld origin: %s\n%v\n\n%s", out["id"], out["date"], str0(out["subject"], ""), str0(out["origin"], "none stated"), out["url"], text), out)
}

func (t tools) duty(ctx context.Context, hdr http.Header, in dutyIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "GET", path: "/v1/duty-rates", query: url.Values{"hts": {in.HTS}}})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	if in.Country != "" {
		var keep []any
		for _, x := range list(out["rates"]) {
			if strings.EqualFold(str0(obj(x)["origin"], ""), in.Country) {
				keep = append(keep, x)
			}
		}
		out["rates"] = keep
	}
	var b strings.Builder
	fmt.Fprintf(&b, "HTS %v: %s.", out["hts"], str0(out["description"], ""))
	for _, x := range list(out["rates"]) {
		r := obj(x)
		fmt.Fprintf(&b, "\nFrom %v: %s total ad valorem", r["origin"], pct(r["total"]))
		if r["unverified"] == true {
			b.WriteString(" (unverified)")
		}
		b.WriteString(".")
	}
	if m := obj(out["live_mfn"]); len(m) > 0 {
		fmt.Fprintf(&b, "\nLive MFN (column 1 general) rate from the USITC HTS, without additional duties: %s.", str0(m["general"], "see live_mfn"))
	}
	if in.Country != "" && len(list(out["rates"])) == 0 && out["live_mfn"] == nil {
		fmt.Fprintf(&b, "\nNo sourced rate for %s on this line.", in.Country)
	}
	return done(b.String(), out)
}

func (t tools) getCase(ctx context.Context, hdr http.Header, in caseIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "GET", path: casePath(in.CaseID, ""), access: in.AccessKey})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	p := obj(out["prediction"])
	var b strings.Builder
	fmt.Fprintf(&b, "Case %v %q (step %v).", out["id"], str0(out["display_name"], str0(obj(out["product"])["name"], "")), out["step"])
	if len(p) > 0 {
		fmt.Fprintf(&b, "\nPredicted origin: %s (%s).", str0(p["origin_country"], "unknown"), confPhrase(p))
	}
	if pl := obj(out["plan"]); len(pl) > 0 {
		fmt.Fprintf(&b, "\nLatest plan search: target %v, %d shown plans.", pl["target"], len(list(pl["plans"])))
	}
	if l := ruleCheckLine(out); l != "" {
		b.WriteString("\n" + l)
	}
	if l := rulingAlertLines(out); l != "" {
		b.WriteString("\n" + l)
	}
	return done(b.String(), out)
}

// rulingAlertLines words the case's revocation alerts (D480), or "" when
// it has none. The saved answer is not changed by them.
func rulingAlertLines(out map[string]any) string {
	ra := obj(out["ruling_alerts"])
	if len(ra) == 0 {
		return ""
	}
	var lines []string
	var final, readOnly int
	for _, x := range list(ra["items"]) {
		a := obj(x)
		relied := false
		for _, r := range list(a["roles"]) {
			if r != "read" {
				relied = true
			}
		}
		switch {
		case !relied:
			if a["status"] == "final" {
				readOnly++
			}
		case a["status"] == "final":
			final++
			l := fmt.Sprintf("Ruling %s relied on by this case was %s.", str0(a["ruling"], ""), str0(a["note"], "changed"))
			if a["in_effect"] != true {
				l = fmt.Sprintf("Ruling %s relied on by this case is scheduled to be %s.", str0(a["ruling"], ""), str0(a["note"], "changed"))
			}
			if n := str0(a["relevance_note"], ""); n != "" {
				l += " " + n
			}
			if r := a["relevance"]; r == "not_answer" || r == "holding_unchanged" {
				l += " Re-check still recommended."
			} else {
				l += " Re-check recommended."
			}
			lines = append(lines, l)
		default:
			lines = append(lines, fmt.Sprintf("Ruling %s relied on by this case: %s; proposed, not final, so information only.", str0(a["ruling"], ""), str0(a["note"], "")))
		}
	}
	if readOnly > 0 {
		lines = append(lines, fmt.Sprintf("%d ruling(s) the prediction read but did not cite have changed since.", readOnly))
	}
	if final > 0 {
		if ra["rechecked_at"] != nil {
			lines = append(lines, fmt.Sprintf("The ruling watch re-checked this case on %v (see watch).", ra["rechecked_at"]))
		}
		lines = append(lines, "The saved answer is unchanged; a re-check runs the ruling watch with today's rulings (REST POST /v1/cases/{id}/watch).")
	}
	return "Changed rulings: " + strings.Join(lines, " ")
}

// ruleCheckLine summarises the 19 CFR 102.20 rule check (D363), or "" when
// the case has none. It never restates the model's answer as the rule's.
func ruleCheckLine(out map[string]any) string {
	rc := obj(out["rule_check"])
	if len(rc) == 0 {
		return ""
	}
	res := obj(rc["result"])
	var b strings.Builder
	b.WriteString("Rule check (19 CFR 102.20")
	if rc["applies"] == "informational" {
		b.WriteString(", for information: this case is decided under substantial transformation")
	}
	b.WriteString("): ")
	switch res["status"] {
	case "decided":
		fmt.Fprintf(&b, "%s under %s.", str0(res["origin"], "unknown"), str0(res["basis"], "the rule"))
		if rc["review"] == true {
			b.WriteString(" It differs from the model's answer: worth a broker's look.")
		}
	case "out_of_scope":
		fmt.Fprintf(&b, "not covered (%s).", str0(res["reason"], ""))
	default:
		fmt.Fprintf(&b, "undetermined: %s.", str0(res["reason"], ""))
	}
	if rb := obj(rc["robustness"]); len(rb) > 0 {
		fmt.Fprintf(&b, " Over the other suggested part codes: %s.", str0(rb["summary"], ""))
	}
	b.WriteString(" Display only; it does not change the prediction or triage.")
	return b.String()
}

func (t tools) listCases(ctx context.Context, hdr http.Header, in listIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "GET", path: "/v1/cases", access: in.AccessKey})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	cs := list(out["cases"])
	var b strings.Builder
	fmt.Fprintf(&b, "%d cases.", len(cs))
	for i, x := range cs {
		if i == 25 {
			fmt.Fprintf(&b, "\n... %d more in cases.", len(cs)-25)
			break
		}
		c := obj(x)
		demo := ""
		if c["pinned"] == true {
			demo = " [demo]"
		}
		fmt.Fprintf(&b, "\n%v%s: %v. Origin %s, step %v.", c["id"], demo, c["display_name"], str0(c["predicted_origin"], "not analysed"), c["step"])
		if n := obj(c["ruling_alerts"]); len(n) > 0 {
			fmt.Fprintf(&b, " Changed rulings it relies on: %v final, %v proposed.", n["final"], n["proposed"])
			if t, ok := n["touches_answer"]; ok {
				fmt.Fprintf(&b, " %v bear on the answer, a shown plan or the current setup.", t)
			}
		}
	}
	return done(b.String(), out)
}

func (t tools) export(ctx context.Context, hdr http.Header, in exportIn) (*mcp.CallToolResult, any, error) {
	body := map[string]any{"preview": true}
	if in.Format != "" {
		body["format"] = in.Format
	}
	if in.PlanIndex != nil {
		body["plan_index"] = *in.PlanIndex
	}
	if in.Target != "" {
		body["target"] = in.Target
	}
	if in.MoveKey != "" {
		body["move_key"] = in.MoveKey
	}
	if in.PlanID != "" {
		body["plan_id"] = in.PlanID
	}
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: casePath(in.CaseID, "/export"), body: body, access: in.AccessKey})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	var b strings.Builder
	b.WriteString(str0(out["content"], ""))
	if notes := list(out["notes"]); len(notes) > 0 {
		b.WriteString("\n\nReviewer notes (not part of the letter):")
		for _, n := range notes {
			fmt.Fprintf(&b, "\n- %v", n)
		}
	}
	return done(b.String(), out)
}

func (t tools) createReview(ctx context.Context, hdr http.Header, in createReviewIn) (*mcp.CallToolResult, any, error) {
	body := map[string]any{}
	if in.Format != "" {
		body["format"] = in.Format
	}
	if in.PlanIndex != nil {
		body["plan_index"] = *in.PlanIndex
	}
	if in.Target != "" {
		body["target"] = in.Target
	}
	if in.MoveKey != "" {
		body["move_key"] = in.MoveKey
	}
	if in.PlanID != "" {
		body["plan_id"] = in.PlanID
	}
	if in.ExpiresInDays != nil {
		body["expires_in_days"] = *in.ExpiresInDays
	}
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: casePath(in.CaseID, "/reviews"), body: body, access: in.AccessKey})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	rv := obj(out["review"])
	var b strings.Builder
	fmt.Fprintf(&b, "Broker review link for %s, expires %v.", str0(rv["draft_label"], "the draft"), rv["expires_at"])
	fmt.Fprintf(&b, "\nOpen on the Unde web app at: %v", out["review_path"])
	b.WriteString("\nThe link is shown once; Unde keeps only a hash. Share it with the broker only: whoever holds it can read this draft until it expires or is revoked.")
	return done(b.String(), out)
}

func (t tools) reviewStatus(ctx context.Context, hdr http.Header, in caseIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "GET", path: casePath(in.CaseID, "/reviews"), access: in.AccessKey})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	rvs := list(out["reviews"])
	var b strings.Builder
	fmt.Fprintf(&b, "%d broker review links.", len(rvs))
	for _, x := range rvs {
		rv := obj(x)
		fmt.Fprintf(&b, "\n%v: %v (%s), expires %v.", rv["id"], rv["status"], str0(rv["draft_label"], ""), rv["expires_at"])
		for _, d := range list(rv["decisions"]) {
			dd := obj(d)
			if c := str0(dd["comment"], ""); c != "" {
				fmt.Fprintf(&b, "\n  %v at %v: %q", dd["decision"], dd["at"], c)
			} else {
				fmt.Fprintf(&b, "\n  %v at %v", dd["decision"], dd["at"])
			}
		}
	}
	return done(b.String(), out)
}

// ---- results ----

// done is a successful tool result: the summary text with the disclaimer,
// and the API's answer (trimmed) as structured content.
func done(text string, out map[string]any) (*mcp.CallToolResult, any, error) {
	if _, ok := out["disclaimer"]; !ok {
		out["disclaimer"] = Disclaimer
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text + "\n\n" + Disclaimer}}}, out, nil
}

// failed is a tool error: the API's status, code and message, so the model
// can tell a rate limit (retry later) from bad input (fix it) or a missing
// case (wrong id or key).
func failed(res apiResult, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return nil, nil, err
	}
	code, msg := str0(res.body["code"], "error"), str0(res.body["message"], http.StatusText(res.status))
	text := fmt.Sprintf("Unde API error %d %s: %s", res.status, code, msg)
	switch {
	case res.status == http.StatusNotFound && code == "not_found":
		text += "\nThe id does not exist or is not visible to this caller: cases are private to the API key, or the case_access_key, that created them."
	case res.status == http.StatusTooManyRequests || code == "budget_reached":
		if ra := str0(res.body["retry_after"], ""); ra != "" {
			text += fmt.Sprintf("\nRetry after %s seconds.", ra)
		}
	case res.status == http.StatusUnauthorized && code == "unauthorized":
		text += "\nCheck the API key in the connection's Authorization header."
	}
	out := map[string]any{"status": res.status, "error": res.body}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: text}}}, out, nil
}

// trim drops bulky internals (search trails, model traces, cost stats,
// history, chat) from the structured result; get_case over REST has them.
func trim(m map[string]any) map[string]any {
	for _, k := range []string{"trail", "trace", "stats", "history", "chat"} {
		delete(m, k)
	}
	for k, v := range m {
		m[k] = trimAny(v)
	}
	return m
}

func trimAny(v any) any {
	switch x := v.(type) {
	case map[string]any:
		return trim(x)
	case []any:
		for i := range x {
			x[i] = trimAny(x[i])
		}
	}
	return v
}

func planLine(p map[string]any) string {
	var b strings.Builder
	b.WriteString(str0(p["description"], "move"))
	pred := obj(p["prediction"])
	if o := str0(pred["origin_country"], ""); o != "" {
		fmt.Fprintf(&b, "; predicted origin %s", o)
	}
	v := obj(p["verdict"])
	if r := str0(v["supporting_ruling"], ""); r != "" {
		fmt.Fprintf(&b, "; supported by %s (check confidence %s)", r, pct(v["confidence"]))
	}
	if p["net_priced"] == true {
		fmt.Fprintf(&b, "; modelled net %v USD a year", p["net_per_year"])
	}
	if l := planRuleWords(p); l != "" {
		b.WriteString("; " + l)
	}
	if r := str0(p["rejection_reason"], ""); r != "" {
		fmt.Fprintf(&b, "; not shown: %s", r)
	}
	return b.String()
}

// planRuleWords is a move's 19 CFR 102.20 rule check (D441) in a clause, or
// "" when the plan has none.
func planRuleWords(p map[string]any) string {
	rc := obj(p["rule_check"])
	l := str0(rc["label"], "")
	if l == "" {
		return ""
	}
	out := "102.20 rule check: " + l
	if rc["review"] != true {
		return out
	}
	// The flag has two sources (D441, D591): the rule disagrees with the
	// precedent check, or, on a Part 102 case, with the engine on a row no
	// precedent check ran on. review_note says which; without one, only a
	// row the precedent check saw can be called a precedent disagreement.
	switch note := strings.TrimSuffix(strings.TrimSpace(str0(rc["review_note"], "")), "."); {
	case note != "":
		out += " (" + strings.ToLower(note[:1]) + note[1:] + ")"
	case len(obj(p["verdict"])) > 0:
		out += " (it differs from the precedent check: worth a broker's look)"
	default:
		out += " (worth a broker's look)"
	}
	return out
}

func casePath(id, rest string) string { return "/v1/cases/" + url.PathEscape(id) + rest }

func hasKey(h http.Header) bool { return h != nil && h.Get("X-API-Key") != "" }

func newAccessKey() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "mcp_" + hex.EncodeToString(b)
}

func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }
func list(v any) []any         { l, _ := v.([]any); return l }

func str0(v any, def string) string {
	if s, ok := v.(string); ok && s != "" {
		return s
	}
	return def
}

// confPhrase names which confidence a prediction carries: the calibrated
// probability when the API returned one (D160), else the model's stated
// value, said to be uncalibrated.
func confPhrase(p map[string]any) string {
	if c, ok := p["calibrated_confidence"].(float64); ok {
		return fmt.Sprintf("calibrated confidence %s; the model stated %s", pct(c), pct(p["confidence"]))
	}
	return fmt.Sprintf("confidence %s as stated by the model, not calibrated", pct(p["confidence"]))
}

func pct(v any) string {
	f, ok := v.(float64)
	if !ok {
		return "unknown"
	}
	return fmt.Sprintf("%.0f%%", f*100)
}

func joinAny(xs []any) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, ", ")
}
