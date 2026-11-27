package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Supplier origin declarations (#916, D937): a link like the broker review
// link (D200), per part answers, and the declared origin as a sourced fact.

// memBlobs is an in-memory blob store for the attached documents.
type memBlobs struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (b *memBlobs) Get(_ context.Context, k string) ([]byte, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.m[k]
	return v, ok, nil
}

func (b *memBlobs) Put(_ context.Context, k string, v []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.m == nil {
		b.m = map[string][]byte{}
	}
	b.m[k] = append([]byte(nil), v...)
	return nil
}

type declEnv struct {
	*roleEnv
	docs *memBlobs
}

func newDeclEnv(t *testing.T, adjust ...func(*Deps)) *declEnv {
	t.Helper()
	de := &declEnv{docs: &memBlobs{}}
	de.roleEnv = newRoleEnv(t, true, append([]func(*Deps){func(d *Deps) { d.SupplierDeclarations, d.SupplierDocs = true, de.docs }}, adjust...)...)
	return de
}

func declTok(tok string) map[string]string { return map[string]string{declarationTokenHeader: tok} }

func today() time.Time { return time.Now().UTC().Truncate(24 * time.Hour) }

func dayIn(n int) string { return today().AddDate(0, 0, n).Format(time.DateOnly) }

// ask has dave (an editor) ask supplier "Acme Motors" about parts and returns
// the token and the request as the requester sees it.
func (e *declEnv) ask(id string, parts ...int) (string, map[string]any) {
	e.t.Helper()
	ps, _ := json.Marshal(parts)
	out := e.call("POST", "/v1/cases/"+id+"/supplier-declarations", `{"supplier_name":"Acme Motors","supplier_email":"sales@acme-motors.example","parts":`+string(ps)+`}`, user("dave"), http.StatusCreated)
	tok, _ := out["token"].(string)
	if !declarationTokenPattern.MatchString(tok) || out["declaration_path"] != "/supplier-declaration#"+tok {
		e.t.Fatalf("token %q path %v", tok, out["declaration_path"])
	}
	return tok, out["request"].(map[string]any)
}

func supplierAnswerJSON(origin string, validDays int, extra string) string {
	return fmt.Sprintf(`{"origin":%q,"hts":"8501.10","how_made":"Wound and assembled at our Rayong plant.","signer_name":"Somchai P.","signer_title":"Plant manager","valid_until":%q%s}`, origin, dayIn(validDays), extra)
}

func (e *declEnv) answer(tok string, part int, body string, want int) map[string]any {
	e.t.Helper()
	return e.call("PUT", fmt.Sprintf("/v1/supplier-declaration/parts/%d", part), body, declTok(tok), want)
}

func (e *declEnv) stored(id string) store.Case {
	e.t.Helper()
	c, err := e.cs.Get(context.Background(), id)
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

func (e *declEnv) edit(id string, f func(*store.Case)) {
	e.t.Helper()
	if _, err := e.cs.Update(context.Background(), id, func(c *store.Case) error { f(c); return nil }); err != nil {
		e.t.Fatal(err)
	}
}

func TestSupplierDeclarationsAreOffByDefault(t *testing.T) {
	hs := newHarness(t, "")
	id := hs.newCase(t, nil)
	for _, rt := range []struct{ m, p string }{
		{"POST", "/v1/cases/" + id + "/supplier-declarations"}, {"GET", "/v1/cases/" + id + "/supplier-declarations"},
		{"GET", "/v1/supplier-declarations/expiring"}, {"GET", "/v1/supplier-declaration"}, {"PUT", "/v1/supplier-declaration/parts/0"},
	} {
		if code, _ := hs.doAs(t, rt.m, rt.p, `{}`, nil); code != http.StatusNotFound && code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s with SUPPLIER_DECLARATIONS off = %d, want 404", rt.m, rt.p, code)
		}
	}
	// Teams on, the flag still off: the same.
	e := newRoleEnv(t, true, func(d *Deps) { d.SupplierDeclarations = false })
	if code, _ := e.hs.doAs(t, "GET", "/v1/supplier-declaration", "", declTok("usd_000000000000_"+strings.Repeat("A", 43))); code != http.StatusNotFound {
		t.Errorf("flag off, teams on = %d", code)
	}
	if got := e.call("GET", "/v1/cases/"+e.wsCase(), "", user("dave"), http.StatusOK); got["supplier_declarations"] != nil {
		t.Error("a case carries supplier_declarations with the flag off")
	}
}

func TestSupplierDeclarationLifecycle(t *testing.T) {
	e := newDeclEnv(t)
	id := e.wsCase()
	tok, rq := e.ask(id, 3, 4)
	if rq["status"] != "pending" || rq["supplier_name"] != "Acme Motors" || rq["supplier_email"] != "sales@acme-motors.example" {
		t.Errorf("new request = %v", rq)
	}
	created, _ := time.Parse(time.RFC3339Nano, rq["created_at"].(string))
	expires, _ := time.Parse(time.RFC3339Nano, rq["expires_at"].(string))
	if expires.Sub(created) != 14*24*time.Hour {
		t.Errorf("default life = %v", expires.Sub(created))
	}

	// Only the hash is stored.
	c := e.stored(id)
	blob, _ := json.Marshal(c)
	if strings.Contains(string(blob), tok) || strings.Contains(string(blob), tok[len(tok)-43:]) {
		t.Fatal("the token is stored in the case")
	}
	if len(c.SupplierRequests) != 1 || c.SupplierRequests[0].TokenHash != declarationTokenHash(tok) {
		t.Fatalf("stored request = %+v", c.SupplierRequests)
	}

	// The supplier sees the parts asked about and nothing else.
	rec := serve(e.hs, "GET", "/v1/supplier-declaration", "", declTok(tok))
	if rec.Code != http.StatusOK {
		t.Fatalf("supplier read = %d %s", rec.Code, rec.Body)
	}
	if rec.Header().Get("X-Robots-Tag") != "noindex, nofollow" || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("headers = %v", rec.Header())
	}
	body := rec.Body.String()
	for _, leak := range []string{id, "heat gun", "Kobalt", "main housing", "front cover", "heating element", "Thailand", "China", "token_hash", "sales@acme-motors.example", "supplier_email", "owner", "history", "workspace", "dave"} {
		if strings.Contains(body, leak) {
			t.Errorf("the supplier's view carries %q: %s", leak, body)
		}
	}
	var view struct {
		Request map[string]any `json:"request"`
		Parts   []struct {
			Index  int            `json:"index"`
			Name   string         `json:"name"`
			Answer map[string]any `json:"answer"`
		} `json:"parts"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if len(view.Parts) != 2 || view.Parts[0].Index != 3 || view.Parts[0].Name != "electric motor" || view.Parts[1].Name != "fan" || view.Request["supplier_name"] != "Acme Motors" {
		t.Fatalf("view = %s", body)
	}
	if got := e.call("GET", "/v1/cases/"+id+"/supplier-declarations", "", user("dave"), http.StatusOK)["requests"].([]any)[0].(map[string]any)["status"]; got != "viewed" {
		t.Errorf("after the first read = %v", got)
	}

	// Answers are checked.
	for name, b := range map[string]string{
		"no origin":     `{"how_made":"x","signer_name":"s","signer_title":"t","valid_until":"` + dayIn(30) + `"}`,
		"past date":     supplierAnswerJSON("Malaysia", -1, ""),
		"unknown field": supplierAnswerJSON("Malaysia", 30, `,"made_in":"China"`),
		"bad hts":       strings.Replace(supplierAnswerJSON("Malaysia", 30, ""), "8501.10", "motor", 1),
		"no signer":     strings.Replace(supplierAnswerJSON("Malaysia", 30, ""), `"Somchai P."`, `""`, 1),
		"not json":      `origin=Malaysia`,
	} {
		if got := e.answer(tok, 3, b, http.StatusBadRequest); got["code"] != "invalid_input" {
			t.Errorf("%s = %v", name, got)
		}
	}
	// A part that was not asked about is not reachable, though it exists.
	if got := e.answer(tok, 0, supplierAnswerJSON("Malaysia", 30, ""), http.StatusNotFound); got["code"] != "part_not_found" {
		t.Errorf("part 0 = %v", got)
	}
	e.answer(tok, 99, supplierAnswerJSON("Malaysia", 30, ""), http.StatusNotFound)

	// A good answer is stored on the part, with who and when.
	out := e.answer(tok, 3, supplierAnswerJSON("malaysia", 90, ""), http.StatusOK)
	if p := out["part"].(map[string]any); p["answer"].(map[string]any)["origin"] != "Malaysia" {
		t.Errorf("answer = %v", out)
	}
	c = e.stored(id)
	a := c.SupplierRequests[0].Answers
	if len(a) != 1 || a[0].Part != 3 || a[0].PartName != "electric motor" || a[0].Origin != "Malaysia" || a[0].SignerName != "Somchai P." || a[0].AnsweredAt.IsZero() || a[0].ValidUntil != dayIn(90) {
		t.Fatalf("stored answers = %+v", a)
	}
	if c.Product.Components[3].MadeIn != "China" {
		t.Fatal("the user's value was overwritten")
	}

	// The requester sees both values side by side, and the request half answered.
	list := e.call("GET", "/v1/cases/"+id+"/supplier-declarations", "", user("dave"), http.StatusOK)
	if list["requests"].([]any)[0].(map[string]any)["status"] != "partly_answered" {
		t.Errorf("request = %v", list["requests"])
	}
	parts := list["parts"].([]any)
	if len(parts) != 1 {
		t.Fatalf("parts = %v", parts)
	}
	pt := parts[0].(map[string]any)
	if pt["part"] != "electric motor" || pt["user_made_in"] != "China" || pt["declared_origin"] != "Malaysia" || pt["differs"] != true || pt["status"] != "valid" ||
		pt["source"] != "supplier-declared" || pt["supplier"] != "Acme Motors" || pt["declared_on"] != today().Format(time.DateOnly) || pt["valid_until"] != dayIn(90) || pt["signer_name"] != "Somchai P." {
		t.Errorf("part fact = %v", pt)
	}
	// The case answer carries it too.
	got := e.call("GET", "/v1/cases/"+id, "", user("dave"), http.StatusOK)
	sd, _ := got["supplier_declarations"].(map[string]any)
	if sd == nil || len(sd["parts"].([]any)) != 1 || sd["parts"].([]any)[0].(map[string]any)["declared_origin"] != "Malaysia" {
		t.Errorf("case supplier_declarations = %v", got["supplier_declarations"])
	}
	if got["facts"] != c.Facts {
		t.Error("facts changed by an answer alone")
	}

	// The second part completes the request; a later answer replaces the first.
	e.answer(tok, 4, supplierAnswerJSON("Malaysia", 60, ""), http.StatusOK)
	if got := e.call("GET", "/v1/cases/"+id+"/supplier-declarations", "", user("dave"), http.StatusOK)["requests"].([]any)[0].(map[string]any)["status"]; got != "answered" {
		t.Errorf("both parts answered = %v", got)
	}
	e.answer(tok, 3, supplierAnswerJSON("Vietnam", 45, ""), http.StatusOK)
	list = e.call("GET", "/v1/cases/"+id+"/supplier-declarations", "", user("dave"), http.StatusOK)
	for _, p := range list["parts"].([]any) {
		if p := p.(map[string]any); p["part"] == "electric motor" && (p["declared_origin"] != "Vietnam" || p["valid_until"] != dayIn(45)) {
			t.Errorf("the newer answer is not in effect: %v", p)
		}
	}
	if len(e.stored(id).SupplierRequests[0].Answers) != 3 {
		t.Error("answers are history: none is dropped")
	}
}

func TestDeclaredOriginFeedsThePredictInput(t *testing.T) {
	e := newDeclEnv(t)
	id := e.wsCase()
	tok, _ := e.ask(id, 3)
	e.answer(tok, 3, supplierAnswerJSON("Malaysia", 90, ""), http.StatusOK)

	reask := func() {
		e.call("POST", "/v1/cases/"+id+"/answers", `{"answers":[{"question":"Is the motor made in China?","answer":"Yes, the motor is made in China."}]}`, user("dave"), http.StatusOK)
	}
	reask()
	want := fmt.Sprintf("- electric motor: declared by supplier Acme Motors on %s (valid until %s) to originate in Malaysia, tariff code 8501.10; the case lists it as made in China.", today().Format(time.DateOnly), dayIn(90))
	sent := e.hs.engine.last
	if !strings.Contains(sent.Facts, want) || !strings.Contains(sent.Facts, "Supplier declarations") {
		t.Fatalf("the engine's facts lack the declaration:\n%s", sent.Facts)
	}
	if strings.Contains(sent.Facts, "Wound and assembled") || strings.Contains(sent.Facts, "Somchai") {
		t.Error("the supplier's free text reached the engine's input")
	}
	if sent.Product.Components[3].MadeIn != "China" {
		t.Error("the product sent to the engine had the user's value replaced")
	}
	// The case's own facts keep the mark too, once, and the user's answers.
	c := e.stored(id)
	if strings.Count(c.Facts, "Supplier declarations") != 1 || !strings.Contains(c.Facts, want) || !strings.Contains(c.Facts, "Yes, the motor is made in China.") {
		t.Errorf("stored facts:\n%s", c.Facts)
	}
	reask() // the block is rebuilt from the original facts, not added twice
	if c := e.stored(id); strings.Count(c.Facts, "Supplier declarations") != 1 {
		t.Errorf("block added twice:\n%s", c.Facts)
	}

	// A declaration that has lapsed no longer feeds the input.
	e.edit(id, func(c *store.Case) {
		rs := append([]store.SupplierRequest(nil), c.SupplierRequests...)
		rs[0].Answers = append([]store.SupplierAnswer(nil), rs[0].Answers...)
		rs[0].Answers[0].ValidUntil = dayIn(-1)
		c.SupplierRequests = rs
	})
	reask()
	if strings.Contains(e.hs.engine.last.Facts, "Supplier declarations") || strings.Contains(e.stored(id).Facts, "Supplier declarations") {
		t.Error("an expired declaration still feeds the input")
	}
}

func TestExpiredDeclarationsAreShownAsExpiredAndListed(t *testing.T) {
	e := newDeclEnv(t)
	a, b := e.wsCase(), e.wsCase()
	ta, _ := e.ask(a, 3, 4)
	tb, _ := e.ask(b, 0)
	e.answer(ta, 3, supplierAnswerJSON("Malaysia", 10, ""), http.StatusOK)
	e.answer(ta, 4, supplierAnswerJSON("Malaysia", 90, ""), http.StatusOK)
	e.answer(tb, 0, supplierAnswerJSON("Thailand", 20, ""), http.StatusOK)

	items := func(q string, who map[string]string) []map[string]any {
		var out []map[string]any
		for _, x := range e.call("GET", "/v1/supplier-declarations/expiring"+q, "", who, http.StatusOK)["items"].([]any) {
			out = append(out, x.(map[string]any))
		}
		return out
	}
	if got := items("?days=30", user("dave")); len(got) != 2 || got[0]["valid_until"] != dayIn(10) || got[0]["case_id"] != a || got[0]["part"] != "electric motor" || got[0]["days_left"] != float64(10) || got[1]["case_id"] != b {
		t.Errorf("within 30 days = %v", got)
	}
	if got := items("?days=15", user("dave")); len(got) != 1 || got[0]["case_id"] != a {
		t.Errorf("within 15 days = %v", got)
	}
	if got := items("", user("dave")); len(got) != 2 { // the default is 30
		t.Errorf("default window = %v", got)
	}
	if got := items("?days=5", user("dave")); len(got) != 0 {
		t.Errorf("within 5 days = %v", got)
	}
	// Someone outside the workspace sees none of them.
	if got := items("?days=365", user("zoe")); len(got) != 0 {
		t.Errorf("another workspace's listing = %v", got)
	}
	// A viewer may read the list.
	if got := items("?days=30", user("vera")); len(got) != 2 {
		t.Errorf("viewer listing = %v", got)
	}
	for _, bad := range []string{"?days=0", "?days=366", "?days=x", "?days=-3", "?include_expired=maybe"} {
		e.call("GET", "/v1/supplier-declarations/expiring"+bad, "", user("dave"), http.StatusBadRequest)
	}

	// Lapse the first part's declaration: the case shows it as expired, and
	// the listing leaves it out unless asked.
	e.edit(a, func(c *store.Case) {
		rs := append([]store.SupplierRequest(nil), c.SupplierRequests...)
		rs[0].Answers = append([]store.SupplierAnswer(nil), rs[0].Answers...)
		rs[0].Answers[0].ValidUntil = dayIn(-2)
		c.SupplierRequests = rs
	})
	list := e.call("GET", "/v1/cases/"+a+"/supplier-declarations", "", user("dave"), http.StatusOK)
	for _, p := range list["parts"].([]any) {
		if p := p.(map[string]any); p["part"] == "electric motor" && (p["status"] != "expired" || p["days_left"] != float64(-2)) {
			t.Errorf("a lapsed declaration = %v", p)
		}
	}
	if got := items("?days=30", user("dave")); len(got) != 1 || got[0]["case_id"] != b {
		t.Errorf("listing after the lapse = %v", got)
	}
	if got := items("?days=30&include_expired=true", user("dave")); len(got) != 2 || got[0]["status"] != "expired" {
		t.Errorf("listing with expired = %v", got)
	}
	// The case view says so too.
	sd := e.call("GET", "/v1/cases/"+a, "", user("dave"), http.StatusOK)["supplier_declarations"].(map[string]any)
	found := false
	for _, p := range sd["parts"].([]any) {
		if p := p.(map[string]any); p["part"] == "electric motor" {
			found = p["status"] == "expired"
		}
	}
	if !found {
		t.Errorf("the case does not show the expired declaration: %v", sd)
	}
}

func TestSupplierLinkExpiresAndRevokes(t *testing.T) {
	e := newDeclEnv(t)
	id := e.wsCase()
	tok, rq := e.ask(id, 3)
	rid := rq["id"].(string)
	e.answer(tok, 3, supplierAnswerJSON("Malaysia", 90, ""), http.StatusOK)

	// Only an editor revokes; the link is gone at once (410) and what it
	// declared stops counting, but stays on the record.
	e.call("DELETE", "/v1/cases/"+id+"/supplier-declarations/"+rid, "", user("vera"), http.StatusForbidden)
	e.call("DELETE", "/v1/cases/"+id+"/supplier-declarations/sd_0123456789abcdef", "", user("dave"), http.StatusNotFound)
	e.call("DELETE", "/v1/cases/"+id+"/supplier-declarations/bad", "", user("dave"), http.StatusNotFound)
	out := e.call("DELETE", "/v1/cases/"+id+"/supplier-declarations/"+rid, "", user("dave"), http.StatusOK)
	first := out["request"].(map[string]any)["revoked_at"]
	if out["request"].(map[string]any)["status"] != "revoked" || first == nil {
		t.Errorf("revoked = %v", out)
	}
	again := e.call("DELETE", "/v1/cases/"+id+"/supplier-declarations/"+rid, "", user("dave"), http.StatusOK)
	if again["request"].(map[string]any)["revoked_at"] != first {
		t.Error("revoking twice moved the revocation time")
	}
	if got := e.call("GET", "/v1/supplier-declaration", "", declTok(tok), http.StatusGone); got["code"] != "link_revoked" {
		t.Errorf("read on a revoked link = %v", got)
	}
	if got := e.answer(tok, 3, supplierAnswerJSON("China", 30, ""), http.StatusGone); got["code"] != "link_revoked" {
		t.Errorf("answer on a revoked link = %v", got)
	}
	list := e.call("GET", "/v1/cases/"+id+"/supplier-declarations", "", user("dave"), http.StatusOK)
	if p := list["parts"].([]any)[0].(map[string]any); p["status"] != "revoked" {
		t.Errorf("a revoked link's answer = %v", p)
	}
	e.call("POST", "/v1/cases/"+id+"/answers", `{"answers":[{"question":"q","answer":"The motor is made in China."}]}`, user("dave"), http.StatusOK)
	if strings.Contains(e.hs.engine.last.Facts, "Supplier declarations") {
		t.Error("a revoked declaration feeds the input")
	}

	// A link past its life is 410 too, and so does the request's status.
	tok2, rq2 := e.ask(id, 4)
	e.edit(id, func(c *store.Case) {
		rs := append([]store.SupplierRequest(nil), c.SupplierRequests...)
		rs[1].ExpiresAt = time.Now().UTC().Add(-time.Minute)
		c.SupplierRequests = rs
	})
	if got := e.call("GET", "/v1/supplier-declaration", "", declTok(tok2), http.StatusGone); got["code"] != "link_expired" {
		t.Errorf("read on an expired link = %v", got)
	}
	if got := e.answer(tok2, 4, supplierAnswerJSON("China", 30, ""), http.StatusGone); got["code"] != "link_expired" {
		t.Errorf("answer on an expired link = %v", got)
	}
	for _, r := range e.call("GET", "/v1/cases/"+id+"/supplier-declarations", "", user("dave"), http.StatusOK)["requests"].([]any) {
		if r := r.(map[string]any); r["id"] == rq2["id"] && r["status"] != "expired" {
			t.Errorf("expired request = %v", r)
		}
	}
	// A bad expiry is refused.
	for _, d := range []string{"0", "61", "-1"} {
		e.call("POST", "/v1/cases/"+id+"/supplier-declarations", `{"supplier_name":"A","parts":[0],"expires_in_days":`+d+`}`, user("dave"), http.StatusBadRequest)
	}
}

func TestSupplierRequestIsValidated(t *testing.T) {
	e := newDeclEnv(t)
	id := e.wsCase()
	base := "/v1/cases/" + id + "/supplier-declarations"
	for name, b := range map[string]string{
		"no supplier":    `{"parts":[0]}`,
		"no parts":       `{"supplier_name":"A","parts":[]}`,
		"unknown part":   `{"supplier_name":"A","parts":[13]}`,
		"negative part":  `{"supplier_name":"A","parts":[-1]}`,
		"duplicate part": `{"supplier_name":"A","parts":[1,1]}`,
		"bad email":      `{"supplier_name":"A","supplier_email":"not an email","parts":[0]}`,
		"long name":      `{"supplier_name":"` + strings.Repeat("a", 121) + `","parts":[0]}`,
		"unknown field":  `{"supplier_name":"A","parts":[0],"send":true}`,
	} {
		if got := e.call("POST", base, b, user("dave"), http.StatusBadRequest); got["code"] != "invalid_input" {
			t.Errorf("%s = %v", name, got)
		}
	}
	e.call("POST", base, `{"supplier_name":"A","parts":[0]}`, user("vera"), http.StatusForbidden)
	// The email is optional, and a message that names a bad value never echoes it.
	e.call("POST", base, `{"supplier_name":"A","parts":[0]}`, user("dave"), http.StatusCreated)
	got := e.call("POST", base, `{"supplier_name":"A","supplier_email":"private@example.org x","parts":[0]}`, user("dave"), http.StatusBadRequest)
	if b, _ := json.Marshal(got); strings.Contains(string(b), "private@example.org") {
		t.Errorf("an error echoes the email: %s", b)
	}
	// Open links are capped.
	for i := 0; i < 4; i++ {
		e.call("POST", base, `{"supplier_name":"A","parts":[0]}`, user("dave"), http.StatusCreated)
	}
	e.call("POST", base, `{"supplier_name":"A","parts":[0]}`, user("dave"), http.StatusConflict)
	// A pinned demo is not shared out, even by the admin token.
	demo := e.hs.newCase(t, admin)
	e.call("PUT", "/v1/cases/"+demo+"/pin", `{"pinned":true}`, admin, http.StatusOK)
	e.call("POST", "/v1/cases/"+demo+"/supplier-declarations", `{"supplier_name":"A","parts":[0]}`, admin, http.StatusForbidden)
}

func TestSupplierTokenChecks(t *testing.T) {
	e := newDeclEnv(t, func(d *Deps) {
		d.Limits = Limits{AuthFailuresPerMin: 3, DeclarationPerMin: 1000, ReviewPerMin: 1000}
	})
	id := e.wsCase()
	other := e.wsCase()
	tok, _ := e.ask(id, 3)
	otherTok, _ := e.ask(other, 3)
	review := e.call("POST", "/v1/cases/"+id+"/reviews", `{}`, user("dave"), http.StatusCreated)["token"].(string)

	// The token travels in its header only. (Each refusal here comes from its
	// own client address, so none of them spends the budget tested below.)
	n := 0
	from := func(h map[string]string) map[string]string {
		n++
		return with(h, map[string]string{"X-Client-IP": fmt.Sprintf("198.51.100.%d", n)})
	}
	e.call("GET", "/v1/supplier-declaration?token="+tok, "", from(nil), http.StatusNotFound)
	e.call("GET", "/v1/supplier-declaration", "", from(map[string]string{"X-Review-Token": tok}), http.StatusNotFound)
	e.call("GET", "/v1/supplier-declaration", "", from(nil), http.StatusNotFound)
	// A broker's review token is not a supplier token, nor the reverse.
	e.call("GET", "/v1/supplier-declaration", "", from(declTok(review)), http.StatusNotFound)
	e.call("GET", "/v1/review", "", from(tokenHeader(tok)), http.StatusNotFound)
	// Altered, truncated and well-formed but unknown tokens are all the same 404.
	flip := tok[:len(tok)-1] + map[bool]string{true: "B", false: "A"}[tok[len(tok)-1] == 'A']
	var want []byte
	for i, bad := range []string{flip, tok[:len(tok)-2], strings.Replace(tok, tok[4:16], "ffffffffffff", 1), "usd_" + strings.Repeat("0", 12) + "_" + strings.Repeat("A", 43)} {
		code, out := e.hs.doAs(t, "GET", "/v1/supplier-declaration", "", declTok(bad))
		b, _ := json.Marshal(out)
		if code != http.StatusNotFound && code != http.StatusTooManyRequests {
			t.Errorf("bad token %d = %d %s", i, code, b)
		}
		if code == http.StatusNotFound {
			if want == nil {
				want = b
			} else if !bytes.Equal(want, b) {
				t.Errorf("bad tokens answer differently: %s vs %s", want, b)
			}
		}
	}
	// Invalid tokens spend a per-client budget (3 a minute here): the next is 429.
	if code, out := e.hs.doAs(t, "GET", "/v1/supplier-declaration", "", declTok(flip)); code != http.StatusTooManyRequests || out["code"] != "auth_rate_limited" {
		t.Errorf("past the failure budget = %d %v", code, out)
	}
	// ... for the same client only: another address still gets in.
	if code, _ := e.hs.doAs(t, "GET", "/v1/supplier-declaration", "", with(declTok(tok), map[string]string{"X-Client-IP": "203.0.113.9"})); code != http.StatusOK {
		t.Errorf("another client = %d", code)
	}

	// One token reaches one request: it cannot answer for another case's
	// part, and the other case never learns of it.
	e.call("PUT", "/v1/supplier-declaration/parts/3", supplierAnswerJSON("Malaysia", 30, ""), with(declTok(tok), map[string]string{"X-Client-IP": "203.0.113.11"}), http.StatusOK)
	if got := e.stored(other).SupplierRequests[0].Answers; len(got) != 0 {
		t.Errorf("another case's request has answers: %+v", got)
	}
	e.call("GET", "/v1/supplier-declaration", "", with(declTok(otherTok), map[string]string{"X-Client-IP": "203.0.113.10"}), http.StatusOK)

	// The token is no identity on a case route.
	e.call("GET", "/v1/cases/"+id, "", declTok(tok), http.StatusNotFound)
	e.call("POST", "/v1/cases/"+id+"/supplier-declarations", `{"supplier_name":"A","parts":[0]}`, declTok(tok), http.StatusNotFound)
	e.call("GET", "/v1/supplier-declarations/expiring", "", declTok(tok), http.StatusOK) // anonymous: sees no case of anyone's
}

func TestSupplierRoutesHaveTheirOwnRateClass(t *testing.T) {
	e := newDeclEnv(t, func(d *Deps) {
		d.Limits = Limits{DeclarationPerMin: 2, ReviewPerMin: 1000, AuthFailuresPerMin: 100}
	})
	id := e.wsCase()
	tok, _ := e.ask(id, 3)
	hit := func() int { code, _ := e.hs.doAs(t, "GET", "/v1/supplier-declaration", "", declTok(tok)); return code }
	if a, b, c := hit(), hit(), hit(); a != http.StatusOK || b != http.StatusOK || c != http.StatusTooManyRequests {
		t.Errorf("three reads = %d %d %d, want 200 200 429", a, b, c)
	}
	// The broker's bucket is a different one.
	if code, _ := e.hs.doAs(t, "GET", "/v1/review", "", tokenHeader("ubr_000000000000_"+strings.Repeat("A", 43))); code == http.StatusTooManyRequests {
		t.Error("the review class was spent by supplier requests")
	}
}

func TestSupplierDocument(t *testing.T) {
	e := newDeclEnv(t)
	id := e.wsCase()
	tok, rq := e.ask(id, 3, 4)
	rid := rq["id"].(string)
	img := testPNG(t, 8, 8)
	doc := func(name string, b []byte) string {
		return `,"document":{"name":` + fmt.Sprintf("%q", name) + `,"content_base64":"` + base64.StdEncoding.EncodeToString(b) + `"}`
	}

	// The bytes are checked as uploads are: by their content, not the name.
	if got := e.answer(tok, 3, supplierAnswerJSON("Malaysia", 30, doc("cert.pdf", []byte("just text, not a pdf"))), http.StatusUnsupportedMediaType); got["code"] != "invalid_input" {
		t.Errorf("text named .pdf = %v", got)
	}
	if got := e.answer(tok, 3, supplierAnswerJSON("Malaysia", 30, doc("a.png", bytes.Repeat(img[:8], 20000))), http.StatusRequestEntityTooLarge); got["code"] != "invalid_input" {
		t.Errorf("an over-large document = %v", got)
	}
	e.answer(tok, 3, supplierAnswerJSON("Malaysia", 30, `,"document":{"name":"a.png","content_base64":"%%%"}`), http.StatusBadRequest)
	e.answer(tok, 3, supplierAnswerJSON("Malaysia", 30, `,"document":{"name":"a.png","content_base64":""}`), http.StatusBadRequest)
	if len(e.docs.m) != 0 {
		t.Fatal("a refused document was stored")
	}

	out := e.answer(tok, 3, supplierAnswerJSON("Malaysia", 30, doc(`../../etc/"cert".png`, img)), http.StatusOK)
	d := out["part"].(map[string]any)["answer"].(map[string]any)["document"].(map[string]any)
	if d["mime_type"] != "image/png" || d["bytes"] != float64(len(img)) || strings.ContainsAny(d["name"].(string), `/\"`) {
		t.Errorf("document = %v", d)
	}
	if _, has := d["sha256"]; has {
		t.Error("the supplier is shown the document's storage key")
	}
	// The case holds the description, never the bytes.
	blob, _ := json.Marshal(e.stored(id))
	if strings.Contains(string(blob), base64.StdEncoding.EncodeToString(img)) {
		t.Error("the document's bytes are in the case")
	}

	// The requester downloads it; viewers too; other workspaces cannot.
	url := "/v1/cases/" + id + "/supplier-declarations/" + rid + "/parts/3/document"
	for _, who := range []string{"dave", "vera", "carol"} {
		rec := serve(e.hs, "GET", url, "", user(who))
		if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), img) {
			t.Fatalf("%s downloads = %d", who, rec.Code)
		}
		h := rec.Header()
		if h.Get("Content-Type") != "image/png" || h.Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(h.Get("Content-Disposition"), "attachment") || strings.ContainsAny(h.Get("Content-Disposition"), "/\\") {
			t.Errorf("download headers = %v", h)
		}
	}
	e.call("GET", url, "", user("zoe"), http.StatusNotFound)
	e.call("GET", url, "", nil, http.StatusNotFound)
	// A part with no document, and a request that is not the case's.
	e.call("GET", "/v1/cases/"+id+"/supplier-declarations/"+rid+"/parts/4/document", "", user("dave"), http.StatusNotFound)
	e.call("GET", "/v1/cases/"+id+"/supplier-declarations/sd_0123456789abcdef/parts/3/document", "", user("dave"), http.StatusNotFound)
	// Without a document store, a document is refused and nothing else is.
	e2 := newDeclEnv(t, func(d *Deps) { d.SupplierDocs = nil })
	id2 := e2.wsCase()
	tok2, _ := e2.ask(id2, 3)
	e2.answer(tok2, 3, supplierAnswerJSON("Malaysia", 30, doc("a.png", img)), http.StatusServiceUnavailable)
	e2.answer(tok2, 3, supplierAnswerJSON("Malaysia", 30, ""), http.StatusOK)
}

