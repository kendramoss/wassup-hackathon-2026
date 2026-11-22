// Typed client for the Unde API, called through the /api/v1 proxy.
// Response shapes come from schema.d.ts, generated from docs/api/openapi.yaml
// (npm run gen:api); never hand-write them.
import type { components, paths } from "./schema";
import type { BrokerPacket, CreateReviewBody, CreateReviewResponse, DecisionKind, ReviewList, ReviewSummary } from "../broker-review";

type S = components["schemas"];
export type CaseResponse = S["CaseResponse"];
export type CaseSummary = S["CaseSummary"];
export type Prediction = S["Prediction"];
export type PrecedentRef = S["PrecedentRef"];
export type Product = S["Product"];
export type Plant = S["Plant"];
export type Component = S["Component"];
export type Operation = S["Operation"];
export type PredictRequest = S["PredictRequest"];
export type PlanRequest = S["PlanRequest"];
export type PlanResult = S["PlanResult"];
export type Plan = S["Plan"];
export type Economics = S["Economics"];
export type Comparison = S["Comparison"];
// PUT /cases/{id}/economics returns the stored economics, not the case, plus
// the case's scenarios repriced with them (omitted when it has none).
// The stale flags are true-only: a missing key means fresh (#281).
export type EconomicsResponse = { case_id: string; economics: Economics; scenarios?: Scenario[] } & Pick<CaseResponse, "plan_economics_stale" | "targets_economics_stale">;
export type TargetSummary = S["TargetSummary"];
export type MoveRef = S["MoveRef"];
export type EvalSummary = S["EvalSummary"];
export type Ruling = S["Ruling"];
export type ApiErrorBody = S["Error"];
export type Scenario = NonNullable<CaseResponse["scenarios"]>[number];
export type Watch = NonNullable<CaseResponse["watch"]>;

type Json<P extends keyof paths, M extends "get" | "post", Code extends number = 200> = paths[P][M] extends {
  responses: { [K in Code]: { content: { "application/json": infer T } } };
}
  ? T
  : never;

export type DemoList = Json<"/v1/demos", "get">;
export type Demo = Json<"/v1/demos/{id}", "get">;
export type ImportResponse = Json<"/v1/products/import", "post">;
export type ExportResponse = Json<"/v1/cases/{id}/export", "post">;
export type CompareResponse = Json<"/v1/cases/{id}/compare-targets", "post">;
export type AskResponse = Json<"/v1/cases/{id}/ask", "post">;
export type WatchRunResponse = Json<"/v1/watch/run", "post">;
export type ExtractResponse = Json<"/v1/products/extract", "post">;
export type ExtractMediaResponse = Json<"/v1/products/extract-media", "post">;
export type BatchResponse = Json<"/v1/cases/batch", "post">;
export type HistoryEvent = NonNullable<CaseResponse["history"]>[number];
export type PlanEstimate = S["PlanEstimate"];
export type HtsSuggestResponse = Json<"/v1/products/hts-suggest", "post">;
export type HtsCandidate = NonNullable<HtsSuggestResponse["candidates"]>[number];
export type PartsHtsSuggestResponse = Json<"/v1/products/parts-hts-suggest", "post">;
export type PartHtsCandidate = components["schemas"]["PartHtsCandidate"];
export type MoneyPrefillResponse = Json<"/v1/products/money-prefill", "post">;
export type RcepCheckResponse = Json<"/v1/rcep/check", "post">;
export type RcepCheckRequest = NonNullable<paths["/v1/rcep/check"]["post"]["requestBody"]>["content"]["application/json"];
export type FamilyCheckResponse = Json<"/v1/products/family-check", "post">;
export type CostEstimateResponse = Json<"/v1/products/cost-estimate", "post">;
export type BomVersionResponse = Json<"/v1/cases/{id}/bom-version", "post">;
export type Workspace = S["Workspace"];
export type WorkspaceRole = S["WorkspaceRole"];
// DELETE /workspaces/{id}/members/{email}: the removed email and the caller's
// updated workspace (null when the caller left).
export type RemoveMemberResponse = { removed: string; workspace: Workspace | null };
export type WorkspacesMine = Json<"/v1/workspaces/mine", "get">;
export type ChatTurn = NonNullable<AskResponse["chat"]>[number];
export type WhatIf = NonNullable<NonNullable<AskResponse["reply"]>["what_ifs"]>[number];

