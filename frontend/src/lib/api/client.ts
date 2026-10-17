// Typed client for the Origin Proof API, called through the /api/v1 proxy.
// Response shapes come from schema.d.ts, generated from docs/api/openapi.yaml
// (npm run gen:api); never hand-write them.
import type { components, paths } from "./schema";

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
export type BatchResponse = Json<"/v1/cases/batch", "post">;
export type HistoryEvent = NonNullable<CaseResponse["history"]>[number];
export type HtsSuggestResponse = Json<"/v1/products/hts-suggest", "post">;
export type HtsCandidate = NonNullable<HtsSuggestResponse["candidates"]>[number];
export type BomVersionResponse = Json<"/v1/cases/{id}/bom-version", "post">;
export type Workspace = S["Workspace"];
export type WorkspacesMine = Json<"/v1/workspaces/mine", "get">;
export type ChatTurn = NonNullable<AskResponse["chat"]>[number];
export type WhatIf = NonNullable<NonNullable<AskResponse["reply"]>["what_ifs"]>[number];

// ApiError carries the API's {code, message}. retryable marks model errors
// worth a "Try again".
export class ApiError extends Error {
  constructor(
    message: string,
    public status: number,
    public code: ApiErrorBody["code"] | "network" | "rate_limited" | "budget_reached",
    public retryable: boolean,
    // Seconds to wait before retrying (429 rate_limited), from Retry-After.
    public retryAfter?: number,
  ) {
    super(message);
  }
}

export const SIGNIN_EXPIRED = "op:signin-expired";

const retryableCodes = new Set(["backend_unreachable", "model_unavailable", "model_bad_response", "internal", "network"]);

async function toError(res: Response): Promise<ApiError> {
  const err = await readError(res);
  // The sign-in expired (D68): the header's sign-in button says so.
  if (err.status === 401 && (err.code as string) === "unauthenticated" && typeof window !== "undefined") window.dispatchEvent(new Event(SIGNIN_EXPIRED));
  return err;
}

async function readError(res: Response): Promise<ApiError> {
  let body: Partial<Omit<ApiErrorBody, "code">> & { code?: string; error?: string; details?: { retry_after_s?: number } } = {};
  try {
    body = await res.json();
  } catch {}
  if (res.status === 429 || body.code === "rate_limited") {
    const after = Number(res.headers.get("Retry-After")) || body.details?.retry_after_s || 30;
    return new ApiError(body.message ?? "Too many requests.", 429, "rate_limited", true, after);
  }
  // 503 "budget_reached" (D59): the day's model budget is spent; not retryable today.
  if (body.code === "budget_reached") {
    return new ApiError(body.message ?? "Today's demo budget for model calls is used up.", res.status, "budget_reached", false, body.details?.retry_after_s);
  }
  const code = (body.code ?? (res.status >= 500 ? "internal" : "invalid_input")) as ApiErrorBody["code"];
  const message = body.message ?? body.error ?? `The API returned ${res.status}.`;
  return new ApiError(message, res.status, code, retryableCodes.has(code));
}

async function call<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`/api/v1${path}`, { cache: "no-store", ...init });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError("Could not reach Origin Proof. Check your connection and try again.", 0, "network", true);
  }
  if (!res.ok) throw await toError(res);
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

export const api = {
  listCases: () => call<{ cases: CaseSummary[] }>("/cases"),
  getCase: (id: string) => call<CaseResponse>(`/cases/${enc(id)}`),
  predict: (body: PredictRequest) => post<CaseResponse>("/cases/predict", body),
  answers: (id: string, answers: { question: string; answer: string }[]) =>
    post<CaseResponse>(`/cases/${enc(id)}/answers`, { answers }),
  plan: (id: string, body: PlanRequest) => post<{ case_id?: string; plan?: PlanResult }>(`/cases/${enc(id)}/plan`, body),
  exportDraft: (id: string, body: { plan_index?: number; move_key?: string; target?: string; preview?: boolean; format?: "markdown" | "cfr177" }) =>
    post<ExportResponse>(`/cases/${enc(id)}/export`, body),
  scenarios: (id: string, body: { scenarios: { label?: string; facts?: string; product?: Product }[]; economics?: Economics }) =>
    post<CaseResponse>(`/cases/${enc(id)}/scenarios`, body),
  watch: (id: string) => post<CaseResponse>(`/cases/${enc(id)}/watch`),
  watchAll: () => post<WatchRunResponse>("/watch/run"),
  compareTargets: (id: string, targets: string[], signal?: AbortSignal) =>
    post<CompareResponse>(`/cases/${enc(id)}/compare-targets`, { targets }, signal),
  ask: (id: string, question: string, signal?: AbortSignal) => post<AskResponse>(`/cases/${enc(id)}/ask`, { question }, signal),
  resetChat: (id: string) => post<AskResponse>(`/cases/${enc(id)}/ask`, { reset: true }),
  demos: () => call<DemoList>("/demos"),
  demo: (id: string) => call<Demo>(`/demos/${enc(id)}`),
  ruling: (id: string) => call<Ruling>(`/rulings/${enc(id)}`),
  evalSummary: () => call<EvalSummary>("/eval/summary"),
  importBom: (form: FormData) => call<ImportResponse>("/products/import", { method: "POST", body: form }),
  importCsv: (body: { csv: string; name?: string; family?: string }) => post<ImportResponse>("/products/import", body),
  extract: (body: { facts: string; family?: string; name?: string }, signal?: AbortSignal) => post<ExtractResponse>("/products/extract", body, signal),
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
  putEconomics: (id: string, economics: Economics) => post<CaseResponse>(`/cases/${enc(id)}/economics`, economics, undefined, "PUT"),
  htsSuggest: (body: { facts: string } | { product: Product }, signal?: AbortSignal) => post<HtsSuggestResponse>("/products/hts-suggest", body, signal),
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
      body: JSON.stringify(body),
      signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError("Could not reach Origin Proof. Check your connection and try again.", 0, "network", true);
  }
  if (!res.ok) throw await toError(res);
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
      body: JSON.stringify(body),
      cache: "no-store",
    });
  } catch {
    throw new ApiError("Could not reach Origin Proof. Check your connection and try again.", 0, "network", true);
  }
  if (!res.ok) throw await toError(res);
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
