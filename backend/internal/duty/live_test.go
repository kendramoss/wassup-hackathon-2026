package duty

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// htsResponses are trimmed copies of real GET /reststop/search answers
// (17 Oct 2026), keyed by keyword.
var htsResponses = map[string]string{
	// Rate on the 8-digit row; statistical rows below print none.
	"8419.89.95": `[{"htsno":"8419.89.95","description":"Other","general":"4.2%","other":"35%"},
		{"htsno":"8419.89.95.10","description":"Heating mantles","general":"","other":""},
		{"htsno":"8419.89.95.20","description":"For food and beverages","general":"","other":""}]`,
	// Rate on the only 10-digit row.
	"8471.30.01": `[{"htsno":"8471.30.01.00","description":"Portable automatic data processing machines","general":"Free","other":"35%"}]`,
	"6109.10.00": `[{"htsno":"6109.10.00","description":"Of cotton","general":"16.5%"},{"htsno":"6109.10.00.04","description":"T-shirts","general":""}]`,
	// Specific and compound rates.
	"0201.10.05": `[{"htsno":"0201.10.05","description":"Described in general note 15","general":"4.4¢/kg"}]`,
	"6505.00.90": `[{"htsno":"6505.00.90","description":"Other","general":"20.7¢/kg + 7.5%"}]`,
	// A fuzzy search: other headings come back too.
	"0402.21.25": `[{"htsno":"0402.21.25.00","description":"Other","general":"86.5¢/kg"},{"htsno":"2106.90.03.00","description":"Described in general note 15","general":"2.9¢/kg"}]`,
	"2106.90.04": `[{"htsno":"2106.90.03.00","description":"Described in general note 15","general":"2.9¢/kg"}]`,
	// A heading row with no rate printed.
	"8419.89.00": `[{"htsno":"8419.89.00","description":"Other:","general":null}]`,
	// Statistical lines with different rates.
	"1234.56.78": `[{"htsno":"1234.56.78.10","description":"A","general":"5%"},{"htsno":"1234.56.78.20","description":"B","general":"7%"}]`,
	"9999.99.99": `[]`,
}

