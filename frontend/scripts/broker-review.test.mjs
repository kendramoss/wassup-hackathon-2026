import "./render-support.mjs";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import { json } from "./lib/hook-runtime.mjs";

// Broker review links: the pure logic in lib/broker-review.ts, the client
// calls (the token travels only in X-Review-Token), the proxy's log words and
// the page's metadata and headers.
const B = await import("../src/lib/broker-review.ts");
const { api } = await import("../src/lib/api/client.ts");
process.env.BACKEND_URL = "http://backend.test";
const { routeOf } = await import("../src/lib/backend.ts");

const NOW = Date.parse("2026-11-21T12:00:00Z");
const base = {
  id: "rv_0123456789abcdef",
  status: "pending",
  subject: "plan",
  draft_label: "Move motor-fan assembly to Thailand",
  format: "markdown",
  created_at: "2026-11-20T09:00:00Z",
  expires_at: "2026-12-04T09:00:00Z",
  decisions: [],
};

test("status labels and tones", () => {
  assert.deepEqual(B.statusLabel("approved"), { label: "Approved", tone: "ok" });
  assert.deepEqual(B.statusLabel("changes_requested"), { label: "△ Changes requested", tone: "ask" });
  assert.deepEqual(B.statusLabel("file_requested"), { label: "Filing requested", tone: "ok" });
  for (const s of ["pending", "viewed", "revoked", "expired"]) assert.equal(B.statusLabel(s).tone, undefined, s);
  assert.equal(B.statusLabel("pending").label, "Not opened yet");
  assert.equal(B.statusLabel("nonsense").label, "Unknown");
  for (const s of ["pending", "viewed", "approved", "changes_requested", "file_requested", "revoked", "expired"]) assert.match(B.statusLabel(s).label, /^(△ )?[A-Z][a-z ]+$/, "sentence case");
});

test("a link past its expiry reads as expired; a revoked one as revoked", () => {
  assert.equal(B.effectiveStatus(base, NOW), "pending");
  assert.equal(B.effectiveStatus({ ...base, status: "approved", expires_at: "2026-11-21T11:59:59Z" }, NOW), "expired");
  assert.equal(B.effectiveStatus({ ...base, status: "viewed", revoked_at: "2026-11-21T10:00:00Z" }, NOW), "revoked");
  assert.equal(B.isActive(base, NOW), true);
  assert.equal(B.isActive({ ...base, status: "revoked" }, NOW), false);
});

test("the case chip shows the newest active link, else the newest", () => {
  assert.equal(B.caseReviewChip([], NOW), null);
  assert.equal(B.caseReviewChip(undefined, NOW), null);
  const old = { ...base, id: "rv_a", status: "approved" };
  const gone = { ...base, id: "rv_b", status: "revoked" };
  assert.equal(B.caseReviewChip([gone, old], NOW).label, "Approved");
  assert.equal(B.caseReviewChip([gone, old], NOW).active, 1);
  assert.equal(B.caseReviewChip([gone], NOW).label, "Revoked");
});

test("the link is the page origin plus /review#<token>, nothing else", () => {
  assert.equal(B.reviewLink("https://undelabs.dev", "/review#ubr_abc-DEF_123"), "https://undelabs.dev/review#ubr_abc-DEF_123");
  assert.equal(B.reviewLink("http://localhost:3000/", "/review#ubr_x"), "http://localhost:3000/review#ubr_x");
  for (const bad of [null, "", "/cases/x", "https://evil.example/review#ubr_x", "//evil.example/review#x", "/review/ubr_x", "/review?t=ubr_x", "/review#ubr_x?y=1", "/review/a#b", "/review#"]) assert.equal(B.reviewLink("https://undelabs.dev", bad), null, String(bad));
});

test("the generated link never has the token in its path or query", () => {
  const tok = "ubr_0123456789ab_" + "A".repeat(43);
  const u = new URL(B.reviewLink("https://undelabs.dev", "/review#" + tok));
  assert.equal(u.pathname, "/review");
  assert.equal(u.search, "");
  assert.equal(u.hash, "#" + tok);
  assert.ok(!u.pathname.includes("ubr_") && !u.search.includes("ubr_"));
  // What a browser sends to the server: no fragment.
  assert.equal(u.origin + u.pathname + u.search, "https://undelabs.dev/review");
});

test("the token is read from the fragment only", () => {
  assert.equal(B.tokenFromHash("#ubr_abc-DEF_1"), "ubr_abc-DEF_1");
  assert.equal(B.tokenFromHash("ubr_x"), "ubr_x");
  for (const bad of [null, undefined, "", "#", "#a b", "#<script>", "#" + "x".repeat(257)]) assert.equal(B.tokenFromHash(bad), null, String(bad));
});

