package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/spend"
)

func withKeys(d *Deps) { d.APIKeys = apikey.New(apikey.NewMemory()) }

// issueKey issues a key through the admin endpoint and returns its secret and ID.
func issueKey(t *testing.T, hs harness, name string, quota int) (string, string) {
	t.Helper()
	code, out := hs.doAs(t, "POST", "/v1/admin/api-keys", `{"name":"`+name+`","daily_quota":`+strconv.Itoa(quota)+`}`, admin)
	if code != http.StatusCreated {
		t.Fatalf("issue = %d %v", code, out)
	}
	return out["key"].(string), out["api_key"].(map[string]any)["id"].(string)
}

func TestAPIKeysIssueListDisable(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", withKeys)
	if code, _ := hs.do(t, "POST", "/v1/admin/api-keys", `{"name":"Acme","daily_quota":10}`); code != http.StatusForbidden {
		t.Errorf("issue without the admin token = %d, want 403", code)
	}
	for _, body := range []string{`{"name":"","daily_quota":10}`, `{"name":"Acme","daily_quota":0}`, `{"name":"Acme","daily_quota":10,"extra":1}`} {
		if code, _ := hs.doAs(t, "POST", "/v1/admin/api-keys", body, admin); code != http.StatusBadRequest {
			t.Errorf("issue %s = %d, want 400", body, code)
		}
	}
	secret, id := issueKey(t, hs, "Acme Customs", 10)
	if !strings.HasPrefix(secret, "op_") || len(secret) != 46 {
		t.Errorf("secret = %q", secret)
	}

	code, out := hs.doAs(t, "GET", "/v1/admin/api-keys", "", admin)
	keys, _ := out["api_keys"].([]any)
	if code != http.StatusOK || len(keys) != 1 {
		t.Fatalf("list = %d %v", code, out)
	}
	k := keys[0].(map[string]any)
	if k["id"] != id || k["name"] != "Acme Customs" || k["daily_quota"] != 10.0 || k["disabled"] != false || k["created_at"] == nil {
		t.Errorf("listed key = %v", k)
	}
	for field := range k {
		if field != "id" && field != "name" && field != "daily_quota" && field != "disabled" && field != "created_at" {
			t.Errorf("list returns %q", field)
		}
	}
	if code, _ := hs.do(t, "GET", "/v1/admin/api-keys", ""); code != http.StatusForbidden {
		t.Errorf("list without the admin token = %d", code)
	}

	key := map[string]string{"X-API-Key": secret}
	if code, _ := hs.doAs(t, "GET", "/v1/cases", "", key); code != http.StatusOK {
		t.Errorf("valid key = %d", code)
	}
	if code, _ := hs.do(t, "DELETE", "/v1/admin/api-keys/"+id, ""); code != http.StatusForbidden {
		t.Errorf("disable without the admin token = %d", code)
	}
	if code, _ := hs.doAs(t, "DELETE", "/v1/admin/api-keys/"+id, "", admin); code != http.StatusNoContent {
		t.Errorf("disable = %d", code)
	}
	if code, _ := hs.doAs(t, "DELETE", "/v1/admin/api-keys/0123456789abcdef", "", admin); code != http.StatusNotFound {
		t.Errorf("disable unknown = %d, want 404", code)
	}
	if code, out := hs.doAs(t, "GET", "/v1/cases", "", key); code != http.StatusUnauthorized || out["code"] != "unauthorized" {
		t.Errorf("disabled key = %d %v, want 401", code, out)
	}
	_, out = hs.doAs(t, "GET", "/v1/admin/api-keys", "", admin)
	if out["api_keys"].([]any)[0].(map[string]any)["disabled"] != true {
		t.Errorf("disabled key not shown as disabled: %v", out)
	}
}

func TestAPIKeyInvalidIs401(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", withKeys)
	for _, bad := range []string{"op_nope", "op_" + strings.Repeat("A", 43), "Bearer something"} {
		code, out := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), map[string]string{"X-API-Key": bad})
		if code != http.StatusUnauthorized || out["code"] != "unauthorized" {
			t.Errorf("key %q = %d %v, want 401", bad, code, out)
		}
	}
	// No key: anonymous access as before.
	if code, _ := hs.do(t, "GET", "/v1/cases", ""); code != http.StatusOK {
		t.Errorf("no key = %d", code)
	}
	if code, _ := hs.do(t, "GET", "/health", ""); code != http.StatusOK {
		t.Errorf("health = %d", code)
	}
}

func TestAPIKeysOffByDefault(t *testing.T) {
	hs := newHarness(t, "substantial_transformation")
	if code, _ := hs.doAs(t, "POST", "/v1/admin/api-keys", `{"name":"Acme","daily_quota":10}`, admin); code != http.StatusNotFound {
		t.Errorf("admin endpoint with keys off = %d, want 404", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-API-Key": "op_whatever"}); code != http.StatusOK {
		t.Errorf("X-API-Key with keys off = %d, want it ignored", code)
	}
}

func TestAPIKeyDailyQuota(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", withKeys)
	secret, _ := issueKey(t, hs, "Acme", 2)
	other, _ := issueKey(t, hs, "Beta", 2)
	key := map[string]string{"X-API-Key": secret}
	for i := 0; i < 2; i++ {
		if code, _ := hs.doAs(t, "GET", "/v1/cases", "", key); code != http.StatusOK {
			t.Fatalf("request %d = %d", i, code)
		}
	}
	req := httptest.NewRequest("GET", "/v1/cases", nil)
	req.Header.Set("X-API-Key", secret)
	rec := httptest.NewRecorder()
	hs.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), `"quota_exceeded"`) {
		t.Errorf("over quota = %d %s", rec.Code, rec.Body)
	}
	if secs, err := strconv.Atoi(rec.Header().Get("Retry-After")); err != nil || secs < 1 || secs > 86400 {
		t.Errorf("Retry-After = %q, want seconds until the next UTC day", rec.Header().Get("Retry-After"))
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases", "", map[string]string{"X-API-Key": other}); code != http.StatusOK {
		t.Errorf("another key's quota was used: %d", code)
	}
	if code, _ := hs.do(t, "GET", "/v1/cases", ""); code != http.StatusOK {
		t.Errorf("anonymous requests hit the key's quota: %d", code)
	}
}

