package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/extract"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/media"
)

// secret is a marker written into every test image (a PNG text chunk) so a
// test can prove the bytes never reach a log line or the response.
const secret = "BOM-SECRET-7f3a"

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.White)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	// Insert a tEXt-like private chunk before IEND carrying the marker; the
	// header still decodes.
	iend := bytes.LastIndex(b, []byte("IEND")) - 4
	chunk := append([]byte{0, 0, 0, byte(len(secret))}, append([]byte("prVt"), secret...)...)
	chunk = append(chunk, 0, 0, 0, 0) // CRC is not checked by DecodeConfig
	return append(append(append([]byte{}, b[:iend]...), chunk...), b[iend:]...)
}

func testPDF(pages int) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	var refs []string
	for i := 0; i < pages; i++ {
		refs = append(refs, fmt.Sprintf("%d 0 R", i+3))
	}
	fmt.Fprintf(&b, "1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n")
	fmt.Fprintf(&b, "2 0 obj << /Type /Pages /Kids [%s] /Count %d >> endobj\n", strings.Join(refs, " "), pages)
	for i := 0; i < pages; i++ {
		fmt.Fprintf(&b, "%d 0 obj << /Type /Page /Parent 2 0 R >> endobj\n", i+3)
	}
	b.WriteString("%%EOF\n")
	return []byte(b.String())
}

// testPDFEscapedPages is the #381 reproduction: the page tree and 40 pages
// use PDF's #xx name escapes, plus a loose /Type /Page in a comment.
func testPDFEscapedPages(pages int) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.5\n% /Type /Page\n")
	var refs []string
	for i := 0; i < pages; i++ {
		refs = append(refs, fmt.Sprintf("%d 0 R", i+3))
	}
	b.WriteString("1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n")
	fmt.Fprintf(&b, "2 0 obj << /Type /P#61ges /Kids [%s] /Count %d >> endobj\n", strings.Join(refs, " "), pages)
	for i := 0; i < pages; i++ {
		fmt.Fprintf(&b, "%d 0 obj << /Type /P#61ge /Parent 2 0 R >> endobj\n", i+3)
	}
	b.WriteString("%%EOF\n")
	return []byte(b.String())
}

type upload struct {
	name, contentType string
	data              []byte
}

