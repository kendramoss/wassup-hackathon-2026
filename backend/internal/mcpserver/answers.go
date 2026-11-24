package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type answersIn struct {
	CaseID            string                    `json:"case_id"`
	AccessKey         string                    `json:"case_access_key,omitempty"`
	Check             string                    `json:"check"`
	ExpectedUpdatedAt *string                   `json:"expected_updated_at,omitempty"`
	GoodHTS           string                    `json:"good_hts,omitempty"`
	Row               string                    `json:"row,omitempty"`
	Answers           map[string]bool           `json:"answers,omitempty"`
	Materials         []map[string]any          `json:"materials,omitempty"`
	FamilyAnswers     map[string]map[string]any `json:"family_answers,omitempty"`
}

// registerAnswers adds save_case_answers (#770, D816): the REST saves of
// PUT /v1/cases/{id}/origin-facts/{market} and PUT /v1/cases/{id}/family-facts,
// with their validation, staleness and precondition, since the tool calls
// the routes themselves.
func (t tools) registerAnswers(s *mcp.Server) {
	share := func(desc string) *jsonschema.Schema {
		return &jsonschema.Schema{Type: "number", Description: desc, Minimum: ptr(0.0), Maximum: ptr(1.0)}
	}
	code := func(desc string) *jsonschema.Schema { return pattern(desc, `^[0-9]{4}(?:[. ]?[0-9]{2}){0,3}$`, 14) }
	material := object("Facts for one part of the case's product, as the check's needs ask.", []string{"index", "name"}, map[string]*jsonschema.Schema{
		"index":        {Type: "integer", Description: "The part's position in product.components (0-based).", Minimum: ptr(0.0), Maximum: ptr(49.0)},
		"name":         str("The part's name as it stands (if it changes, the facts are not read).", 1, 200),
		"part_hts":     pattern("The part's own code as it stands, if it has one.", `^[0-9. ]{0,14}$`, 14),
		"hts":          code("A code for a part with none, or a longer code that starts with the part's own."),
		"origin":       country("Origin for a part with none."),
		"value_share":  share("Share of the good's ex-works price (0 to 1)."),
		"weight_share": share("Share of the good's weight (0 to 1)."),
	})
	familyAnswer := &jsonschema.Schema{Type: "object", Description: "A country for a \"country\" question, or yes for a \"yes_no\" question.",
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		Properties: map[string]*jsonschema.Schema{
			"country": country("Where it was made or done."),
			"yes":     {Type: "boolean", Description: "Yes or no."},
		}}
	add(s, &mcp.Tool{
		Name: "save_case_answers",
		Description: `Save the user's answers to a case's missing information. No model call; the case's facts, product and parts list never change: answers are kept beside them, marked user-supplied with a time.
check "eu" or "uk": facts for that market's non-preferential origin check, as its result.needs ask (get_evidence_report, or check_eu_origin / check_uk_origin with the case's product): good_hts, row, answers (yes/no keyed as needs name them), materials. On a case asked for that market the stored answer is re-run with them (rules only).
check "family": answers to the case's family questions (get_case: family_intake.questions with state "missing"), keyed by question id: {"country": ...} for kind country, {"yes": true|false} for kind yes_no. Only asked questions are accepted; a family_intake with status not_modelled takes none. The U.S. answer is a model answer and is not re-run.
Each save replaces that check's saved answers; send them all, or nothing to clear. Pass expected_updated_at, the saved answers' updated_at as you read them ("" when none): if they changed since, nothing is saved (conflict). Answers saved before the parts list was replaced are marked stale and not read. Never invent an answer: ask the user, and say which document settles each (each question names it).
Returns the saved answers and the check or questions read with them. REST: PUT /v1/cases/{id}/origin-facts/{market} (operationId putOriginFacts) and PUT /v1/cases/{id}/family-facts (operationId putFamilyFacts).`,
		InputSchema: object("", []string{"case_id", "check"}, map[string]*jsonschema.Schema{
			"case_id":             caseID(),
			"case_access_key":     accessKey(),
			"check":               enum("Which answers: the EU or UK origin check, or the family questions.", "eu", "uk", "family"),
			"expected_updated_at": str("The saved answers' updated_at as you last read them, or \"\" when none were saved.", 0, 40),
			"good_hts":            code("eu/uk: a longer code for the good that starts with the product's own."),
			"row":                 str("eu/uk: the list row the good is (from needs options), or \"none\".", 1, 40),
			"answers": {Type: "object", Description: "eu/uk: yes/no facts keyed as needs name them.",
				AdditionalProperties: &jsonschema.Schema{Type: "boolean"}},
			"materials": array("eu/uk: facts for parts of the product.", material, 0, 50),
			"family_answers": {Type: "object", Description: "family: answers keyed by question id.",
				AdditionalProperties: familyAnswer},
		}),
		Annotations: &mcp.ToolAnnotations{Title: "Save answers to a case's questions", IdempotentHint: false, DestructiveHint: ptr(false), OpenWorldHint: ptr(false)},
	}, t.saveAnswers)
}