func TestAPIKeyOwnsItsCases(t *testing.T) {
	hs := newHarnessWith(t, "substantial_transformation", withKeys) // CASE_OWNERSHIP off
	acme, _ := issueKey(t, hs, "Acme", 100)
	beta, _ := issueKey(t, hs, "Beta", 100)
	ka, kb := map[string]string{"X-API-Key": acme}, map[string]string{"X-API-Key": beta}
	_, a := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), ka)
	_, v := hs.do(t, "POST", "/v1/cases/predict", heatGunBody(t)) // an anonymous visitor
	_, d := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), admin)
	aid, vid, did := a["id"].(string), v["id"].(string), d["id"].(string)
	hs.doAs(t, "PUT", "/v1/cases/"+did+"/pin", `{"pinned":true}`, admin)
	if _, ok := a["owner"]; ok {
		t.Error("the owner hash is returned to the client")
	}

	ids := func(h map[string]string) map[string]bool {
		_, out := hs.doAs(t, "GET", "/v1/cases", "", h)
		m := map[string]bool{}
		for _, c := range out["cases"].([]any) {
			m[c.(map[string]any)["id"].(string)] = true
		}
		return m
	}
	if got := ids(ka); !got[aid] || got[vid] || !got[did] || len(got) != 2 {
		t.Errorf("acme sees %v; want its case and the demo", got)
	}
	if got := ids(kb); got[aid] || got[vid] || !got[did] || len(got) != 1 {
		t.Errorf("beta sees %v; want the demo only", got)
	}
	if got := ids(nil); got[aid] || !got[vid] || !got[did] {
		t.Errorf("a visitor sees %v; the key's case must stay hidden even with ownership off", got)
	}
	if got := ids(admin); !got[aid] || !got[vid] || !got[did] {
		t.Errorf("admin sees %v", got)
	}
	for _, h := range []map[string]string{kb, nil, {"X-Visitor-ID": "some-visitor-id-0001"}} {
		if code, _ := hs.doAs(t, "GET", "/v1/cases/"+aid, "", h); code != http.StatusNotFound {
			t.Errorf("%v opens acme's case: %d, want 404", h, code)
		}
	}
	if code, _ := hs.doAs(t, "DELETE", "/v1/cases/"+aid, "", kb); code != http.StatusNotFound {
		t.Errorf("beta deletes acme's case: %d", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+vid, "", ka); code != http.StatusNotFound {
		t.Errorf("a key opens a visitor's case: %d, want 404", code)
	}
	if code, _ := hs.doAs(t, "GET", "/v1/cases/"+aid, "", ka); code != http.StatusOK {
		t.Errorf("acme can't open its case: %d", code)
	}
	if code, _ := hs.doAs(t, "DELETE", "/v1/cases/"+aid, "", ka); code != http.StatusNoContent {
		t.Errorf("acme can't delete its case: %d", code)
	}
}

func TestAPIKeyHasItsOwnBucketsAndCountsTowardSpend(t *testing.T) {
	meter := spend.New(spend.NewMemory(), 0.01)
	hs := newHarnessWith(t, "substantial_transformation", func(d *Deps) {
		withKeys(d)
		d.Limits = Limits{LightPerMin: 1, HeavyPerMin: 1, GlobalPerMin: 1000}
		d.Spend = meter
	})
	acme, _ := issueKey(t, hs, "Acme", 100)
	beta, _ := issueKey(t, hs, "Beta", 100)
	ip := map[string]string{"X-Client-IP": "198.51.100.7"}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), ip); code != http.StatusCreated {
		t.Fatalf("first anonymous predict = %d", code)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), ip); code != http.StatusTooManyRequests {
		t.Fatalf("second anonymous predict = %d, want 429", code)
	}
	// Same address, with keys: each key has its own bucket, not the address's.
	withIP := func(secret string) map[string]string {
		return map[string]string{"X-Client-IP": "198.51.100.7", "X-API-Key": secret}
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), withIP(acme)); code != http.StatusCreated {
		t.Errorf("acme's first predict = %d", code)
	}
	code, out := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), map[string]string{"X-Client-IP": "203.0.113.9", "X-API-Key": acme})
	if code != http.StatusTooManyRequests || out["code"] != "rate_limited" {
		t.Errorf("acme from another address = %d %v; the key's bucket should follow the key", code, out)
	}
	if code, _ := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), withIP(beta)); code != http.StatusCreated {
		t.Errorf("beta's first predict = %d", code)
	}
	meter.Add(context.Background(), 0.02) // the day's budget is spent
	gamma, _ := issueKey(t, hs, "Gamma", 100)
	if code, out := hs.doAs(t, "POST", "/v1/cases/predict", heatGunBody(t), map[string]string{"X-API-Key": gamma}); code != http.StatusServiceUnavailable || out["code"] != "budget_reached" {
		t.Errorf("key over the spend cap = %d %v, want 503 budget_reached", code, out)
	}
}