// ---- operating-cost inputs on the wire (#281) ----
// A block counts as entered only when it carries a number, a basis, a unit, a
// limit or a horizon. A note, or entries with only a ref or sources, are
// inactive: the server does not store them and they change no result.
type OpsBlock = NonNullable<Economics["operations"]>;
const hasValue = (v: unknown) => v !== undefined && v !== null && v !== "";
const PLANT_FIELDS = ["labour_usd_per_year", "logistics_usd_per_year", "capex_usd", "retooling_usd", "lead_time_days", "moq_units", "moq_unit"] as const;
const plantHasValues = (p: NonNullable<NonNullable<OpsBlock["groups"]>[string]["plants"]>[string]) => PLANT_FIELDS.some((k) => hasValue(p[k]));
const groupHasValues = (g: NonNullable<OpsBlock["groups"]>[string]) =>
  hasValue(g.cost_basis) || hasValue(g.unit) || hasValue(g.annual_demand_units) || hasValue(g.orders_per_year) || Object.values(g.plants ?? {}).some(plantHasValues);

export function opsActive(ops: OpsBlock | undefined | null): boolean {
  if (!ops) return false;
  return hasValue(ops.horizon_years) || hasValue(ops.max_lead_time_days) || Object.values(ops.groups ?? {}).some(groupHasValues);
}

// The economics as they go on the wire: an inactive operations block is left
// out, so a request with no operating costs entered is the legacy request
// exactly (the same object comes back when nothing changes).
export function economicsOnWire<E extends Economics | undefined>(e: E): E {
  if (!e || e.operations === undefined || opsActive(e.operations)) return e;
  const rest = { ...e };
  delete rest.operations;
  return rest;
}

export const withoutOps = (e: Economics): Economics => {
  const out = { ...e };
  delete out.operations;
  return out;
};

// ApiError carries the API's {code, message}. retryable marks model errors
// worth a "Try again".
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code: ApiErrorBody["code"] | "network" | "rate_limited" | "budget_reached" | "client_budget_reached" | "too_many_in_flight" | "timeout",
    public retryable: boolean,
    // Seconds to wait before retrying (429 rate_limited), from Retry-After.
    public retryAfter?: number,
  ) {
    super(message);
  }
}

// plan_economics_stale / targets_economics_stale are true-only on the wire (#281):
// a response that omits one means "fresh", so a merge copies a present true and
// DELETES a cached one when the key is absent. Never inherits a stale flag.
export function withStaleFlags(c: CaseResponse, from: Pick<CaseResponse, "plan_economics_stale" | "targets_economics_stale">): CaseResponse {
  const next = { ...c };
  if (from.plan_economics_stale === true) next.plan_economics_stale = true;
  else delete next.plan_economics_stale;
  if (from.targets_economics_stale === true) next.targets_economics_stale = true;
  else delete next.targets_economics_stale;
  return next;
}

// Folds a money save into a case. The response carries no product or
// prediction, so those stay; its scenarios (repriced with the saved money)
// replace the case's when present, else the case's stay. economics_priced was worked out
// from the old inputs and is dropped, so this is only the fallback when the
// authoritative case can't be re-read (see saveEconomics). A response for a
// different case is an error, never a silent success.
export function mergeEconomics(c: CaseResponse, r: EconomicsResponse): CaseResponse {
  if (r.case_id !== c.id) throw new ApiError("The saved money inputs belong to a different case. Reload the page and try again.", 0, "internal", false);
  const next: CaseResponse = withStaleFlags({ ...c, economics: r.economics }, r);
  if (r.scenarios) next.scenarios = r.scenarios;
  delete next.economics_priced;
  return next;
}