test("the create body uses the draft step's own selection", () => {
  assert.deepEqual(B.createBody({ move: null, target: null, format: "markdown" }, 14), { format: "markdown", expires_in_days: 14 });
  assert.deepEqual(B.createBody({ move: "motor_fan", target: "Thailand", format: "cfr177" }, 7), { format: "cfr177", expires_in_days: 7, move_key: "motor_fan", target: "Thailand" });
  assert.deepEqual(B.createBody({ move: "motor_fan", target: null, format: "markdown" }, 30), { format: "markdown", expires_in_days: 30, move_key: "motor_fan" });
  assert.deepEqual(B.createBody({ move: null, plan_index: 0, format: "markdown" }, 14), { format: "markdown", expires_in_days: 14, plan_index: 0 });
  assert.deepEqual([...B.EXPIRY_DAYS], [7, 14, 30]);
  assert.equal(B.DEFAULT_EXPIRY, 14);
});

test("comments: required to request changes, 2000 characters at most, trimmed", () => {
  assert.deepEqual(B.checkDecision("approve", ""), { ok: true, body: { decision: "approve" } });
  assert.deepEqual(B.checkDecision("file_request", "  go  "), { ok: true, body: { decision: "file_request", comment: "go" } });
  assert.equal(B.checkDecision("changes", "   \n ").ok, false);
  assert.deepEqual(B.checkDecision("changes", "Fix line 2\nand 3"), { ok: true, body: { decision: "changes", comment: "Fix line 2\nand 3" } });
  assert.equal(B.checkDecision("approve", "x".repeat(2000)).ok, true);
  const over = B.checkDecision("approve", "x".repeat(2001));
  assert.equal(over.ok, false);
  assert.match(over.error, /2001 characters\. The limit is 2000\./);
  // Characters, not UTF-16 units: 2000 astral-plane characters fit.
  assert.equal(B.checkDecision("approve", "𝐀".repeat(2000)).ok, true);
  assert.deepEqual(B.counter("abc"), { text: "3 / 2000", over: false });
  assert.equal(B.counter("y".repeat(2001)).over, true);
});

test("the timeline: created, opened, each decision with its comment, then revoked or expired", () => {
  const r = {
    ...base,
    status: "approved",
    first_viewed_at: "2026-11-20T10:00:00Z",
    decisions: [
      { decision: "changes", comment: "Name the plant.\n<b>not html</b>", at: "2026-11-20T11:00:00Z" },
      { decision: "approve", at: "2026-11-21T08:00:00Z" },
    ],
  };
  const t = B.timeline(r, NOW);
  assert.deepEqual(t.map((e) => e.kind), ["created", "viewed", "decision", "decision"]);
  assert.equal(t[0].label, "Link made for Move motor-fan assembly to Thailand");
  assert.equal(t[2].label, "Requested changes");
  assert.equal(t[2].comment, "Name the plant.\n<b>not html</b>", "the comment is kept as written");
  assert.equal(t[2].tone, "ask");
  assert.equal(t[3].comment, undefined);
  const revoked = B.timeline({ ...r, status: "revoked", revoked_at: "2026-11-21T09:00:00Z" }, NOW);
  assert.equal(revoked.at(-1).kind, "revoked");
  const expired = B.timeline({ ...base, expires_at: "2026-11-21T00:00:00Z" }, NOW);
  assert.deepEqual(expired.map((e) => e.kind), ["created", "expired"]);
  // Out-of-order input still comes out oldest first.
  const shuffled = B.timeline({ ...base, first_viewed_at: "2026-11-20T12:00:00Z", decisions: [{ decision: "approve", at: "2026-11-20T10:00:00Z" }] }, NOW);
  assert.deepEqual(shuffled.map((e) => e.kind), ["created", "decision", "viewed"]);
});

test("broker failures map to the page's states", () => {
  assert.equal(B.brokerFailure({ status: 404, code: "not_found" }), "not_found");
  assert.equal(B.brokerFailure({ status: 410, code: "link_expired" }), "expired");
  assert.equal(B.brokerFailure({ status: 410, code: "link_revoked" }), "revoked");
  assert.equal(B.brokerFailure({ status: 429, code: "rate_limited" }), "rate_limited");
  assert.equal(B.brokerFailure({ status: 429, code: "auth_rate_limited" }), "rate_limited");
  assert.equal(B.brokerFailure({ status: 409, code: "conflict" }), "limit");
  assert.equal(B.brokerFailure({ status: 502, code: "backend_unreachable" }), "error");
  assert.match(B.ownerFailure({ status: 409, code: "conflict" }, "create"), /5 active links/);
  assert.equal(B.ownerFailure({ status: 429, code: "rate_limited" }, "create"), null);
});

