// Pinned-demo Ask what-if cards and steps across a full reload (#112).
// A demo's answer is never saved, so its cards (what_ifs) and steps would be
// lost with the page. This keeps, per tab and per case, a small typed copy of
// them for the turns the visitor asked (ownership: ask-history, #309), beside
// the existing per-tab stores. Nothing here reaches the server; the follow-up
// request still carries only question and answer. The stored value is
// untrusted on read: every field is checked, unknown fields are dropped, and
// a card that cannot be restored whole is dropped and the turn marked `lost`,
// so the drawer says so instead of showing a partial card as a verified one.

import type { ChatTurn, WhatIf } from "./api/client";
import { ASK_HISTORY_MAX, markOwned, ownedMarks, turnMark } from "./ask-history";

export type RichStep = { tool?: string; arg?: string };
export type Rich = { whatIfs: WhatIf[]; steps: RichStep[]; lost?: boolean };
type Store = Pick<Storage, "getItem" | "setItem" | "removeItem">;

export const RICH_MAX_WHATIFS = 5;
export const RICH_MAX_STEPS = 20;
const ENTRY_MAX = 80_000; // characters of JSON per turn
const FACTS_MAX = 20_000;
const STEP_MAX = 500;
const FIELD_MAX: Record<string, number> = { change: 500, origin: 100, decisive_step: 2000, facts: FACTS_MAX, regime: 100, specialist: 100, scope: 100, route: 100, triage_reason: 1000, provisional: 1000 };

const richKey = (caseId: string) => `op:ask-rich:${caseId}`;
const defaultStore = (): Store | null => {
  try {
    return typeof sessionStorage === "undefined" ? null : sessionStorage;
  } catch {
    return null;
  }
};
const isObj = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);

function card(v: unknown): WhatIf | null {
  if (!isObj(v)) return null;
  const out: Record<string, unknown> = {};
  for (const [k, max] of Object.entries(FIELD_MAX)) {
    const x = v[k];
    if (x === undefined) continue;
    if (typeof x !== "string" || x.length > max) return null;
    out[k] = x;
  }
  if (v.confidence !== undefined) {
    if (typeof v.confidence !== "number" || !Number.isFinite(v.confidence)) return null;
    out.confidence = v.confidence;
  }
  if (v.calibrated_confidence !== undefined && v.calibrated_confidence !== null) {
    const c = v.calibrated_confidence;
    if (typeof c !== "number" || !Number.isFinite(c) || c < 0 || c > 1) return null;
    out.calibrated_confidence = c;
  }
  return typeof out.change === "string" || typeof out.origin === "string" ? (out as WhatIf) : null;
}

function step(v: unknown): RichStep | null {
  if (!isObj(v)) return null;
  const out: RichStep = {};
  for (const k of ["tool", "arg"] as const) {
    const x = v[k];
    if (x === undefined) continue;
    if (typeof x !== "string" || x.length > STEP_MAX) return null;
    out[k] = x;
  }
  return out;
}

// Checks one turn's cards and steps; `lost` when any card could not be kept.
function clean(whatIfs: unknown, steps: unknown): Rich {
  const w = Array.isArray(whatIfs) ? whatIfs : [];
  const good = w.slice(0, RICH_MAX_WHATIFS).map(card).filter((x): x is WhatIf => !!x);
  const s = (Array.isArray(steps) ? steps : []).slice(0, RICH_MAX_STEPS).map(step).filter((x): x is RichStep => !!x);
  const r: Rich = { whatIfs: good, steps: s };
  if (good.length < w.length) r.lost = true;
  if (JSON.stringify(r).length > ENTRY_MAX) return { whatIfs: [], steps: [], lost: true };
  return r;
}

function readAll(caseId: string, store: Store | null): Record<string, unknown> {
  try {
    const v = JSON.parse(store?.getItem(richKey(caseId)) ?? "null");
    return isObj(v) && v.v === 1 && isObj(v.turns) ? v.turns : {};
  } catch {
    return {};
  }
}

// Remembers a visitor turn's cards and steps. Nothing is written for a reply
// with neither. Only the last ASK_HISTORY_MAX turns are kept.
export function saveRich(caseId: string, t: ChatTurn, reply: { what_ifs?: unknown; steps?: unknown }, store: Store | null = defaultStore()) {
  try {
    const r = clean(reply.what_ifs, reply.steps);
    if (!r.whatIfs.length && !r.steps.length && !r.lost) return;
    const mark = turnMark(t);
    const turns = readAll(caseId, store);
    delete turns[mark];
    turns[mark] = r;
    const keep = Object.fromEntries(Object.entries(turns).slice(-ASK_HISTORY_MAX));
    store?.setItem(richKey(caseId), JSON.stringify({ v: 1, turns: keep }));
  } catch {}
}

// The cards and steps of a turn this visitor asked, or null (nothing stored,
// not the visitor's turn, unreadable storage).
export function readRich(caseId: string, t: ChatTurn, store: Store | null = defaultStore()): Rich | null {
  try {
    const mark = turnMark(t);
    if (!ownedMarks(caseId, store).includes(mark)) return null;
    const e = readAll(caseId, store)[mark];
    if (e === undefined) return null;
    if (!isObj(e)) return { whatIfs: [], steps: [], lost: true };
    const r = clean(e.whatIfs, e.steps);
    if (e.lost === true) r.lost = true;
    return r;
  } catch {
    return null;
  }
}

export function clearRich(caseId: string, store: Store | null = defaultStore()) {
  try {
    store?.removeItem(richKey(caseId));
  } catch {}
}

type Reply = { answer?: string; cited_rulings?: string[]; what_ifs?: unknown; steps?: unknown };

// The pinned demo's turn as the visible chat holds it: plain, as the server
// would have saved it. Cards and steps are kept apart (saveRich).
export const pinnedTurn = (q: string, reply: Reply, at: string): ChatTurn => ({ question: q, answer: reply.answer ?? "", cited_rulings: reply.cited_rulings ?? [], at });

// A pinned demo answered: add the turn to the visible chat, mark it the
// visitor's, and keep its cards. Returns the new chat.
export function commitPinnedAnswer(caseId: string, chat: readonly ChatTurn[], q: string, reply: Reply, at: string, store: Store | null = defaultStore()): ChatTurn[] {
  const turn = pinnedTurn(q, reply, at);
  markOwned(caseId, turn, store);
  saveRich(caseId, turn, reply, store);
  return [...chat, turn];
}