// One writer at a time for a case's money inputs and scenario run: a run
// sends the economics it started with and the server stores them, so a save
// overlapping a run (either order) would be lost. start() returns a release
// function, or null while another operation holds the lock.
export function opLock() {
  let held = false;
  return {
    start(): (() => void) | null {
      if (held) return null;
      held = true;
      return () => {
        held = false;
      };
    },
    get held() {
      return held;
    },
  };
}

export type SaveEconomicsIO = {
  // The newest case this screen holds, read synchronously. Its identity is
  // the change token: any update to the screen replaces it.
  latest: () => CaseResponse;
  apply: (c: CaseResponse) => void;
};

// Saves money inputs, then shows the backend's own view of the case so the
// sourced duty rates and labels (economics_priced) are right at once.
// The re-read can lose a race with another update (a scenario run finishing
// meanwhile): if the screen's case changed while it was in flight its answer
// may predate that update, so it is discarded and re-read. If it never
// settles, or fails, the saved economics are merged into the newest case,
// never into the one from when the save began.
export async function saveEconomics(id: string, economics: Economics, io: SaveEconomicsIO, tries = 3): Promise<void> {
  const r = await api.putEconomics(id, economics);
  if (r.case_id !== id) throw new ApiError("The saved money inputs belong to a different case. Reload the page and try again.", 0, "internal", false);
  // The screen may have moved to another case (reused component): never
  // write this case's answer over it.
  if (io.latest().id !== id) return;
  for (let i = 0; i < tries; i++) {
    const seen = io.latest();
    let fresh: CaseResponse;
    try {
      fresh = await api.getCase(id);
    } catch {
      break;
    }
    if (io.latest().id !== id) return;
    if (fresh.id !== id) break;
    if (io.latest() === seen) {
      io.apply(fresh);
      return;
    }
  }
  if (io.latest().id !== id) return;
  io.apply(mergeEconomics(io.latest(), r));
}

export const SIGNIN_EXPIRED = "op:signin-expired";
// A write was refused with 403 "forbidden" (#283): the caller's role, or the
// case, may have changed since the screen loaded, so what the server says now
// is the truth. Listeners (useCase, the workspace panel) re-read it. Only
// non-GET calls raise it, so a refused read cannot loop.
export const ACCESS_STALE = "op:access-stale";

// "conflict" (409): the case changed while a plan, comparison or export ran and
// the result was discarded (D97). Running it again is right, the same as the
// streamed error event, which the server marks retryable.
const retryableCodes = new Set(["backend_unreachable", "model_unavailable", "model_bad_response", "internal", "network", "conflict"]);

async function toError(res: Response, method = "GET"): Promise<ApiError> {
  const err = await readError(res);
  if (err.status === 403 && (err.code as string) === "forbidden" && method.toUpperCase() !== "GET" && typeof window !== "undefined") window.dispatchEvent(new Event(ACCESS_STALE));
  // The sign-in expired (D68): the header's sign-in button says so.
  if (err.status === 401 && (err.code as string) === "unauthenticated" && typeof window !== "undefined") window.dispatchEvent(new Event(SIGNIN_EXPIRED));
  return err;
}

// 429 codes that last until the next UTC day (D137): this client's share of
// the day's model budget, or an API key's daily quota. They are not short
// rate limits, so they get a calm note, not a countdown of many hours.
const dailyLimitCodes = new Set(["client_budget_reached", "quota_exceeded"]);
export const isDailyLimit = (e: unknown): boolean => e instanceof ApiError && dailyLimitCodes.has(e.code);

// A long JSON call (compare, triage) cut by the hosting's request timeout: the
// answer is a non-JSON 504, but the run may still finish and be saved.
export const TIMEOUT_MESSAGE = "This is taking longer than the page can wait. The run may still finish: check Saved cases.";

