package mcpserver

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// The media tool turns its three input shapes into one multipart upload with
// the REST route's field names, and refuses what the route would refuse
// before calling it.
func TestExtractProductFromMedia(t *testing.T) {
	type part struct{ field, ctype, data string }
	var parts []part
	calls := 0
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || r.URL.Path != "/v1/products/extract-media" {
			t.Errorf("request %s %q", r.URL.Path, r.Header.Get("Content-Type"))
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(p)
			parts = append(parts, part{p.FormName(), p.Header.Get("Content-Type"), string(b)})
		}
		_, _ = w.Write([]byte(`{"product":{"name":"Simulated draft","components":[{"name":"motor","made_in":"China"}]},"warnings":["check the motor"],"unreadable":[{"file":1}],"simulated":true}`))
	})
	cs := connect(t, api, false, nil)
	// Not listed unless the route is served.
	lt, _ := cs.ListTools(t.Context(), nil)
	for _, tool := range lt.Tools {
		if tool.Name == "extract_product_from_media" {
			t.Error("listed without MediaExtract")
		}
	}
	cs = connectOpts(t, Options{API: api, MediaExtract: true})

	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	res, txt, out := call(t, cs, "extract_product_from_media", map[string]any{
		"files": []any{
			map[string]any{"data": enc("PNGBYTES"), "mime_type": "image/png"},
			map[string]any{"type": "image", "data": "data:image/jpeg;base64," + enc("JPEGBYTES"), "mimeType": "image/jpeg"},
			map[string]any{"type": "resource", "resource": map[string]any{"uri": "file:///bom.pdf", "mimeType": "application/pdf", "blob": enc("%PDF-1.4")}},
		},
		"family": "motorised_tools_small_appliances", "name": "Heat gun",
	})
	if res.IsError {
		t.Fatal(txt)
	}
	got := map[string]string{}
	var files []part
	for _, p := range parts {
		if p.field == "file" {
			files = append(files, p)
		} else {
			got[p.field] = p.data
		}
	}
	if len(files) != 3 || files[0].data != "PNGBYTES" || files[1].data != "JPEGBYTES" || files[1].ctype != "image/jpeg" || files[2].data != "%PDF-1.4" || files[2].ctype != "application/pdf" {
		t.Errorf("files = %+v", files)
	}
	if got["family"] != "motorised_tools_small_appliances" || got["name"] != "Heat gun" {
		t.Errorf("fields = %v", got)
	}
	if !strings.HasPrefix(txt, "SIMULATED") || !strings.Contains(txt, "1 places could not be read") || !strings.HasSuffix(txt, Disclaimer) || out["simulated"] != true {
		t.Errorf("summary: %s", txt)
	}

	calls = 0
	big := strings.Repeat("A", mediaMaxB64) // 8 MB decoded
	for name, args := range map[string]map[string]any{
		"five files":    {"files": []any{f(enc("a")), f(enc("a")), f(enc("a")), f(enc("a")), f(enc("a"))}},
		"no files":      {"files": []any{}},
		"bad type":      {"files": []any{map[string]any{"data": enc("x"), "mime_type": "image/gif"}}},
		"bad base64":    {"files": []any{f("!!!!not base64")}},
		"over 8 MB":     {"files": []any{f(big), f(enc("more"))}},
		"mixed shapes":  {"files": []any{map[string]any{"data": enc("x"), "mime_type": "image/png", "mimeType": "image/png"}}},
		"unknown field": {"files": []any{f(enc("x"))}, "facts": "x"},
	} {
		if res, txt, _ := call(t, cs, "extract_product_from_media", args); !res.IsError {
			t.Errorf("%s accepted: %s", name, txt)
		}
	}
	if calls != 0 {
		t.Errorf("%d refused calls reached the API", calls)
	}
}

func f(data string) map[string]any { return map[string]any{"data": data, "mime_type": "image/png"} }
