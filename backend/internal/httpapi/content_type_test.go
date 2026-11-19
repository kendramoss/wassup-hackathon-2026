package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

// multipartForm builds a form body with a file field and optional fields. The
// boundary is returned so a test can exercise any casing of the media type
// while keeping the parameter value exactly as the writer produced it.
func multipartForm(t *testing.T, fields map[string]string, filename, content string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), mw.Boundary()
}

// Media types are case-insensitive; only parameter values such as the
// boundary keep their casing (#325). All three upload endpoints must take the
// multipart branch for every casing variant.
func TestUploadsAcceptCaseInsensitiveMultipartContentType(t *testing.T) {
	batchCSV := "SKU,Description,Date\nG-1,\"Knit polyester gaiters, knit in China\",2024-03-01\n,,\n"
	bomCSV := "Part,Made in,Factory,Operation group,Plant capabilities\n" +
		"main housing,Thailand,Rayong plant,housing_moulding,motor_fan_assembly\n" +
		"electric motor,China,China suppliers,motor_fan_assembly,\n"

	for _, mediaType := range []string{"multipart/form-data", "Multipart/Form-Data", "MULTIPART/FORM-DATA"} {
		t.Run("batch/"+mediaType, func(t *testing.T) {
			hs := newHarness(t, "textile_102_21")
			body, boundary := multipartForm(t, nil, "portfolio.csv", batchCSV)
			req := httptest.NewRequest("POST", "/v1/cases/batch", bytes.NewReader(body))
			req.Header.Set("Content-Type", mediaType+"; boundary="+boundary)
			rec := httptest.NewRecorder()
			hs.h.ServeHTTP(rec, req)
			var out map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("status %d, body %q: %v", rec.Code, rec.Body.String(), err)
			}
			if rec.Code != http.StatusOK || out["rows"] == nil {
				t.Fatalf("status %d, body %v; want a parsed batch", rec.Code, out)
			}
		})

		t.Run("import/"+mediaType, func(t *testing.T) {
			hs := newHarness(t, "substantial_transformation")
			fields := map[string]string{"name": "Heat gun", "family": "motorised_tools_small_appliances"}
			body, boundary := multipartForm(t, fields, "bom.csv", bomCSV)
			req := httptest.NewRequest("POST", "/v1/products/import", bytes.NewReader(body))
			req.Header.Set("Content-Type", mediaType+"; boundary="+boundary)
			rec := httptest.NewRecorder()
			hs.h.ServeHTTP(rec, req)
			var out map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("status %d, body %q: %v", rec.Code, rec.Body.String(), err)
			}
			if rec.Code != http.StatusOK || out["product"] == nil {
				t.Fatalf("status %d, body %v; want an imported product", rec.Code, out)
			}
		})

		t.Run("bom-version/"+mediaType, func(t *testing.T) {
			hs := bomHarness(t, nil)
			_, c := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t))
			id := c["id"].(string)
			body, boundary := multipartForm(t, nil, "bom-v2.csv", motorInThailand(t))
			req := httptest.NewRequest("POST", "/v1/cases/"+id+"/bom-version", bytes.NewReader(body))
			req.Header.Set("Content-Type", mediaType+"; boundary="+boundary)
			rec := httptest.NewRecorder()
			hs.h.ServeHTTP(rec, req)
			var out map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("status %d, body %q: %v", rec.Code, rec.Body.String(), err)
			}
			if rec.Code != http.StatusOK || out["changes"] == nil {
				t.Fatalf("status %d, body %v; want a BOM change", rec.Code, out)
			}
		})
	}
}

func TestMalformedMultipartContentTypeReturnsJSONError(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	req := httptest.NewRequest("POST", "/v1/products/import", bytes.NewBufferString("--broken--"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary")
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	var out ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("status %d, body %q: %v", rec.Code, rec.Body.String(), err)
	}
	if rec.Code != http.StatusBadRequest || out.Code != "invalid_input" {
		t.Fatalf("status %d, body %+v; want a 400 invalid_input", rec.Code, out)
	}
}