async function readError(res: Response): Promise<ApiError> {
  let body: Partial<Omit<ApiErrorBody, "code">> & { code?: string; error?: string; details?: { retry_after_s?: number } } = {};
  try {
    body = await res.json();
  } catch {}
  if (res.status === 429 && body.code && dailyLimitCodes.has(body.code)) {
    const after = Number(res.headers.get("Retry-After")) || body.details?.retry_after_s;
    return new ApiError(body.message ?? "Today's model budget for this network is used up.", 429, body.code as "client_budget_reached" | "quota_exceeded", false, after);
  }
  // Short limits (rate_limited, too_many_in_flight, an empty body): a countdown.
  if (res.status === 429 || body.code === "rate_limited") {
    const after = Number(res.headers.get("Retry-After")) || body.details?.retry_after_s || 30;
    return new ApiError(body.message ?? "Too many requests.", 429, "rate_limited", true, after);
  }
  if ((res.status === 504 || res.status === 408) && !body.code) return new ApiError(TIMEOUT_MESSAGE, res.status, "timeout", false);
  // 503 "budget_reached" (D59): the day's model budget is spent; not retryable today.
  if (body.code === "budget_reached") {
    return new ApiError(body.message ?? "Today's demo budget for model calls is used up.", res.status, "budget_reached", false, body.details?.retry_after_s);
  }
  const code = (body.code ?? (res.status >= 500 ? "internal" : "invalid_input")) as ApiErrorBody["code"];
  const message = body.message ?? body.error ?? `The API returned ${res.status}.`;
  return new ApiError(message, res.status, code, retryableCodes.has(code));
}

// errorText is a backend message as a person reads it: Go-style prefixes
// ("bom: ", "planner: ", "invalid input: ") are dropped, a bare "internal
// error" says what to do, and the first letter is capitalised unless the
// text follows a lead-in such as "The model didn't answer: ".
export function errorText(message: string | undefined | null, capitalise = true): string {
  let m = (message ?? "").trim();
  // A field-coded validation message ("economics: operations.horizon_required:
  // ...") is shown whole: the code names the field, and the form puts the
  // plain part beside it.
  if (/^[a-z_]+: [a-z_]+\.[a-z_]+/.test(m)) return m;
  for (let prev = ""; prev !== m; ) {
    prev = m;
    m = m.replace(/^(?:invalid input|[a-z][a-z0-9_]*): /, "");
  }
  if (!m || m === "internal error") return "Something went wrong on our side. Try again in a moment.";
  return capitalise ? m[0].toUpperCase() + m.slice(1) : m;
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`/api/v1${path}`, { cache: "no-store", ...init });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError("Couldn't reach Unde. Check your connection and try again.", 0, "network", true);
  }
  if (!res.ok) throw await toError(res, init?.method);
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

function post<T>(path: string, body?: unknown, signal?: AbortSignal, method = "POST"): Promise<T> {
  return call<T>(path, {
    method,
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  });
}

const enc = encodeURIComponent;

// A request that carries money: an inactive operating-cost block is left out,
// so with nothing entered the request is the legacy request exactly (#281).
function wireRequest<B extends { economics?: Economics }>(body: B): B {
  const e = economicsOnWire(body.economics);
  return e === body.economics ? body : { ...body, economics: e };
}
function noOps<B extends { economics?: Economics }>(body: B): B {
  return body.economics?.operations === undefined ? body : { ...body, economics: withoutOps(body.economics) };
}

