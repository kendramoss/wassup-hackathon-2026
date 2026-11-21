"use client";

import { useCallback, useEffect, useId, useRef, useState, type FormEvent, type ReactNode } from "react";
import { api, ApiError } from "@/lib/api/client";
import {
  COMMENT_MAX,
  DISCLAIMER,
  brokerFailure,
  checkDecision,
  counter,
  dateLabel,
  decisionLabel,
  effectiveStatus,
  plausibleToken,
  tokenFromHash,
  statusLabel,
  type BrokerPacket,
  type DecisionKind,
} from "@/lib/broker-review";
import { copy } from "@/lib/copy";
import { conf, originConf, originConfCaption, shortSubject } from "@/lib/format";
import { when } from "../broker-reviews";
import { ErrorBox } from "../chrome";
import { LetterMarkdown, ReviewNotes } from "../letter";
import { LogoMark } from "../logo";
import { Term } from "../term";
import { Button, Chip, Country, Panel, PanelHead, RulingLink, Spinner, cx } from "../ui";

// The page frame: the brand and the page's name, nothing that leads into the
// owner's app. 390px-safe; the content column is the only scroller.
export function BrokerReviewShell({ children }: { children: ReactNode }) {
  return (
    <>
      <header className="no-print flex min-h-14 flex-wrap items-center gap-x-4 gap-y-1 border-b border-line bg-paper px-4 py-2 sm:px-10">
        <span className="flex items-center gap-2.5 text-[19px] font-semibold tracking-[-0.03em]">
          <LogoMark size={24} />
          <span>
            Unde <i className="not-italic text-brand">Labs</i>
          </span>
        </span>
        <span className="text-[13px] text-dim">Broker review</span>
      </header>
      <main id="main" className="column mx-auto w-full max-w-[880px] flex-1 px-4 pb-20 pt-8 sm:px-10">
        {children}
      </main>
    </>
  );
}

type State = { kind: "loading" } | { kind: "ready"; p: BrokerPacket } | { kind: "failed"; e: unknown };

export function BrokerReviewPage() {
  // The token is the URL fragment (/review#<token>), which the browser never
  // sends to a server. It is read once, then dropped from the address bar
  // and history; from then on it lives in this component's memory only and
  // goes out only in the X-Review-Token header (lib/api/client.ts), never in
  // a URL, a log or storage. A reload therefore needs the link again.
  const [token, setToken] = useState<string | null>(null);
  const [read, setRead] = useState(false);
  const valid = plausibleToken(token);
  const [state, setState] = useState<State>({ kind: "loading" });

  // Read once: development's strict mode runs effects twice, and the second
  // run would find the fragment already cleared.
  const readOnce = useRef(false);
  useEffect(() => {
    if (readOnce.current) return;
    readOnce.current = true;
    const t = tokenFromHash(window.location.hash);
    if (window.location.hash) window.history.replaceState(null, "", window.location.pathname);
    setToken(t);
    setRead(true);
  }, []);

  const fetchPacket = useCallback(() => {
    if (!valid || !token) return;
    api.brokerReview(token).then(
      (p) => setState({ kind: "ready", p }),
      (e) => setState({ kind: "failed", e }),
    );
  }, [token, valid]);

  useEffect(() => {
    fetchPacket();
  }, [fetchPacket]);

  const retry = () => {
    setState({ kind: "loading" });
    fetchPacket();
  };

  if (!read) return <Spinner label="Loading the review…" />;
  if (!valid || !token) return <Gone kind={token === null ? "missing" : "not_found"} />;
  if (state.kind === "loading") return <Spinner label="Loading the review…" />;
  if (state.kind === "failed") {
    const kind = brokerFailure(state.e instanceof ApiError ? { status: state.e.status, code: state.e.code } : null);
    if (kind === "not_found" || kind === "expired" || kind === "revoked") return <Gone kind={kind} />;
    return (
      <div className="flex flex-col gap-3">
        <h1 className="text-xl tracking-[-0.04em]">{kind === "rate_limited" ? "Too many tries" : "The review didn't load"}</h1>
        <ErrorBox error={state.e} onRetry={retry} />
      </div>
    );
  }
  return (
    <Packet
      p={state.p}
      token={token}
      onPacket={(p) => setState({ kind: "ready", p })}
      onGone={(e) => setState({ kind: "failed", e })}
    />
  );
}

const GONE = {
  missing: { title: "Open the review link again", body: "This page needs the full link the sender gave you. For safety the link is cleared from the address bar once the page reads it, so a reload or a bookmark can't open the review. Open the link from their message again." },
  not_found: { title: "This link is not valid", body: "Check that you copied the whole link, or ask the person who sent it for a new one." },
  expired: { title: "This link has expired", body: "Review links work for a set number of days. Ask the person who sent it for a new one." },
  revoked: { title: "This link was revoked", body: "The sender turned this link off. Ask them for a new one if you still need to review the draft." },
} as const;

