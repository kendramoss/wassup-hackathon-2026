package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The demo cases as resources: unde://demos lists them, unde://demos/{id} is
// one with its ready predict and plan request bodies (GET /v1/demos...).
func (t tools) registerResources(s *mcp.Server) {
	s.AddResource(&mcp.Resource{
		URI: "unde://demos", Name: "demos", Title: "Unde demo cases", MIMEType: "application/json",
		Description: "The demo products (real CBP rulings) with their ids. Read unde://demos/{id} for one with ready request bodies.",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return t.resource(ctx, req, "/v1/demos")
	})
	s.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "unde://demos/{id}", Name: "demo", Title: "Unde demo case", MIMEType: "application/json",
		Description: "One demo product: facts, product, the CBP holding and ready predict_origin and plan inputs.",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		id := strings.TrimPrefix(req.Params.URI, "unde://demos/")
		if id == "" || strings.ContainsAny(id, "/?#") || len(id) > 64 {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		return t.resource(ctx, req, "/v1/demos/"+url.PathEscape(id))
	})
}

func (t tools) resource(ctx context.Context, req *mcp.ReadResourceRequest, path string) (*mcp.ReadResourceResult, error) {
	var hdr http.Header
	if req.Extra != nil {
		hdr = req.Extra.Header
	}
	res, err := t.call(ctx, hdr, apiCall{method: "GET", path: path})
	if err != nil {
		return nil, err
	}
	if res.status == http.StatusNotFound {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	if !res.ok() {
		return nil, fmt.Errorf("unde API error %d %v", res.status, res.body["code"])
	}
	b, err := json.MarshalIndent(res.body, "", "  ")
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: string(b)}}}, nil
}

// One prompt: the whole flow for a production move.
func (t tools) registerPrompts(s *mcp.Server) {
	s.AddPrompt(&mcp.Prompt{
		Name: "plan_production_move", Title: "Plan a production move",
		Description: "Predict a product's U.S. country of origin, then search production moves that earlier CBP rulings support for a target country.",
		Arguments: []*mcp.PromptArgument{
			{Name: "product", Description: "The product: its parts, where each is made, and where it is assembled.", Required: true},
			{Name: "target_country", Description: "The country of origin to aim for, e.g. Thailand.", Required: true},
		},
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		product, target := req.Params.Arguments["product"], req.Params.Arguments["target_country"]
		if strings.TrimSpace(product) == "" || strings.TrimSpace(target) == "" {
			return nil, fmt.Errorf("product and target_country are required")
		}
		text := fmt.Sprintf(`Use the Unde tools to plan a production move for this product, aiming for %s as the U.S. country of origin.

1. Call extract_product with the product text, and show me the parts and plants it found. Ask me about anything it flags.
2. Call predict_origin with the facts and the reviewed product. Tell me the likely origin, the confidence and the cited CBP rulings.
3. Call plan_production_moves with the case id and %s. Summarise the precedent-supported plans and why the closest rejected moves failed.
4. Offer export_ruling_request_draft for the best plan.

Say clearly that these are predictions, not rulings, and that a licensed customs broker or a CBP binding ruling must confirm them.

Product:
%s`, target, target, product)
		return &mcp.GetPromptResult{Description: "Plan a production move", Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: text}}}}, nil
	})
}
