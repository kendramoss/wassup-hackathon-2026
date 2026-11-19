package duty

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

var validHTS = []struct{ in, sub, stat string }{
	{"84198995", "8419.89.95", ""},
	{"8419.89.95", "8419.89.95", ""},
	{"8419 89 95", "8419.89.95", ""},
	{"8419-89-95", "8419.89.95", ""},
	{" 8419.89.95 ", "8419.89.95", ""},
	{"8419899500", "8419.89.95", "00"},
	{"8419.89.9500", "8419.89.95", "00"},
	{"8419.89.95.00", "8419.89.95", "00"},
	{"8419 89 95 85", "8419.89.95", "85"},
	{"8443.32.10", "8443.32.10", ""},
	{"8443.32.1050", "8443.32.10", "50"},
	{"8443321050", "8443.32.10", "50"},
}

var invalidHTS = []string{
	"", "   ", "8419", "841989", "8419.89", "841989951", "84198995001", "8419.89.950",
	"garbage84198995", "84198995garbage", "8419.89.95x", "8419.89.9S", "x8419.89.95",
	"841989950000000", "8419.89.9500000", "8419..89.95", "8419.89..95", ".8419.89.95", "8419.89.95.",
	"8419.89.95; drop", "8419/89/95", "8419_89_95", "8419.89 95", "8419 89.95.00", "84.19.89.95",
	"8419.8.995", "8 4 1 9 8 9 9 5", "８４１９８９９５", "8419.89.95\n00", "8419.89.95 00 00", "-8419.89.95",
	"84-19.89.95", "8419.899.5", "8419.89.9", "841.989.95",
}

func TestParseHTS(t *testing.T) {
	for _, tt := range validHTS {
		c, err := ParseHTS(tt.in)
		if err != nil || c.Subheading != tt.sub || c.Statistical != tt.stat {
			t.Errorf("ParseHTS(%q) = %+v, %v; want %s/%s", tt.in, c, err, tt.sub, tt.stat)
		}
	}
	for _, in := range invalidHTS {
		if c, err := ParseHTS(in); err == nil || !errors.Is(err, ErrInvalidHTS) || !errors.Is(err, ErrNoRate) {
			t.Errorf("ParseHTS(%q) = %+v, %v; want ErrInvalidHTS", in, c, err)
		}
	}
}

// Table.Line and LiveHTS must never disagree on what is a valid number.
func TestLineAndLiveHTSAgree(t *testing.T) {
	tb, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	all := append([]string{}, invalidHTS...)
	for _, v := range validHTS {
		all = append(all, v.in)
	}
	for _, in := range all {
		sub, liveOK := LiveHTS(in)
		c, perr := ParseHTS(in)
		if liveOK != (perr == nil) || (liveOK && sub != c.Subheading) {
			t.Errorf("%q: LiveHTS = %q,%v but ParseHTS = %+v,%v", in, sub, liveOK, c, perr)
		}
		if _, ok := tb.Line(in); ok && !liveOK {
			t.Errorf("%q: Line found a line for an invalid number", in)
		}
	}
	for _, in := range []string{"garbage84198995", "841989950000000", "8419.89.95x", "8419.89.9S", "x84198995"} {
		if _, ok := tb.Line(in); ok {
			t.Errorf("Line(%q) found a line", in)
		}
	}
	for _, in := range []string{"84198995", "8419.89.95", "8419 89 95", "8419.89.9500", "8419899500"} {
		if l, ok := tb.Line(in); !ok || l.HTS != "8419.89.95" {
			t.Errorf("Line(%q) = %v, %v", in, l.HTS, ok)
		}
	}
}

// Malformed numbers never reach the network, are not cached as fetches and
// return a meaningful unpriced error.
func TestLiveRejectsMalformedOffline(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = w.Write([]byte("[]")) }))
	defer srv.Close()
	l := testLive(srv)
	for _, in := range []string{"garbage84198995", "841989950000000", "8419.89.95x", "8419", "8419.89.95; drop"} {
		_, err := l.MFN(context.Background(), in)
		if !errors.Is(err, ErrInvalidHTS) || !errors.Is(err, ErrNoRate) {
			t.Errorf("MFN(%q) err = %v", in, err)
		}
		if _, ok, cerr := l.Cached(in); !ok || !errors.Is(cerr, ErrInvalidHTS) {
			t.Errorf("Cached(%q) = ok %v err %v", in, ok, cerr)
		}
	}
	if n := calls.Load(); n != 0 {
		t.Errorf("%d HTTP calls for malformed numbers", n)
	}
}

func TestLiveSeparatorControls(t *testing.T) {
	srv, calls := fakeHTS(t)
	l := testLive(srv)
	for _, in := range []string{"8419.89.95", "84198995", "8419 89 95", "8419.89.9500", "8419899500"} {
		m, err := l.MFN(context.Background(), in)
		if err != nil || m.HTS != "8419.89.95" || m.Rate == nil || *m.Rate != 0.042 {
			t.Errorf("MFN(%q) = %+v, %v", in, m, err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("%d fetches; every form should share one cache entry", n)
	}
}