func TestSupplierAnswersAreStaleWhenThePartsChange(t *testing.T) {
	e := newDeclEnv(t)
	id := e.wsCase()
	tok, _ := e.ask(id, 3)
	e.answer(tok, 3, supplierAnswerJSON("Malaysia", 90, ""), http.StatusOK)
	e.edit(id, func(c *store.Case) {
		p := *c.Product
		p.Components = append([]domain.Component(nil), p.Components...)
		p.Components[3].Name = "brushless rotor"
		c.Product = &p
	})
	list := e.call("GET", "/v1/cases/"+id+"/supplier-declarations", "", user("dave"), http.StatusOK)
	if parts := list["parts"].([]any); len(parts) != 0 {
		t.Errorf("a declaration for a part that is gone is shown as fact: %v", parts)
	}
	// And the supplier can no longer answer for a position that changed.
	e.answer(tok, 3, supplierAnswerJSON("Malaysia", 90, ""), http.StatusConflict)
}

func TestSupplierDeclarationAudit(t *testing.T) {
	ae := newAuditEnvWith(t, true, func(d *Deps) { d.SupplierDeclarations, d.SupplierDocs = true, &memBlobs{} })
	de := &declEnv{roleEnv: ae.roleEnv}
	id := ae.wsCase()
	ten := ae.tenant()

	before := len(ae.events(ten))
	tok, rq := de.ask(id, 3)
	in, _ := pair(t, "create", ae.since(ten, before), audit.SupplierRequestCreate, "")
	if !strings.HasPrefix(in.Object, "case:case_") || in.Actor != ae.actor("dave@example.com") {
		t.Errorf("create intent = %+v", in)
	}
	if !audit.SupplierRequestCreate.Sensitive() {
		t.Error("creating a link hands case content outside: it must fail closed like a review link")
	}

	before = len(ae.events(ten))
	de.call("GET", "/v1/supplier-declaration", "", declTok(tok), http.StatusOK)
	evs := ae.since(ten, before)
	if len(evs) != 1 || evs[0].Action != audit.SupplierView || evs[0].ActorKind != audit.ActorAnonymous || evs[0].Phase != audit.PhaseSingle {
		t.Errorf("supplier read = %s", brief(evs))
	}

	before = len(ae.events(ten))
	de.answer(tok, 3, supplierAnswerJSON("Malaysia", 30, ""), http.StatusOK)
	in, _ = pair(t, "answer", ae.since(ten, before), audit.SupplierAnswer, "")
	if in.ActorKind != audit.ActorAnonymous {
		t.Errorf("the supplier is anonymous in the trail: %+v", in)
	}

	before = len(ae.events(ten))
	de.call("DELETE", "/v1/cases/"+id+"/supplier-declarations/"+rq["id"].(string), "", user("dave"), http.StatusOK)
	pair(t, "revoke", ae.since(ten, before), audit.SupplierRequestRevoke, "")

	// No name, email, answer text or token reaches the trail.
	blob, _ := json.Marshal(ae.events(ten))
	for _, leak := range []string{"Acme", "acme-motors", "Somchai", "Rayong", "Plant manager", "Malaysia", tok[len(tok)-43:]} {
		if strings.Contains(string(blob), leak) {
			t.Errorf("the audit trail carries %q", leak)
		}
	}
	// A bad token is a denial in the platform's trail, like a bad review link.
	de.call("GET", "/v1/supplier-declaration", "", declTok("usd_000000000000_"+strings.Repeat("A", 43)), http.StatusNotFound)
}

func TestSupplierEmailIsNeverLogged(t *testing.T) {
	buf := useLogger(t, policyJSON)
	e := newDeclEnv(t)
	id := e.wsCase()
	tok, _ := e.ask(id, 3)
	e.call("GET", "/v1/supplier-declaration", "", declTok(tok), http.StatusOK)
	e.answer(tok, 3, supplierAnswerJSON("Malaysia", 30, ""), http.StatusOK)
	e.call("GET", "/v1/supplier-declaration", "", declTok("usd_000000000000_"+strings.Repeat("A", 43)), http.StatusNotFound)
	e.call("POST", "/v1/cases/"+id+"/supplier-declarations", `{"supplier_name":"Acme Motors","supplier_email":"bad email@x","parts":[0]}`, user("dave"), http.StatusBadRequest)
	for _, leak := range []string{"sales@acme-motors.example", "bad email", "acme-motors", tok[len(tok)-43:], "Somchai", "Rayong"} {
		if strings.Contains(buf.String(), leak) {
			t.Errorf("the log carries %q:\n%s", leak, buf)
		}
	}
	if !strings.Contains(buf.String(), "/v1/supplier-declaration") {
		t.Error("the route was not logged at all")
	}
}