func mediaBody(t *testing.T, files []upload, fields map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, f := range files {
		h := make(map[string][]string)
		h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename=%q`, f.name)}
		h["Content-Type"] = []string{f.contentType}
		w, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(f.data)
	}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	return buf.Bytes(), mw.FormDataContentType()
}

// mediaHarness wires the real extractor over a scripted fake model, so the
// handler, the checks and the draft mapping run together.
func mediaHarness(t *testing.T, answer string) (harness, *llm.Fake) {
	t.Helper()
	fake := &llm.Fake{Respond: func(llm.Request) (llm.Response, error) {
		return llm.Response{Text: answer, FinishReason: "STOP", Usage: llm.Usage{InputTokens: 1500, OutputTokens: 200}}, nil
	}}
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		lg := harnessLogger
		if lg == nil {
			lg = slog.New(suiteHandler)
		}
		d.MediaExtractor = extract.New(llm.WithLogging(fake, lg), "gemini-4-argon")
	})
	return hs, fake
}

func postMedia(t *testing.T, hs harness, body []byte, contentType string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("POST", "/v1/products/extract-media", bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

const readAnswer = `{"product":"Heat gun","components":[
{"name":"main housing","country":"Thailand","group":"housing_moulding","file":1,"page":1,"legible":true},
{"name":"fan","country":"China","group":"motor_fan_assembly","file":2,"page":2,"legible":false},
{"name":"screw","country":"","group":"","file":1,"page":1,"legible":true}],
"operations":[{"name":"final assembly","country":"Thailand","group":"final_assembly","file":1,"page":1,"legible":true}],
"plants":[],"stated_component_count":null,
"unreadable":[{"file":2,"page":2,"what":"row 7, the country is cut off"},{"file":9,"page":1,"what":"not a file"}],"notes":[]}`

func TestExtractMediaDraftsFromImageAndPDF(t *testing.T) {
	logs := useLogger(t, plainJSON)
	hs, fake := mediaHarness(t, readAnswer)
	body, ct := mediaBody(t, []upload{{"bom.png", "image/png", testPNG(t, 64, 48)}, {"spec.pdf", "application/pdf", testPDF(2)}},
		map[string]string{"family": "motorised_tools_small_appliances"})
	code, out := postMedia(t, hs, body, ct)
	if code != 200 {
		t.Fatalf("status %d: %v", code, out)
	}
	if out["source"] != "media" || out["product"].(map[string]any)["name"] != "Heat gun" {
		t.Errorf("draft = %v", out)
	}
	comps := out["components"].([]any)
	if len(comps) != 2 {
		t.Fatalf("components = %v (the screw has no country and must not be guessed)", comps)
	}
	fan := comps[1].(map[string]any)
	if fan["source_file"] != 2.0 || fan["source_page"] != 2.0 || fan["unclear"] != true || fan["source_paragraph"] != 0.0 {
		t.Errorf("fan provenance = %v", fan)
	}
	files := out["files"].([]any)
	if len(files) != 2 || files[0].(map[string]any)["width"] != 64.0 || files[1].(map[string]any)["pages"] != 2.0 {
		t.Errorf("files = %v", files)
	}
	unread := out["unreadable"].([]any)
	if len(unread) != 1 || unread[0].(map[string]any)["what"] != "row 7, the country is cut off" {
		t.Errorf("unreadable = %v (a citation to a file not sent is dropped)", unread)
	}
	ws := fmt.Sprint(out["warnings"])
	for _, want := range []string{"Hard to read, check against the document: fan.", "File 2, page 2: could not read row 7", "The document mentions screw"} {
		if !strings.Contains(ws, want) {
			t.Errorf("warnings %s miss %q", ws, want)
		}
	}
	if out["trace"].(map[string]any)["prompt_version"] != extract.MediaPromptVersion {
		t.Errorf("trace = %v", out["trace"])
	}
	// The model got the bytes as typed parts, after a text listing them.
	calls := fake.Calls()
	if len(calls) != 1 {
		t.Fatalf("%d model calls", len(calls))
	}
	msg := calls[0].Messages[0]
	if len(msg.Parts) != 2 || msg.Parts[0].MIMEType != media.PNG || msg.Parts[1].MIMEType != media.PDF ||
		!strings.Contains(msg.Content, "[2] PDF, 2 page(s)") || calls[0].SchemaName != extract.MediaSchemaName {
		t.Errorf("request = %q parts %d", msg.Content, len(msg.Parts))
	}
	// The bytes are wiped once the request is done.
	if bytes.Contains(msg.Parts[0].Data, []byte(secret)) {
		t.Error("the upload's bytes are still held after the response")
	}
	// No log line holds the content, the file names or the model's answer.
	for _, leak := range []string{secret, "bom.png", "spec.pdf", "main housing", "row 7"} {
		if strings.Contains(logs.String(), leak) || strings.Contains(fmt.Sprint(out), secret) {
			t.Errorf("%q leaked into the logs or response", leak)
		}
	}
	if !strings.Contains(logs.String(), `"route":"POST /v1/products/extract-media"`) {
		t.Errorf("request line missing: %s", logs.String())
	}
}

// Under the production allowlist the new route and its model call log with
// nothing masked (TestMain also fails on any masking in the suite).
func TestExtractMediaLogsUnderProductionPolicy(t *testing.T) {
	logs := useLogger(t, policyJSON)
	hs, _ := mediaHarness(t, readAnswer)
	body, ct := mediaBody(t, []upload{{"bom.png", "image/png", testPNG(t, 8, 8)}}, nil)
	if code, out := postMedia(t, hs, body, ct); code != 200 {
		t.Fatalf("status %d: %v", code, out)
	}
	if strings.Contains(logs.String(), "[masked]") || strings.Contains(logs.String(), "masked_attrs") || strings.Contains(logs.String(), secret) {
		t.Errorf("logs: %s", logs.String())
	}
}

func TestExtractMediaRejects(t *testing.T) {
	hs, fake := mediaHarness(t, readAnswer)
	big := bytes.Repeat([]byte{0}, media.MaxTotalBytes/2+1)
	bigPNG := append(testPNG(t, 4, 4), big...)
	tests := []struct {
		name  string
		files []upload
		ct    string
		want  int
		msg   string
	}{
		{"no files", nil, "", 400, "is required"},
		{"html named .png", []upload{{"x.png", "image/png", []byte("<html><body>hi</body></html>")}}, "", 415, "file 1"},
		{"csv", []upload{{"x.csv", "text/csv", []byte("name,country\nmotor,China\n")}}, "", 415, "PNG, JPEG or WebP"},
		{"empty file", []upload{{"x.png", "image/png", nil}}, "", 400, "empty"},
		{"bomb header", []upload{{"x.png", "image/png", pngBombHeader()}}, "", 400, "pixels"},
		{"too many files", []upload{{"a.png", "", testPNG(t, 2, 2)}, {"b.png", "", testPNG(t, 2, 2)}, {"c.png", "", testPNG(t, 2, 2)}, {"d.png", "", testPNG(t, 2, 2)}, {"e.png", "", testPNG(t, 2, 2)}}, "", 400, "at most 4"},
		{"too many pages", []upload{{"a.pdf", "application/pdf", testPDF(7)}, {"b.pdf", "application/pdf", testPDF(4)}}, "", 400, "11 pages"},
		{"40 escaped pages", []upload{{"a.pdf", "application/pdf", testPDFEscapedPages(40)}}, "", 400, "40 pages"},
		{"over 8 MB together", []upload{{"a.png", "image/png", bigPNG}, {"b.png", "image/png", bigPNG}}, "", 413, "8 MB"},
		{"unknown family", []upload{{"a.png", "image/png", testPNG(t, 2, 2)}}, "family", 400, "no template"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := map[string]string{}
			if tt.ct == "family" {
				fields["family"] = "nope"
			}
			body, ct := mediaBody(t, tt.files, fields)
			code, out := postMedia(t, hs, body, ct)
			if code != tt.want || !strings.Contains(fmt.Sprint(out["message"]), tt.msg) || out["code"] != "invalid_input" {
				t.Errorf("status %d %v; want %d with %q", code, out, tt.want, tt.msg)
			}
		})
	}
	if code, _ := postMedia(t, hs, []byte(`{"facts":"x"}`), "application/json"); code != http.StatusUnsupportedMediaType {
		t.Errorf("JSON body: %d, want 415", code)
	}
	body, ct := mediaBody(t, []upload{{"a.png", "image/png", testPNG(t, 2, 2)}}, map[string]string{"extra": "x"})
	if code, _ := postMedia(t, hs, body, ct); code != 400 {
		t.Errorf("unknown field: %d", code)
	}
	if n := len(fake.Calls()); n != 0 {
		t.Errorf("a rejected upload reached the model %d times", n)
	}
}

func pngBombHeader() []byte {
	// 60000 x 60000, valid IHDR CRC; no pixel data.
	return []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR\x00\x00\xea\x60\x00\x00\xea\x60\x08\x00\x00\x00\x00" + crcOf("IHDR\x00\x00\xea\x60\x00\x00\xea\x60\x08\x00\x00\x00\x00"))
}

// The fake model's fixed answer comes back labelled as simulated.
func TestExtractMediaSimulatedInFakeMode(t *testing.T) {
	hs, _ := mediaHarness(t, extract.SimulatedMediaAnswer)
	body, ct := mediaBody(t, []upload{{"a.jpg", "image/jpeg", testPNG(t, 4, 4)}}, map[string]string{"family": "motorised_tools_small_appliances"})
	code, out := postMedia(t, hs, body, ct)
	if code != 200 || out["simulated"] != true || out["warnings"].([]any)[0] != extract.SimulatedNote ||
		!strings.Contains(out["product"].(map[string]any)["name"].(string), "Simulated") {
		t.Fatalf("status %d: %v", code, out)
	}
	if out["files"].([]any)[0].(map[string]any)["mime_type"] != media.PNG {
		t.Error("the type must come from the bytes, not the declared image/jpeg")
	}
}

func crcOf(s string) string {
	c := crc32.ChecksumIEEE([]byte(s))
	return string([]byte{byte(c >> 24), byte(c >> 16), byte(c >> 8), byte(c)})
}