func fakeHTS(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/reststop/search" || r.Header.Get("User-Agent") != "test-agent" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch kw := r.URL.Query().Get("keyword"); kw {
		case "5555.55.55":
			http.Error(w, "maintenance", http.StatusServiceUnavailable)
		case "4444.44.44":
			_, _ = w.Write([]byte("<html>not json</html>"))
		case "3333.33.33":
			time.Sleep(200 * time.Millisecond)
			_, _ = w.Write([]byte("[]"))
		default:
			body, ok := htsResponses[kw]
			if !ok {
				body = "[]"
			}
			w.Header().Set("Content-Type", "application/json;charset=UTF-8")
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func testLive(srv *httptest.Server) *Live {
	l := NewLive("test-agent", time.Hour, time.Second)
	l.BaseURL = srv.URL
	return l
}

func TestLiveMFN(t *testing.T) {
	srv, _ := fakeHTS(t)
	l := testLive(srv)
	tests := []struct {
		hts       string
		general   string
		rate      float64 // when supported
		supported bool
		note      string // fragment
		err       error
	}{
		{"8419.89.95", "4.2%", 0.042, true, "Section 301", nil},
		{"8419899540", "4.2%", 0.042, true, "", nil}, // a statistical number asks for its subheading
		{"8471.30.01", "Free", 0, true, "", nil},
		{"6109.10.00", "16.5%", 0.165, true, "", nil},
		{"0201.10.05", "4.4¢/kg", 0, false, "specific or compound", nil},
		{"6505.00.90", "20.7¢/kg + 7.5%", 0, false, "specific or compound", nil},
		{"0402.21.25", "86.5¢/kg", 0, false, "", nil},
		{"2106.90.04", "", 0, false, "", ErrNoRate}, // only another subheading came back
		{"8419.89.00", "", 0, false, "no column 1 general rate", nil},
		{"1234.56.78", "", 0, false, "print different rates", nil},
		{"9999.99.99", "", 0, false, "", ErrNoRate},
		{"8419", "", 0, false, "", ErrNoRate},
		{"8419.89.95; drop", "", 0, false, "", ErrNoRate},
	}
	for _, tt := range tests {
		m, err := l.MFN(context.Background(), tt.hts)
		if tt.err != nil {
			if !errors.Is(err, tt.err) {
				t.Errorf("%s: err = %v, want %v", tt.hts, err, tt.err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tt.hts, err)
			continue
		}
		if m.General != tt.general || m.Supported != tt.supported || !m.MFNOnly || !m.Live {
			t.Errorf("%s = %+v", tt.hts, m)
		}
		if tt.supported && (m.Rate == nil || *m.Rate != tt.rate) {
			t.Errorf("%s rate = %v, want %v", tt.hts, m.Rate, tt.rate)
		}
		if !tt.supported && m.Rate != nil {
			t.Errorf("%s: unsupported rate %q was converted to %v", tt.hts, m.General, *m.Rate)
		}
		if !strings.Contains(m.Note, tt.note) || !strings.Contains(m.Note, "additional duties (Section 301, forced-labor, 232) not included") {
			t.Errorf("%s note = %q", tt.hts, m.Note)
		}
		if !strings.HasPrefix(m.Source, srv.URL+"/reststop/search?keyword=") || m.FetchedAt.IsZero() {
			t.Errorf("%s source/fetched_at = %q %v", tt.hts, m.Source, m.FetchedAt)
		}
	}
}

func TestLiveCachesWithTTL(t *testing.T) {
	srv, calls := fakeHTS(t)
	l := testLive(srv)
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	if _, ok, _ := l.Cached("8419.89.95"); ok {
		t.Fatal("cached before any fetch")
	}
	for range 3 {
		if m, err := l.MFN(context.Background(), "8419.89.95"); err != nil || *m.Rate != 0.042 {
			t.Fatalf("MFN = %+v %v", m, err)
		}
	}
	if _, err := l.MFN(context.Background(), "8419.89.95.10"); err != nil { // same subheading
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Errorf("%d fetches, want 1", calls.Load())
	}
	if m, ok, err := l.Cached("8419.89.95"); !ok || err != nil || !m.FetchedAt.Equal(now) {
		t.Errorf("Cached = %+v %v %v", m, ok, err)
	}
	// A missing line is cached for the full TTL too.
	_, _ = l.MFN(context.Background(), "9999.99.99")
	_, _ = l.MFN(context.Background(), "9999.99.99")
	if calls.Load() != 2 {
		t.Errorf("%d fetches after the missing line, want 2", calls.Load())
	}
	now = now.Add(time.Hour + time.Second)
	if _, ok, _ := l.Cached("8419.89.95"); ok {
		t.Error("still cached after the TTL")
	}
	if _, err := l.MFN(context.Background(), "8419.89.95"); err != nil || calls.Load() != 3 {
		t.Errorf("refetch after TTL: %v, %d fetches", err, calls.Load())
	}
}

func TestLiveFailsGracefully(t *testing.T) {
	srv, calls := fakeHTS(t)
	l := testLive(srv)
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }
	for _, hts := range []string{"5555.55.55", "4444.44.44"} {
		if _, err := l.MFN(context.Background(), hts); err == nil || errors.Is(err, ErrNoRate) {
			t.Errorf("%s: err = %v, want a fetch error", hts, err)
		}
	}
	// A failure is reused for FailTTL (1 min), not the hour.
	n := calls.Load()
	_, _ = l.MFN(context.Background(), "5555.55.55")
	if calls.Load() != n {
		t.Error("a failure was refetched at once")
	}
	now = now.Add(61 * time.Second)
	_, _ = l.MFN(context.Background(), "5555.55.55")
	if calls.Load() != n+1 {
		t.Error("a failure was kept past FailTTL")
	}

	slow := testLive(srv)
	slow.HTTP.Timeout = 50 * time.Millisecond
	if _, err := slow.MFN(context.Background(), "3333.33.33"); err == nil {
		t.Error("timeout not enforced")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := l.MFN(ctx, "8419.89.95"); err == nil {
		t.Error("cancelled request succeeded")
	}
	if _, ok, _ := l.Cached("8419.89.95"); ok {
		t.Error("a cancelled request's failure was cached")
	}
	down := NewLive("test-agent", time.Hour, time.Second)
	down.BaseURL = "http://127.0.0.1:1" // nothing listens
	if _, err := down.MFN(context.Background(), "8419.89.95"); err == nil {
		t.Error("unreachable host succeeded")
	}
}

func TestParseAdValorem(t *testing.T) {
	for in, want := range map[string]float64{"Free": 0, "free": 0, " 4.2% ": 0.042, "16.5%": 0.165, "25%": 0.25, "0.9%": 0.009, "100%": 1} {
		if got, ok := ParseAdValorem(in); !ok || got != want {
			t.Errorf("ParseAdValorem(%q) = %v %v, want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "4.4¢/kg", "20.7¢/kg + 7.5%", "6.3¢/liter", "The duty provided in the applicable subheading + 25%", "150%", "Free (A,AU)", "4.2", "5%/kg"} {
		if got, ok := ParseAdValorem(in); ok {
			t.Errorf("ParseAdValorem(%q) = %v, want unsupported", in, got)
		}
	}
}

func TestMFNFill(t *testing.T) {
	r := 0.042
	m := MFN{HTS: "8419.89.95", General: "4.2%", Rate: &r, Supported: true, MFNOnly: true, Source: "https://hts.usitc.gov/reststop/search?keyword=8419.89.95",
		FetchedAt: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC), Note: AdditionalDutiesWarning + ". " + specialNote}
	got, filled := m.Fill(map[string]float64{"china": 0.417}, []string{"China", "Thailand", "viet nam", "Thailand", "Russia", "", "Cuba"})
	if got["china"] != 0.417 || got["Thailand"] != 0.042 || got["Vietnam"] != 0.042 || len(got) != 3 {
		t.Errorf("fill = %v", got)
	}
	if strings.Join(filled, ",") != "Thailand,Vietnam" {
		t.Errorf("filled = %v (column 2 origins and given rates must not be filled)", filled)
	}
	note := m.FillNote(filled)
	for _, want := range []string{"Thailand, Vietnam", "live HTS MFN rate for 8419.89.95, 4.2%", "fetched 2026-10-17", "Section 301, forced-labor, 232", "hts.usitc.gov"} {
		if !strings.Contains(note, want) {
			t.Errorf("note %q lacks %q", note, want)
		}
	}
	// A US plant, current origin or reshoring target is not imported: no
	// MFN rate for it, however the country is written.
	us, filled := m.Fill(nil, []string{"USA", "United States", "u.s.", "Puerto Rico", "Mexico"})
	if len(us) != 1 || us["Mexico"] != 0.042 || strings.Join(filled, ",") != "Mexico" {
		t.Errorf("US origins filled: %v %v", us, filled)
	}
	compound := MFN{HTS: "6505.00.90", General: "20.7¢/kg + 7.5%", Note: "general rate is specific or compound"}
	if out, filled := compound.Fill(nil, []string{"China"}); out != nil || filled != nil {
		t.Errorf("unsupported rate filled: %v %v", out, filled)
	}
	if n := compound.FillNote(nil); !strings.Contains(n, "specific or compound") {
		t.Errorf("unsupported note = %q", n)
	}
}

// TestLiveSmoke asks the real USITC HTS; it runs only with
// DUTY_LIVE_SMOKE=1 (CI has no network access).
func TestLiveSmoke(t *testing.T) {
	if os.Getenv("DUTY_LIVE_SMOKE") == "" {
		t.Skip("set DUTY_LIVE_SMOKE=1 to query hts.usitc.gov")
	}
	l := NewLive("origin-proof (Google Cloud AI Builder Cup 2026 prototype; smoke test)", time.Hour, 20*time.Second)
	m, err := l.MFN(context.Background(), "8419.89.95")
	if err != nil {
		t.Fatal(err)
	}
	// rates.json (checked 17 Oct 2026) has the same MFN component.
	if !m.Supported || m.Rate == nil || *m.Rate != 0.042 || m.General != "4.2%" {
		t.Errorf("8419.89.95 = %+v", m)
	}
	m, err = l.MFN(context.Background(), "6505.00.90")
	if err != nil || m.Supported {
		t.Errorf("compound 6505.00.90 = %+v %v", m, err)
	}
	t.Logf("8419.89.95: %s; 6505.00.90: %q", "4.2%", m.General)
}