export const api = {
  listCases: () => call<{ cases: CaseSummary[] }>("/cases"),
  getCase: (id: string) => call<CaseResponse>(`/cases/${enc(id)}`),
  predict: (body: PredictRequest) => post<CaseResponse>("/cases/predict", wireRequest(body)),
  answers: (id: string, answers: { question: string; answer: string }[]) =>
    post<CaseResponse>(`/cases/${enc(id)}/answers`, { answers }),
  // Cost and time of a plan search before it runs; no model call (D87).
  planEstimate: (id: string, body: PlanRequest, signal?: AbortSignal) => post<{ case_id?: string; estimate?: PlanEstimate }>(`/cases/${enc(id)}/plan/estimate`, wireRequest(body), signal),
  plan: (id: string, body: PlanRequest) => post<{ case_id?: string; plan?: PlanResult }>(`/cases/${enc(id)}/plan`, wireRequest(body)),
  exportDraft: (id: string, body: { plan_index?: number; move_key?: string; target?: string; preview?: boolean; format?: "markdown" | "cfr177" }) =>
    post<ExportResponse>(`/cases/${enc(id)}/export`, body),
  scenarios: (id: string, body: { scenarios: { label?: string; facts?: string; product?: Product }[]; economics?: Economics }) =>
    post<CaseResponse>(`/cases/${enc(id)}/scenarios`, wireRequest(body)),
  watch: (id: string) => post<CaseResponse>(`/cases/${enc(id)}/watch`),
  watchAll: () => post<WatchRunResponse>("/watch/run"),
  compareTargets: (id: string, targets: string[], signal?: AbortSignal) =>
    post<CompareResponse>(`/cases/${enc(id)}/compare-targets`, { targets }, signal),
  // history: a pinned demo's visitor turns (askBody builds it); see lib/ask-history.
  ask: (id: string, body: { question: string; history?: { question: string; answer: string }[] }, signal?: AbortSignal) =>
    post<AskResponse>(`/cases/${enc(id)}/ask`, body, signal),
  resetChat: (id: string) => post<AskResponse>(`/cases/${enc(id)}/ask`, { reset: true }),
  demos: () => call<DemoList>("/demos"),
  demo: (id: string) => call<Demo>(`/demos/${enc(id)}`),
  ruling: (id: string) => call<Ruling>(`/rulings/${enc(id)}`),
  evalSummary: () => call<EvalSummary>("/eval/summary"),
  importBom: (form: FormData) => call<ImportResponse>("/products/import", { method: "POST", body: form }),
  importCsv: (body: { csv: string; name?: string; fallback_name?: string; name_hint?: string; family?: string }) => post<ImportResponse>("/products/import", body),
  extract: (body: { facts: string; family?: string; name?: string }, signal?: AbortSignal) => post<ExtractResponse>("/products/extract", body, signal),
  // Import from an image or scanned document: form fields "file" (1-4 times),
  // "name", "family". The answer is a draft to review, never a prediction.
  extractMedia: (form: FormData, signal?: AbortSignal) => call<ExtractMediaResponse>("/products/extract-media", { method: "POST", body: form, signal }),
  batch: (file: File, signal?: AbortSignal) => {
    const form = new FormData();
    form.append("file", file);
    return call<BatchResponse>("/cases/batch", { method: "POST", body: form, signal });
  },
  // CSV rows the preview already parsed. Empty facts stay in the list so the
  // backend reports that row as failed instead of dropping it.
  batchItems: (items: { name: string; facts: string; date?: string }[], signal?: AbortSignal) =>
    post<BatchResponse>("/cases/batch", { items }, signal),
  rename: (id: string, display_name: string) => post<CaseResponse>(`/cases/${enc(id)}`, { display_name }, undefined, "PATCH"),
  deleteCase: (id: string) => call<void>(`/cases/${enc(id)}`, { method: "DELETE" }),
  putEconomics: (id: string, economics: Economics) => post<EconomicsResponse>(`/cases/${enc(id)}/economics`, economicsOnWire(economics), undefined, "PUT"),
  htsSuggest: (body: { facts: string } | { product: Product }, signal?: AbortSignal) => post<HtsSuggestResponse>("/products/hts-suggest", body, signal),
  // Codes for each part (D380): suggestions only, nothing is stored.
  partsHtsSuggest: (body: { product: Product }, signal?: AbortSignal) => post<PartsHtsSuggestResponse>("/products/parts-hts-suggest", body, signal),
  // The lookups price duty and group costs only: the operating-cost block is
  // not sent to them (a draft of it may not validate yet) and is never invented.
  moneyPrefill: (body: { product: Product; economics?: Economics }, signal?: AbortSignal) => post<MoneyPrefillResponse>("/products/money-prefill", noOps(body), signal),
  familyCheck: (body: { text: string; product?: Product }, signal?: AbortSignal) => post<FamilyCheckResponse>("/products/family-check", body, signal),
  // RCEP preference (D690): rules only, no model call, nothing stored.
  rcepCheck: (body: RcepCheckRequest, signal?: AbortSignal) => post<RcepCheckResponse>("/rcep/check", body, signal),
  costEstimate: (body: { product: Product; economics?: Economics }, signal?: AbortSignal) => post<CostEstimateResponse>("/products/cost-estimate", noOps(body), signal),
  bomVersion: (id: string, file: File, apply = false) => {
    const form = new FormData();
    form.append("file", file);
    if (apply) form.append("apply", "true");
    return call<BomVersionResponse>(`/cases/${enc(id)}/bom-version`, { method: "POST", body: form });
  },
  // No file: a 400 when BOM_VERSIONS is on, a 404 when off; reads nothing.
  bomVersionProbe: (id: string) => call<void>(`/cases/${enc(id)}/bom-version`, { method: "POST", body: new FormData() }),
  workspacesMine: () => call<WorkspacesMine>("/workspaces/mine"),
  createWorkspace: (name: string) => post<Workspace>("/workspaces", { name }),
  inviteMember: (id: string, email: string) => post<Workspace>(`/workspaces/${enc(id)}/members`, { email }),
  joinWorkspace: (id: string) => post<Workspace>(`/workspaces/${enc(id)}/join`),
  // Admin only (#283). The body is the role being set on the target member,
  // which is what the endpoint is for; the caller's own authority never
  // travels from here, the server reads it from the verified sign-in.
  setMemberRole: (id: string, email: string, role: WorkspaceRole) => post<Workspace>(`/workspaces/${enc(id)}/members/${enc(email)}/role`, { role }, undefined, "PUT"),
  // An admin removes anyone; any member removes themself (leave).
  removeMember: (id: string, email: string) => call<RemoveMemberResponse>(`/workspaces/${enc(id)}/members/${enc(email)}`, { method: "DELETE" }),
  // Broker review links, owner side. The token comes back once, on create.
  createReview: (id: string, body: CreateReviewBody) => post<CreateReviewResponse>(`/cases/${enc(id)}/reviews`, body),
  listReviews: (id: string) => call<ReviewList>(`/cases/${enc(id)}/reviews`),
  revokeReview: (id: string, reviewId: string) => call<{ review: ReviewSummary }>(`/cases/${enc(id)}/reviews/${enc(reviewId)}`, { method: "DELETE" }),
  // Broker side (/review#<token>). The token travels ONLY in the
  // X-Review-Token header: never in the URL or a query string, never logged,
  // never stored.
  brokerReview: (token: string) => call<BrokerPacket>("/review", { headers: { "X-Review-Token": token } }),
  brokerDecision: (token: string, body: { decision: DecisionKind; comment?: string }) =>
    call<{ review: ReviewSummary }>("/review/decision", {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Review-Token": token },
      body: JSON.stringify(body),
    }),
};