test("the client sends the token only in X-Review-Token, never in the URL", async () => {
  const seen = [];
  const TOKEN = "ubr_SYNTHETIC_0123456789";
  globalThis.fetch = async (url, init = {}) => {
    seen.push({ url: String(url), init });
    if (String(url).endsWith("/review/decision")) return json({ review: { ...base, status: "approved" } });
    return json({ review: base, product_name: "Heat gun" });
  };
  await api.brokerReview(TOKEN);
  await api.brokerDecision(TOKEN, { decision: "approve" });
  assert.deepEqual(seen.map((s) => s.url), ["/api/v1/review", "/api/v1/review/decision"]);
  for (const s of seen) {
    assert.equal(new Headers(s.init.headers).get("x-review-token"), TOKEN);
    assert.ok(!s.url.includes(TOKEN));
    assert.ok(!String(s.init.body ?? "").includes(TOKEN), "not in the body either");
  }
  assert.equal(seen[1].init.method, "POST");
  assert.deepEqual(JSON.parse(seen[1].init.body), { decision: "approve" });
});

test("a 410 from the review endpoint keeps its code for the page", async () => {
  globalThis.fetch = async () => new Response(JSON.stringify({ code: "link_revoked", message: "link revoked" }), { status: 410, headers: { "content-type": "application/json" } });
  await assert.rejects(api.brokerReview("ubr_x"), (e) => e.status === 410 && e.code === "link_revoked" && B.brokerFailure(e) === "revoked");
  globalThis.fetch = async () => new Response(JSON.stringify({ code: "auth_rate_limited", message: "slow down", details: { retry_after_s: 42 } }), { status: 429, headers: { "content-type": "application/json" } });
  await assert.rejects(api.brokerReview("ubr_x"), (e) => e.status === 429 && e.retryAfter === 42 && B.brokerFailure(e) === "rate_limited");
});

test("owner calls hit the case's reviews routes", async () => {
  const seen = [];
  globalThis.fetch = async (url, init = {}) => {
    seen.push(`${init.method ?? "GET"} ${url}`);
    return json({ review: base, reviews: [], token: "t", review_path: "/review#t" });
  };
  await api.createReview("c 1", { format: "markdown", expires_in_days: 14 });
  await api.listReviews("c 1");
  await api.revokeReview("c 1", "rv_1");
  assert.deepEqual(seen, ["POST /api/v1/cases/c%201/reviews", "GET /api/v1/cases/c%201/reviews", "DELETE /api/v1/cases/c%201/reviews/rv_1"]);
});

test("the proxy's log keeps the review route words and drops ids", () => {
  assert.equal(routeOf("/v1/review/decision"), "/v1/review/decision");
  assert.equal(routeOf("/v1/cases/c1/reviews/rv_0123456789abcdef"), "/v1/cases/{}/reviews/{}");
});

test("comments render as escaped text with pre-wrap, never as HTML or Markdown", async () => {
  for (const f of ["../src/components/op/broker-reviews.tsx", "../src/components/op/screens/broker-review-page.tsx"]) {
    const src = await readFile(new URL(f, import.meta.url), "utf8");
    assert.doesNotMatch(src, /dangerouslySetInnerHTML/, f);
    assert.match(src, /whitespace-pre-wrap[^>]*>\{(e|d)\.comment\}/, f);
    assert.doesNotMatch(src, /localStorage|sessionStorage|console\./, f);
  }
});

test("the review page is noindex, nofollow, no-referrer, titled Broker review", async () => {
  const page = await readFile(new URL("../src/app/review/page.tsx", import.meta.url), "utf8");
  assert.match(page, /title: "Broker review"/);
  assert.match(page, /robots: \{ index: false, follow: false/);
  assert.match(page, /referrer: "no-referrer"/);
  const { default: config } = await import("../next.config.ts");
  const rules = await config.headers();
  const review = rules.find((r) => r.source === "/review/:path*");
  assert.ok(review, "a header rule for /review");
  assert.ok(rules.indexOf(review) > rules.findIndex((r) => r.source === "/:path*"), "after the site-wide rule, so it wins");
  const h = Object.fromEntries(review.headers.map((x) => [x.key, x.value]));
  assert.equal(h["Referrer-Policy"], "no-referrer");
  assert.equal(h["X-Robots-Tag"], "noindex, nofollow");
  const exact = rules.find((r) => r.source === "/review");
  assert.ok(exact && rules.indexOf(exact) > rules.findIndex((r) => r.source === "/:path*"), "a rule for /review itself, after the site-wide one");
  const he = Object.fromEntries(exact.headers.map((x) => [x.key, x.value]));
  assert.equal(he["Referrer-Policy"], "no-referrer");
  assert.equal(he["X-Robots-Tag"], "noindex, nofollow");
});

test("a token in a path is never accepted: /review/[token] is only a 404", async () => {
  const page = await readFile(new URL("../src/app/review/[token]/page.tsx", import.meta.url), "utf8");
  assert.match(page, /notFound\(\)/);
  const code = page.replace(/^\s*\/\/.*$/gm, "");
  assert.doesNotMatch(code, /params|redirect\(|BrokerReviewPage/);
  const screen = await readFile(new URL("../src/components/op/screens/broker-review-page.tsx", import.meta.url), "utf8");
  assert.doesNotMatch(screen, /useParams/);
  assert.match(screen, /tokenFromHash\(window\.location\.hash\)/);
  assert.match(screen, /history\.replaceState\(null, "", window\.location\.pathname\)/);
});
