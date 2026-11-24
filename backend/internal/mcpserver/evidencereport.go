package mcpserver

import (
	"context"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerEvidenceReport adds get_evidence_report (#771, D803): the
// separate checks on one case's product, as GET /v1/cases/{id}/evidence-report.
func (t tools) registerEvidenceReport(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name: "get_evidence_report",
		Description: `Get a case's evidence report: every rules check the server serves, run separately on the case's product as stored (U.S. 19 CFR 102.20 tariff shift, Canada marking, EU and UK non-preferential origin, RCEP preference, USMCA preference for pilot goods), beside the case's stored U.S. answer (reused, never re-run: no model call). Each check is its own section with the legal question it answers, its outcome, missing information and the document that would settle it, what is not modelled, its sources, its rule or data version and its limits. The case's facts are shown as stored, and facts the user supplied to a check are listed apart from them.
Use it when the user wants one product compared across jurisdictions or a report to file. The checks answer different legal questions: never merge them into one verdict, score or "overall" answer, and never answer a needs_judgement check yourself. Not a ruling.
Returns the report as Markdown, and the structured report. Nothing is saved. REST: GET /v1/cases/{id}/evidence-report (operationId getEvidenceReport).`,
		InputSchema: object("", []string{"case_id"}, map[string]*jsonschema.Schema{
			"case_id":         caseID(),
			"case_access_key": accessKey(),
		}),
		Annotations: readOnly("Get an evidence report", false),
	}, t.evidenceReport)
}

func (t tools) evidenceReport(ctx context.Context, hdr http.Header, in caseIn) (*mcp.CallToolResult, any, error) {
	res, err := t.call(ctx, hdr, apiCall{method: "GET", path: casePath(in.CaseID, "/evidence-report"), access: in.AccessKey})
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	md := str0(out["markdown"], "")
	delete(out, "markdown") // the text content carries it once
	return done(md, out)
}
