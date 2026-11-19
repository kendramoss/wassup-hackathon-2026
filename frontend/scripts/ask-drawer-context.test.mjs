import assert from "node:assert/strict";
import { test } from "node:test";
import { html, text } from "./render-support.mjs";

// Renders the real AskDrawer (static markup; the repo has no DOM test library).
const { AskDrawer, markVisitorTurn } = await import("../src/components/op/ask-drawer.tsx");

const mk = (pinned, id, answer) => {
  const t = { question: "Why China?", answer, cited_rulings: [], at: "2026-11-20T00:00:00.000Z" };
  if (pinned) markVisitorTurn(id, t);
  return { id, pinned, date: "2026-01-01", chat: [t] };
};
const render = (c) => text(html(AskDrawer, { c, onClose() {}, onChat() {} }));
const NOTE = /too long to carry into follow-up questions/;

test("a pinned demo shows the notice once a visitor answer was omitted from context, and still shows the answer", () => {
  const out = render(mk(true, "demo-long", "A".repeat(4001)));
  assert.match(out, NOTE);
  assert.match(out, /A{200}/); // the visible original answer is kept
});

test("no notice for an answer within the limit, or on a normal case", () => {
  assert.doesNotMatch(render(mk(true, "demo-short", "A".repeat(4000))), NOTE);
  assert.doesNotMatch(render(mk(false, "own-long", "A".repeat(9000))), NOTE);
});

test("a seeded turn (not the visitor's) never raises the notice", () => {
  const c = mk(false, "demo-seeded", "A".repeat(9000));
  assert.doesNotMatch(render({ ...c, pinned: true }), NOTE);
});
