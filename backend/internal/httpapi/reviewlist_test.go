package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// #715: a cases-list row carries the status of its newest non-revoked broker
// link, and the expiry of a pending or viewed one, from the case document
// already loaded.

// link is a stored review created at created that runs for life.
func link(id string, created time.Time, life time.Duration) store.BrokerReview {
	return store.BrokerReview{ID: "rv_" + id, TokenHash: strings.Repeat(id, 64)[:64], CreatedAt: created, ExpiresAt: created.Add(life)}
}

func decided(rv store.BrokerReview, decision string) store.BrokerReview {
	rv.Decisions = []store.ReviewDecision{{Decision: decision, At: rv.CreatedAt}}
	return rv
}

func viewed(rv store.BrokerReview) store.BrokerReview {
	t := rv.CreatedAt
	rv.FirstViewedAt = &t
	return rv
}

func revoked(rv store.BrokerReview) store.BrokerReview {
	t := rv.CreatedAt.Add(time.Minute)
	rv.RevokedAt = &t
	return rv
}

func listRow(t *testing.T, hs harness, id string, h map[string]string) map[string]any {
	t.Helper()
	code, out := hs.doAs(t, "GET", "/v1/cases", "", h)
	if code != http.StatusOK {
		t.Fatalf("list = %d %v", code, out)
	}
	for _, r := range out["cases"].([]any) {
		if row := r.(map[string]any); row["id"] == id {
			return row
		}
	}
	t.Fatalf("case %s not listed", id)
	return nil
}

func TestListRowShowsNewestNonRevokedLinkStatus(t *testing.T) {
	now := time.Now().UTC()
	h := func(d time.Duration) time.Time { return now.Add(d) }
	day := 24 * time.Hour
	tie := h(-2 * day)
	cases := []struct {
		name       string
		links      []store.BrokerReview // in append order
		status     string
		hasExpires bool
	}{
		{"pending", []store.BrokerReview{link("a", h(-day), 14*day)}, reviewPending, true},
		{"viewed", []store.BrokerReview{viewed(link("a", h(-day), 14*day))}, reviewViewed, true},
		{"changes requested has no expiry", []store.BrokerReview{decided(viewed(link("a", h(-day), 14*day)), decisionChanges)}, reviewChanges, false},
		{"approved has no expiry", []store.BrokerReview{decided(link("a", h(-day), 14*day), decisionApprove)}, reviewApproved, false},
		{"file requested has no expiry", []store.BrokerReview{decided(link("a", h(-day), 14*day), decisionFile)}, reviewFileReq, false},
		{"an expired link reads expired, never pending", []store.BrokerReview{link("a", h(-10*day), 3*day)}, reviewExpired, false},
		{"newest expired beats an older open one", []store.BrokerReview{link("a", h(-3*day), 14*day), link("b", h(-2*day), day)}, reviewExpired, false},
		{"newest revoked is skipped", []store.BrokerReview{decided(link("a", h(-3*day), 14*day), decisionChanges), revoked(link("b", h(-day), 14*day))}, reviewChanges, false},
		{"older revoked never shows", []store.BrokerReview{revoked(link("a", h(-3*day), 14*day)), link("b", h(-day), 14*day)}, reviewPending, true},
		{"tie goes to the later in the list", []store.BrokerReview{decided(link("a", tie, 20*day), decisionChanges), link("b", tie, 20*day)}, reviewPending, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hs, cs := reviewHarness(t)
			alice := visitor(rvAlice)
			id := hs.newCase(t, alice)
			editCase(t, cs, id, func(c *store.Case) { c.BrokerReviews = tc.links })
			row := listRow(t, hs, id, alice)
			if row["review_status"] != tc.status {
				t.Errorf("review_status = %v, want %s", row["review_status"], tc.status)
			}
			if _, ok := row["review_expires_at"]; ok != tc.hasExpires {
				t.Errorf("review_expires_at present = %v, want %v", ok, tc.hasExpires)
			}
			for _, leak := range []string{"token", "ubr_", "decisions", "draft", "comment"} {
				if strings.Contains(strings.ToLower(mustJSON(row)), leak) {
					t.Errorf("the row carries %q: %v", leak, row)
				}
			}
		})
	}
}

func TestListRowWithoutUsableLinksIsUnchanged(t *testing.T) {
	hs, cs := reviewHarness(t)
	alice := visitor(rvAlice)
	id := hs.newCase(t, alice)
	check := func(why string) {
		t.Helper()
		row := listRow(t, hs, id, alice)
		for _, k := range []string{"review_status", "review_expires_at"} {
			if _, ok := row[k]; ok {
				t.Errorf("%s: row carries %s", why, k)
			}
		}
	}
	check("no links")
	editCase(t, cs, id, func(c *store.Case) {
		now := time.Now().UTC()
		c.BrokerReviews = []store.BrokerReview{revoked(link("a", now.Add(-48*time.Hour), time.Hour*24*14)), revoked(link("b", now.Add(-24*time.Hour), time.Hour*24*14))}
	})
	check("all revoked")
}

func TestListRowReviewStatusRespectsAccess(t *testing.T) {
	e := newRoleEnv(t, true)
	id := e.wsCase()
	now := time.Now().UTC()
	editCase(t, e.cs, id, func(c *store.Case) {
		c.BrokerReviews = []store.BrokerReview{decided(link("a", now.Add(-time.Hour), 14*24*time.Hour), decisionChanges)}
	})
	// A workspace viewer sees the status; a member of another workspace does
	// not see the case at all.
	if row := listRow(t, e.hs, id, user("vera")); row["review_status"] != reviewChanges {
		t.Errorf("viewer row = %v", row)
	}
	if e.hs.listIDs(t, user("zed"))[id] {
		t.Error("another workspace lists the case")
	}
	// A pinned demo shows a visitor no review state, as the case read shows no links.
	editCase(t, e.cs, id, func(c *store.Case) { c.Pinned = true })
	if row := listRow(t, e.hs, id, user("vera")); row["review_status"] != nil || row["review_expires_at"] != nil {
		t.Errorf("a visitor's demo row carries review state: %v", row)
	}
}