function Gone({ kind }: { kind: keyof typeof GONE }) {
  return (
    <div className="flex flex-col gap-3" role="status">
      <h1 className="text-xl tracking-[-0.04em]">{GONE[kind].title}</h1>
      <p className="duo max-w-[64ch] text-sm text-dim">{GONE[kind].body}</p>
    </div>
  );
}

const safeUrl = (u: string | undefined) => (u && /^https:\/\//i.test(u) ? u : null);

function Packet({ p, token, onPacket, onGone }: { p: BrokerPacket; token: string; onPacket: (p: BrokerPacket) => void; onGone: (e: unknown) => void }) {
  const r = p.review;
  const s = statusLabel(effectiveStatus(r));
  const o = originConf(p.prediction);
  const parts = Array.isArray(p.product?.components) ? p.product!.components!.filter((x) => x && typeof x.name === "string" && x.name) : [];
  const rulings = Array.isArray(p.cited_rulings) ? p.cited_rulings.filter((x) => x && typeof x.id === "string") : [];
  const factors = Array.isArray(p.prediction?.key_factors) ? p.prediction!.key_factors!.filter((f) => typeof f === "string" && f) : [];
  const notes = Array.isArray(p.draft?.notes) ? p.draft!.notes!.filter((n) => typeof n === "string") : [];
  const serverDisclaimer = p.disclaimer?.trim() && p.disclaimer.trim() !== DISCLAIMER ? p.disclaimer.trim() : null;

  return (
    <div className="flex flex-col gap-8">
      <div>
        <p className="eyebrow">Broker review{p.shared_at ? ` · shared on ${dateLabel(p.shared_at)}` : ""}</p>
        <h1 className="mt-1 break-words text-2xl tracking-[-0.04em]">{p.product_name || p.product?.name || "Product"}</h1>
        <div className="mt-2 flex flex-wrap items-center gap-2 text-[13px] text-dim">
          <Chip tone={s.tone}>{s.label}</Chip>
          <span>Draft ruling request for {r.draft_label || (r.subject === "plan" ? "a planned change" : "the current setup")}</span>
        </div>
        <p className="duo mt-2 max-w-[70ch] text-[13px] text-dim">
          This is the draft as it was when the link was made. Read it, then record your decision at the end of the page.{" "}
          <a href="#decision" className="text-ok underline underline-offset-2">
            Go to the decision
          </a>
        </p>
      </div>

      <div role="note" className="border-2 border-ink bg-sheet px-4 py-3 text-[15px] font-medium shadow-[4px_4px_0_var(--shadow)]">
        {DISCLAIMER}
        {serverDisclaimer && <p className="duo mt-1.5 text-[13px] font-normal text-dim">{serverDisclaimer}</p>}
      </div>

      <section aria-labelledby="pred-h">
        <h2 id="pred-h" className="eyebrow">
          Unde&apos;s prediction
        </h2>
        <div className="mt-2 flex flex-wrap items-baseline gap-x-4 gap-y-1">
          <Country name={p.prediction?.origin_country} size={18} className="text-2xl tracking-[-0.04em]" />
          {o.value !== undefined && o.value !== null && (
            <span className="text-[13px] text-dim">
              <Term explain={o.calibrated ? copy.terms.calibrated.text : copy.terms.stated.text}>{originConfCaption(o)}</Term>
            </span>
          )}
        </div>
        {o.value !== undefined && o.value !== null && !o.calibrated && (
          <p className="duo mt-1 text-[13px] text-dim">This is the model&apos;s stated confidence, not calibrated.</p>
        )}
        {p.prediction?.decisive_step && <p className="duo mt-3 max-w-[72ch] text-[15px]">{p.prediction.decisive_step}</p>}
        {factors.length > 0 && (
          <>
            <h3 className="eyebrow mt-4">Key factors</h3>
            <ul className="duo mt-1.5 list-disc pl-5 text-sm text-body">
              {factors.map((f, i) => (
                <li key={i}>{f}</li>
              ))}
            </ul>
          </>
        )}
      </section>

      <section aria-labelledby="prod-h">
        <h2 id="prod-h" className="eyebrow">
          Product and parts
        </h2>
        {parts.length ? (
          <ul className="mt-2 flex flex-col">
            {parts.map((c, i) => (
              <li key={i} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-0.5 border-b border-line py-2 text-sm first:border-t">
                <span className="min-w-0 break-words">{c.name}</span>
                {c.made_in ? <Country name={c.made_in} className="text-[13px] text-dim" /> : <span className="text-[13px] text-dim">origin not given</span>}
              </li>
            ))}
          </ul>
        ) : (
          <p className="duo mt-2 text-sm text-dim">The case was described in words, so there is no parts list.</p>
        )}
      </section>

      <section aria-labelledby="plan-h">
        <h2 id="plan-h" className="eyebrow">
          What the draft asks about
        </h2>
        {p.plan ? (
          <div className="mt-2 flex flex-col gap-1.5 text-sm">
            <p className="duo text-[15px]">{p.plan.description || "A planned production change"}</p>
            {p.plan.predicted_origin && (
              <p>
                Predicted origin after the change: <Country name={p.plan.predicted_origin} />
              </p>
            )}
            {(p.plan.supporting_ruling || p.plan.counter_ruling || typeof p.plan.check_confidence === "number") && (
              <p className="text-[13px] text-dim">
                {typeof p.plan.check_confidence === "number" && <>Precedent check {conf(p.plan.check_confidence)}. </>}
                {p.plan.supporting_ruling && (
                  <>
                    Supported by <RulingLink id={p.plan.supporting_ruling} />.{" "}
                  </>
                )}
                {p.plan.counter_ruling && (
                  <>
                    Counter-ruling <RulingLink id={p.plan.counter_ruling} />.
                  </>
                )}
              </p>
            )}
            {p.plan.below_bar && (
              <p role="note" className="duo border border-dashed border-ask-line bg-ask-wash px-3 py-2 text-[13px] text-ask">
                △ Below the 0.92 precedent-check bar. Treat the cited support as weaker.
              </p>
            )}
          </div>
        ) : (
          <p className="duo mt-2 text-sm">Current setup. The draft asks about the product as it is made today.</p>
        )}
      </section>

      <section aria-labelledby="rul-h">
        <h2 id="rul-h" className="eyebrow">
          Cited rulings
        </h2>
        {rulings.length ? (
          <ul className="mt-2 flex flex-col">
            {rulings.map((x) => {
              const url = safeUrl(x.url);
              return (
                <li key={x.id} className="border-b border-line py-2 text-sm first:border-t">
                  {url ? (
                    <a href={url} target="_blank" rel="noreferrer noopener" className="font-mono text-ok underline decoration-1 underline-offset-2">
                      {x.id}
                    </a>
                  ) : (
                    <RulingLink id={x.id} />
                  )}
                  <span className="text-dim">
                    {x.date ? ` · ${dateLabel(x.date)}` : ""}
                    {x.subject ? ` · ${shortSubject(x.subject)}` : ""}
                    {x.origin ? ` · held ${x.origin}` : ""}
                  </span>
                </li>
              );
            })}
          </ul>
        ) : (
          <p className="duo mt-2 text-sm text-dim">No earlier ruling is cited.</p>
        )}
      </section>

      <section aria-labelledby="draft-h">
        <h2 id="draft-h" className="eyebrow mb-3">
          The draft letter
        </h2>
        <ReviewNotes notes={notes} />
        {p.draft?.content ? (
          <article
            className="letter mx-auto max-w-[760px] border border-line bg-white px-5 py-8 text-[15px] leading-[1.7] text-ink shadow-[0_1px_2px_rgba(19,33,58,0.08),0_14px_36px_rgba(19,33,58,0.12)] sm:px-14 sm:py-14"
            style={{ fontFamily: "Georgia, 'Times New Roman', Times, serif" }}
          >
            <LetterMarkdown body={p.draft.content} />
          </article>
        ) : (
          <p className="duo text-sm text-dim">The draft has no text.</p>
        )}
      </section>

      <Decide p={p} token={token} onPacket={onPacket} onGone={onGone} />
    </div>
  );
}

const CHOICES: { key: DecisionKind; label: string; hint: string; button: string }[] = [
  { key: "approve", label: "Approve", hint: "The draft is fine to file as it stands.", button: "Approve the draft" },
  { key: "changes", label: "Request changes", hint: "Say what should change. A comment is required.", button: "Send the changes" },
  { key: "file_request", label: "File as a binding ruling request", hint: "Ask the sender to go ahead and file it with CBP.", button: "Ask to file it" },
];

function Decide({ p, token, onPacket, onGone }: { p: BrokerPacket; token: string; onPacket: (p: BrokerPacket) => void; onGone: (e: unknown) => void }) {
  const uid = useId();
  const [decision, setDecision] = useState<DecisionKind>("approve");
  const [comment, setComment] = useState("");
  const [problem, setProblem] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState("");
  const count = counter(comment);
  const choice = CHOICES.find((c) => c.key === decision)!;
  const history = Array.isArray(p.review.decisions) ? p.review.decisions : [];

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (busy) return;
    setError(null);
    setDone("");
    const check = checkDecision(decision, comment);
    if (!check.ok) {
      setProblem(check.error);
      return;
    }
    setProblem(null);
    setBusy(true);
    try {
      const r = await api.brokerDecision(token, check.body);
      onPacket({ ...p, review: r.review });
      setComment("");
      setDone(`Recorded: ${decisionLabel[decision].toLowerCase()}. The sender sees it on their case. You can record another decision if you change your mind.`);
    } catch (err) {
      const kind = brokerFailure(err instanceof ApiError ? { status: err.status, code: err.code } : null);
      if (kind === "not_found" || kind === "expired" || kind === "revoked") return onGone(err);
      if (kind === "limit") setProblem("This link has reached its limit of 20 decisions. Contact the sender directly.");
      else if (err instanceof ApiError && err.status === 400) setProblem(err.message ? err.message[0].toUpperCase() + err.message.slice(1) : "That decision wasn't valid.");
      else setError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section id="decision" aria-labelledby={`${uid}-h`} className="scroll-mt-6">
      <Panel>
        <div id={`${uid}-h`}>
          <PanelHead>{history.length ? "Record another decision" : "Your decision"}</PanelHead>
        </div>
        <form onSubmit={submit} className="flex flex-col gap-4" noValidate>
          <fieldset>
            <legend className="sr-only">Decision</legend>
            <div className="flex flex-col gap-1.5">
              {CHOICES.map((c) => (
                <label
                  key={c.key}
                  className={cx("flex min-h-11 cursor-pointer items-start gap-2.5 border px-3 py-2.5 text-sm", decision === c.key ? "border-ink bg-white" : "border-line bg-white hover:border-edge")}
                >
                  <input
                    type="radio"
                    name={`${uid}-decision`}
                    checked={decision === c.key}
                    onChange={() => {
                      setDecision(c.key);
                      setProblem(null);
                    }}
                    className="mt-1 accent-[var(--ok)]"
                  />
                  <span>
                    <span className="block font-medium">{c.label}</span>
                    <span className="duo block text-[12.5px] text-dim">{c.hint}</span>
                  </span>
                </label>
              ))}
            </div>
          </fieldset>
          <div>
            <label htmlFor={`${uid}-comment`} className="eyebrow block">
              {decision === "changes" ? "What should change (required)" : "Comment (optional)"}
            </label>
            <textarea
              id={`${uid}-comment`}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              aria-describedby={`${uid}-count${problem ? ` ${uid}-problem` : ""}`}
              aria-invalid={count.over || Boolean(problem) || undefined}
              aria-required={decision === "changes" || undefined}
              className="duo mt-1.5 min-h-32 w-full rounded-[2px] border border-line bg-white px-3 py-2 text-sm"
            />
            <p id={`${uid}-count`} className={cx("mt-1 text-right font-mono text-xs", count.over ? "text-fail" : "text-dim")}>
              {count.text}
              <span className="sr-only"> characters, {COMMENT_MAX} at most</span>
            </p>
            {problem && (
              <p id={`${uid}-problem`} role="alert" className="duo mt-1 text-[13px] text-fail">
                {problem}
              </p>
            )}
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" disabled={busy || count.over}>
              {busy ? "Sending…" : choice.button}
            </Button>
            <span className="duo text-xs text-dim">Plain text. The sender sees your comment exactly as written.</span>
          </div>
          {error ? <ErrorBox error={error} onRetry={() => setError(null)} /> : null}
          <p aria-live="polite" role="status" className={cx("duo text-sm", done && "border border-ok bg-ok-wash px-3 py-2 text-ok")}>
            {done}
          </p>
        </form>
      </Panel>

      {history.length > 0 && (
        <div className="mt-6">
          <h2 className="eyebrow">Decision history · oldest first</h2>
          <ol className="mt-2 flex flex-col">
            {history.map((d, i) => (
              <li key={i} className="border-b border-line py-2.5 text-sm first:border-t">
                <span className="block font-mono text-[11px] text-dim">{when(d.at)}</span>
                <span className={cx(d.decision === "changes" ? "font-medium" : "text-ok")}>{decisionLabel[d.decision] ?? d.decision}</span>
                {d.comment?.trim() && <p className="duo mt-1 whitespace-pre-wrap break-words border border-line bg-white px-2.5 py-1.5 text-[13px] text-body">{d.comment}</p>}
              </li>
            ))}
          </ol>
        </div>
      )}
    </section>
  );
}