// Plan search events (POST /v1/cases/{id}/plan with Accept: text/event-stream).
export type PlanEvent =
  | { event: "start"; data: { target: string; moves: MoveRef[]; total: number } }
  | { event: "baseline"; data: { plan: Plan } }
  | { event: "move_started"; data: { key: string; done: number; total: number } }
  | { event: "move_done"; data: { key: string; plan: Plan; done: number; total: number } }
  | { event: "done"; data: { case_id?: string; plan?: PlanResult } }
  | { event: "error"; data: { code: string; message: string; retryable: boolean } };

// streamPlan reads the plan search with fetch and a ReadableStream
// (EventSource can't POST). 400/404 arrive as plain JSON before the stream
// and are thrown as ApiError.
export async function streamPlan(
  id: string,
  body: PlanRequest,
  onEvent: (e: PlanEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  let res: Response;
  try {
    res = await fetch(`/api/v1/cases/${enc(id)}/plan`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
      body: JSON.stringify(wireRequest(body)),
      signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError("Couldn't reach Unde. Check your connection and try again.", 0, "network", true);
  }
  if (!res.ok) throw await toError(res, "POST");
  if (!res.headers.get("content-type")?.startsWith("text/event-stream")) {
    // The backend answered with the plain JSON body.
    onEvent({ event: "done", data: await res.json() });
    return;
  }
  await readEvents(res, (event, data) => onEvent({ event, data } as PlanEvent));
}

// Predict steps (POST /v1/cases/predict with Accept: text/event-stream, D71).
export type PredictStage = { name: "router" | "precedents" | "engine" | "clarify"; status: "started" | "done"; ms: number };

// streamPredict runs predict and reports each step as the backend streams it.
// A backend with STREAM_PREDICT off answers plain JSON: then onStage is
// never called and the case comes back as from api.predict.
export async function streamPredict(body: PredictRequest, onStage: (s: PredictStage) => void): Promise<CaseResponse> {
  let res: Response;
  try {
    res = await fetch("/api/v1/cases/predict", {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
      body: JSON.stringify(wireRequest(body)),
      cache: "no-store",
    });
  } catch {
    throw new ApiError("Couldn't reach Unde. Check your connection and try again.", 0, "network", true);
  }
  if (!res.ok) throw await toError(res, "POST");
  if (!res.headers.get("content-type")?.startsWith("text/event-stream")) return res.json() as Promise<CaseResponse>;
  // Set from the callback, so an object (TypeScript doesn't follow closures).
  const got: { case?: CaseResponse; error?: ApiError } = {};
  try {
    await readEvents(res, (event, data) => {
      if (event === "stage") onStage(data as PredictStage);
      else if (event === "done") got.case = data as CaseResponse;
      else if (event === "error") {
        const e = data as { code: ApiErrorBody["code"]; message: string; retryable: boolean };
        got.error = new ApiError(e.message, 502, e.code, e.retryable);
      }
    });
  } catch {
    // The connection dropped mid-stream: reported below unless the case arrived.
  }
  if (got.error) throw got.error;
  if (!got.case) throw new ApiError("The connection closed before the analysis finished. Try again.", 0, "network", true);
  return got.case;
}

// readEvents reads a server-sent event body and calls onEvent with each
// event's name and parsed JSON data (EventSource can't POST).
async function readEvents(res: Response, onEvent: (event: string, data: unknown) => void): Promise<void> {
  const reader = res.body!.pipeThrough(new TextDecoderStream()).getReader();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf += value;
    let sep: number;
    while ((sep = buf.search(/\r?\n\r?\n/)) >= 0) {
      const block = buf.slice(0, sep);
      buf = buf.slice(sep).replace(/^\r?\n\r?\n/, "");
      let event = "message";
      const data: string[] = [];
      for (const line of block.split(/\r?\n/)) {
        if (line.startsWith(":")) continue; // keep-alive comment
        if (line.startsWith("event:")) event = line.slice(6).trim();
        else if (line.startsWith("data:")) data.push(line.slice(5).replace(/^ /, ""));
      }
      if (data.length === 0) continue;
      let parsed: unknown;
      try {
        parsed = JSON.parse(data.join("\n"));
      } catch {
        continue; // ignore a malformed event rather than dropping the stream
      }
      onEvent(event, parsed);
    }
  }
}
