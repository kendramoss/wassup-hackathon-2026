package mcpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Limits of POST /v1/products/extract-media (internal/media), repeated here
// so a call that cannot pass them is refused before any API call. The API
// checks them again from the bytes.
const (
	mediaMaxFiles = 4
	mediaMaxBytes = 8 << 20 // all files together, decoded
	// mediaMaxB64 is the base64 length of mediaMaxBytes.
	mediaMaxB64 = (mediaMaxBytes + 2) / 3 * 4
)

var mediaTypes = []string{"image/png", "image/jpeg", "image/webp", "application/pdf"}

// A file is accepted in three shapes: plain {data, mime_type}, an MCP image
// content block {type: "image", data, mimeType}, or an MCP embedded resource
// {type: "resource", resource: {uri, mimeType, blob}}.
type mediaFile struct {
	Type     string         `json:"type,omitempty"`
	Data     string         `json:"data,omitempty"`
	MIME     string         `json:"mime_type,omitempty"`
	MimeType string         `json:"mimeType,omitempty"`
	Resource *mediaResource `json:"resource,omitempty"`
}

type mediaResource struct {
	URI      string `json:"uri,omitempty"`
	MimeType string `json:"mimeType"`
	Blob     string `json:"blob"`
}

type mediaIn struct {
	Files  []mediaFile `json:"files"`
	Family string      `json:"family,omitempty"`
	Name   string      `json:"name,omitempty"`
}

func (f mediaFile) payload() (data, mime string) {
	switch {
	case f.Resource != nil:
		return f.Resource.Blob, f.Resource.MimeType
	case f.MimeType != "":
		return f.Data, f.MimeType
	}
	return f.Data, f.MIME
}

func (t tools) mediaSchema() *jsonschema.Schema {
	b64 := func() *jsonschema.Schema {
		return str("The file's bytes, base64-encoded.", 4, mediaMaxB64)
	}
	types := func(desc string) *jsonschema.Schema { return enum(desc, mediaTypes...) }
	file := &jsonschema.Schema{
		Description: "One file: {data, mime_type}, or an MCP image content block, or an MCP embedded resource with a blob.",
		OneOf: []*jsonschema.Schema{
			object("A base64 file.", []string{"data", "mime_type"}, map[string]*jsonschema.Schema{
				"data": b64(), "mime_type": types("The file type."),
			}),
			object("An MCP image content block.", []string{"type", "data", "mimeType"}, map[string]*jsonschema.Schema{
				"type": enum("", "image"), "data": b64(), "mimeType": types("The image type."),
			}),
			object("An MCP embedded resource.", []string{"type", "resource"}, map[string]*jsonschema.Schema{
				"type": enum("", "resource"),
				"resource": object("", []string{"mimeType", "blob"}, map[string]*jsonschema.Schema{
					"uri": str("", 0, 2000), "mimeType": types("The file type."), "blob": b64(),
				}),
			}),
		},
	}
	return object("", []string{"files"}, map[string]*jsonschema.Schema{
		"files":  array(fmt.Sprintf("1 to %d files, %d MB together: PNG, JPEG or WebP images, or scanned PDFs (10 pages together).", mediaMaxFiles, mediaMaxBytes>>20), file, 1, mediaMaxFiles),
		"family": t.family(),
		"name":   str("Product name; defaults to the name printed in the files.", 1, 200),
	})
}

func (t tools) registerMedia(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name: "extract_product_from_media",
		Description: `Draft a structured product (parts, plants, operations) from 1 to 4 images or scanned PDFs: a photo or screenshot of a bill of materials, a scanned BOM or spec sheet, a product photo with its spec sheet. Nothing is saved and nothing is predicted.
Use it when the user attaches a document or image instead of typing the parts. Input: files (base64 with a type, or MCP image or resource blocks), optional family and name. Limits: 8 MB together; images up to 12,000 pixels a side; PDFs up to 10 pages together.
Returns the same draft as extract_product, with each row citing the file and page it was read from, rows marked unclear, and places that could not be read. A person must review the draft before predict_origin. REST: POST /v1/products/extract-media (operationId extractProductFromMedia).`,
		InputSchema: t.mediaSchema(),
		Annotations: readOnly("Extract a product from images or PDFs", false),
	}, t.extractMedia)
}

func (t tools) extractMedia(ctx context.Context, hdr http.Header, in mediaIn) (*mcp.CallToolResult, any, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	total := 0
	for i, f := range in.Files {
		data, mime := f.payload()
		b, err := decodeBase64(data)
		if err != nil {
			return toolError(fmt.Sprintf("file %d is not valid base64", i+1))
		}
		total += len(b)
		if total > mediaMaxBytes {
			return toolError(fmt.Sprintf("the files are larger than %d MB together", mediaMaxBytes>>20))
		}
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="file%d"`, i+1))
		h.Set("Content-Type", mime)
		w, err := mw.CreatePart(h)
		if err != nil {
			return nil, nil, err
		}
		_, _ = w.Write(b)
		clear(b)
	}
	for k, v := range map[string]string{"family": in.Family, "name": in.Name} {
		if v != "" {
			_ = mw.WriteField(k, v)
		}
	}
	if err := mw.Close(); err != nil {
		return nil, nil, err
	}
	res, err := t.call(ctx, hdr, apiCall{method: "POST", path: "/v1/products/extract-media", raw: buf.Bytes(), contentType: mw.FormDataContentType()})
	clear(buf.Bytes())
	if err != nil || !res.ok() {
		return failed(res, err)
	}
	out := trim(res.body)
	pr := obj(out["product"])
	var b strings.Builder
	if out["simulated"] == true {
		b.WriteString("SIMULATED: the server runs the local fake model, which did not read your files.\n")
	}
	fmt.Fprintf(&b, "Draft product %q from %d files: %d parts, %d plants, %d operations.", str0(pr["name"], ""), len(in.Files), len(list(pr["components"])), len(list(out["plants"])), len(list(pr["operations"])))
	if u := list(out["unreadable"]); len(u) > 0 {
		fmt.Fprintf(&b, "\n%d places could not be read (see unreadable).", len(u))
	}
	for _, w := range list(out["warnings"]) {
		fmt.Fprintf(&b, "\nWarning: %v", w)
	}
	b.WriteString("\nA person must review and edit this draft, then pass it as product to predict_origin.")
	return done(b.String(), out)
}

// decodeBase64 accepts standard or URL-safe base64, padded or not, with
// whitespace, and a data: URL prefix.
func decodeBase64(s string) ([]byte, error) {
	if i := strings.Index(s, ";base64,"); strings.HasPrefix(s, "data:") && i > 0 {
		s = s[i+len(";base64,"):]
	}
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimRight(s, "=")
	if strings.ContainsAny(s, "-_") {
		return base64.RawURLEncoding.DecodeString(s)
	}
	return base64.RawStdEncoding.DecodeString(s)
}

func toolError(msg string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}, map[string]any{"error": map[string]any{"code": "invalid_input", "message": msg}}, nil
}