func (t tools) saveAnswers(ctx context.Context, hdr http.Header, in answersIn) (*mcp.CallToolResult, any, error) {
	body := map[string]any{}
	if in.ExpectedUpdatedAt != nil {
		body["expected_updated_at"] = *in.ExpectedUpdatedAt
	}
	path := ""
	switch in.Check {
	case "family":
		if in.GoodHTS != "" || in.Row != "" || len(in.Answers) > 0 || len(in.Materials) > 0 {
			return toolError("check \"family\" takes family_answers only")
		}
		if len(in.FamilyAnswers) > 0 {
			body["answers"] = in.FamilyAnswers
		}
		path = casePath(in.CaseID, "/family-facts")
	case "eu", "uk":
		if len(in.FamilyAnswers) > 0 {
			return toolError("family_answers go with check \"family\"")
		}
		if in.GoodHTS != "" {
			body["good_hts"] = in.GoodHTS
		}
		if in.Row != "" {
			body["row"] = in.Row
		}
		if len(in.Answers) > 0 {
			body["answers"] = in.Answers
		}
		if len(in.Materials) > 0 {
			body["materials"] = in.Materials
		}
		path = casePath(in.CaseID, "/origin-facts/"+in.Check)
	default:
		return toolError("check must be eu, uk or family")
	}
	res, err := t.call(ctx, hdr, apiCall{method: "PUT", path: path, body: body, access: in.AccessKey})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	var b strings.Builder
	if in.Check == "family" {
		saved := obj(out["family_facts"])
		if len(obj(saved["answers"])) == 0 {
			b.WriteString("Cleared the answers to the family questions.")
		} else {
			fmt.Fprintf(&b, "Saved %d answer(s) to the family questions, at %s.", len(obj(saved["answers"])), str0(saved["updated_at"], ""))
		}
		fi := obj(out["family_intake"])
		var open []string
		for _, x := range list(fi["questions"]) {
			q := obj(x)
			if q["state"] == "missing" {
				open = append(open, fmt.Sprintf("%s (%s) [settled by: %s]", str0(q["question"], ""), str0(q["id"], ""), str0(q["evidence"], "")))
			}
		}
		if len(open) > 0 {
			fmt.Fprintf(&b, "\nStill missing: %s.", strings.Join(open, "; "))
		} else {
			b.WriteString("\nNo family question is open.")
		}
		if fi["status"] != "not_modelled" && len(obj(saved["answers"])) > 0 {
			b.WriteString("\nThe case's U.S. answer is a model answer and was not re-run.")
		}
		return done(b.String(), out)
	}
	r := obj(obj(out["check"])["result"])
	m := strings.ToUpper(in.Check)
	fmt.Fprintf(&b, "Saved the %s answers.", m)
	switch r["status"] {
	case "decided":
		fmt.Fprintf(&b, "\n%s non-preferential origin with them: %s (%s).", m, str0(r["origin"], ""), str0(r["basis"], ""))
	case "needs_judgement":
		b.WriteString("\nThe rules do not decide this good by code: a person has to judge it.")
	default:
		b.WriteString("\nNot decided yet.")
	}
	var next []string
	for _, x := range list(r["needs"]) {
		n := obj(x)
		if n["kind"] == "review" {
			continue
		}
		l := str0(n["label"], str0(n["key"], ""))
		if ev := str0(n["evidence"], ""); ev != "" {
			l += " [settled by: " + ev + "]"
		}
		next = append(next, l)
	}
	if len(next) > 0 {
		fmt.Fprintf(&b, "\nStill missing: %s.", strings.Join(next, "; "))
	}
	if out["prediction"] != nil {
		b.WriteString("\nThe case's stored answer was re-run with them.")
	}
	b.WriteString("\nA rules check, not a ruling.")
	return done(b.String(), out)
}
