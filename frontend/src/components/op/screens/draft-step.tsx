"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useEffect, useId, useMemo, useState } from "react";
import ReactMarkdown from "react-markdown";
import { api, ApiError, type ExportResponse } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { VIEWER_REASON, caseReadOnly } from "@/lib/roles";
import { recordExport } from "@/lib/record-export";
import { CaseShell, type CaseCtx } from "../case-shell";
import { ErrorBox, Footer, Main, Next, NextSteps } from "../chrome";
import { moveKey, movePhrase } from "../plan-bits";
import { tourHref, useStepLabel, useTour } from "../demo-tour";
import { ReviewNotes, fillIn, letterComponents } from "../letter";
import { ReviewNote } from "../review-note";
import { BrokerReviews } from "../broker-reviews";
import { BrokerPreview } from "../broker-preview";
import { Button, Panel, PanelHead, Spinner, cx } from "../ui";

export function DraftStep() {
  return <CaseShell tab="draft">{(ctx) => <Draft {...ctx} />}</CaseShell>;
}

// The backend's draft leaves bracketed placeholders for the requester; the
// form fills them in the live letter. Everything else is shown as returned.
const fields = [
  { key: "[REQUESTER NAME]", label: "Requester (company)", hint: "Named in the opening paragraph" },
  { key: "[NAME]", label: "Name (signs the letter)" },
  { key: "[TITLE]", label: "Title" },
  { key: "[COMPANY]", label: "Company" },
  { key: "[ADDRESS]", label: "Address", area: true },
  { key: "[CONTACT]", label: "Email and phone" },
] as const;

type Format = "markdown" | "cfr177";
const formats: { key: Format; label: string }[] = [
  { key: "markdown", label: "Short draft" },
  { key: "cfr177", label: copy.jurisdiction.part177Label },
];

function Draft({ id, c, setCase }: CaseCtx) {
  const stepNo = useStepLabel("draft", "Step 4 of 4");
  const readOnly = caseReadOnly(c);
  const tour = useTour();
  const params = useSearchParams();
  // With a shown plan, the letter defaults to it: the plan is what people
  // came to draft (D180). ?setup=current asks for today's setup instead.
  const current = params.get("setup") === "current";
  const shownPlan = c.plan?.plans?.find((p) => p.shown && (p.move?.length ?? 0) > 0);
  const defaulted = !params.get("move") && !current && shownPlan ? moveKey(shownPlan) : undefined;
  const move = params.get("move") ?? defaulted;
  const target = params.get("target") ?? undefined;
  const [format, setFormat] = useState<Format>("markdown");
  const [drafts, setDrafts] = useState<Partial<Record<Format, ExportResponse>>>({});
  // null until known: the cfr177 format needs EXPORT_CFR177 on the server,
  // and a 400 to its preview means it's off, so the choice stays hidden.
  const [cfr177, setCfr177] = useState<boolean | null>(null);
  const [error, setError] = useState<unknown>(null);
  const draft = drafts[format] ?? null;
  const [values, setValues] = useState<Record<string, string>>({});
  const formId = useId();

  const pick = move ? { move_key: move, target } : {};
  // A visit only previews; Download and Print record the export (#86).
  const fetchDraft = () => {
    api.exportDraft(id, { ...pick, preview: true }).then((d) => {
      setDrafts((x) => ({ ...x, markdown: d }));
      setCase((d) => (d && d.step !== "export" ? { ...d, step: "export" } : d));
    }, setError);
    api.exportDraft(id, { ...pick, preview: true, format: "cfr177" }).then(
      (d) => {
        setDrafts((x) => ({ ...x, cfr177: d }));
        setCfr177(true);
      },
      () => setCfr177(false),
    );
  };
  const load = () => {
    setError(null);
    setDrafts({});
    setCfr177(null);
    setFormat("markdown");
    fetchDraft();
  };
  // Draft once per move; the case object changes when the step is saved.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => void fetchDraft(), [id, move, target]);

  const text = useMemo(() => {
    let t = draft?.content ?? "";
    for (const f of fields) if (values[f.key]?.trim()) t = t.split(f.key).join(values[f.key].trim());
    return t;
  }, [draft, values]);

  // The letter is only what would be filed; the reviewer notes come apart
  // from it (draft.notes) and show on screen only, never printed or
  // downloaded (18 Oct). The below-bar note is the amber one.
  const body = text;
  const notes = draft?.notes ?? [];
  const dated = new Date().toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric" });

  // Recording answers with the letter as the case stands now. The letter goes
  // out only if that answer matches the preview on screen (raw content, before
  // the requester's own fields are filled in). Anything else, including a
  // failed recording, shows an error with the server's own message, drops the
  // stale draft and does not download or print it. See lib/record-export.ts.
  const record = async () => {
    // Recording is a write; a read-only case previews only (preview: true).
    if (readOnly) return false;
    const r = await recordExport(() => api.exportDraft(id, { ...pick, format }), draft?.content ?? "");
    if (r.ok) return true;
    setDrafts({});
    if (r.reason === "failed" && r.error) setError(r.error);
    else if (r.reason === "changed") setError(new ApiError("The case changed after this letter was drafted. Draft it again.", 409, "conflict", true));
    else setError(new ApiError("Couldn't confirm this letter is current. Try again.", 0, "internal", true));
    return false;
  };

  const download = async () => {
    if (!(await record())) return;
    const blob = new Blob([text], { type: "text/markdown" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = format === "cfr177" ? `ruling-request-cfr177-${id}.md` : `ruling-request-${id}.md`;
    a.click();
    URL.revokeObjectURL(a.href);
  };

  return (
    <>
      <Main className="grid gap-10 lg:grid-cols-[360px_minmax(0,1fr)]">
        <div className="no-print flex flex-col gap-5">
          <div>
            <p className="eyebrow">{stepNo} · Draft for broker</p>
            <h1 className="mt-1 text-xl tracking-[-0.04em]">Ruling request</h1>
            <p className="duo mt-1 text-sm text-dim">
              A draft binding-ruling request under 19 C.F.R. Part 177 {defaulted ? "for the shown plan" : move ? "for the chosen move" : "for the current setup"}. Your broker or counsel confirms and expands it before anything is sent to CBP.
            </p>
            {/* The shown plan is the default letter; today's setup is the
                second choice, and the other way round when asked for. */}
            {defaulted && shownPlan ? (
              <p className="duo mt-2 text-sm">
                <span className="block text-[13px] text-dim">
                  Move {movePhrase(shownPlan)}
                  {c.plan?.target ? ` to ${c.plan.target}` : ""}.
                </span>
                <Link href={tourHref(`/cases/${encodeURIComponent(id)}/draft?setup=current`, tour)} className="text-ok underline underline-offset-2">
                  Draft for today&apos;s setup instead →
                </Link>
              </p>
            ) : current && shownPlan ? (
              <p className="duo mt-2 text-sm">
                <Link href={tourHref(`/cases/${encodeURIComponent(id)}/draft`, tour)} className="text-ok underline underline-offset-2">
                  Draft for the shown plan instead →
                </Link>
                <span className="block text-[13px] text-dim">
                  Move {movePhrase(shownPlan)}
                  {c.plan?.target ? ` to ${c.plan.target}` : ""}.
                </span>
              </p>
            ) : null}
          </div>
          {cfr177 && (
            <fieldset>
              <legend className="eyebrow">Format</legend>
              <div className="mt-1.5 flex flex-col gap-1.5">
                {formats.map((f) => (
                  <label
                    key={f.key}
                    className={cx(
                      "flex min-h-10 cursor-pointer items-center gap-2.5 border px-3 text-sm",
                      format === f.key ? "border-ink bg-sheet" : "border-line bg-white hover:border-edge",
                    )}
                  >
                    <input type="radio" name={`${formId}-format`} checked={format === f.key} onChange={() => setFormat(f.key)} className="accent-[var(--ok)]" />
                    {f.label}
                  </label>
                ))}
              </div>
              <p className="duo mt-2 text-xs text-dim">{copy.jurisdiction.part177Help}</p>
              {format === "cfr177" && (
                <p className="duo mt-2 text-xs text-dim">Laid out on 19 C.F.R. § 177.2. Each highlighted [FILL IN] is something the case doesn&apos;t know: complete or delete it before filing.</p>
              )}
            </fieldset>
          )}
          {format === "markdown" && (
            <Panel>
              <PanelHead>Requester</PanelHead>
              <form id={formId} className="flex flex-col gap-3.5" onSubmit={(e) => e.preventDefault()}>
                {fields.map((f) => (
                  <div key={f.key}>
                    <label htmlFor={`${formId}${f.key}`} className="eyebrow block">
                      {f.label}
                    </label>
                    {"area" in f ? (
                      <textarea
                        id={`${formId}${f.key}`}
                        className="duo mt-1.5 min-h-20 w-full rounded-[2px] border border-line bg-white px-3 py-2 text-sm"
                        value={values[f.key] ?? ""}
                        onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
                      />
                    ) : (
                      <input
                        id={`${formId}${f.key}`}
                        className="mt-1.5 w-full rounded-[2px] border border-line bg-white px-3 py-2 text-sm"
                        value={values[f.key] ?? ""}
                        onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
                      />
                    )}
                  </div>
                ))}
              </form>
              <p className="mt-3 text-xs text-dim">Fills the bracketed placeholders in the letter. Nothing here is sent anywhere.</p>
            </Panel>
          )}
          {/* A pinned demo can't make review links, so it shows the page the
              broker would see instead (D220). */}
          {c.pinned && (
            <BrokerPreview
              c={c}
              draft={draft}
              plan={move && !target ? [...(c.plan?.plans ?? []), ...(c.plan?.rejected ?? [])].find((p) => moveKey(p) === move) : undefined}
              label={defaulted ? "the shown plan" : move ? "the chosen move" : "the current setup"}
            />
          )}
          <BrokerReviews
            id={id}
            c={c}
            setCase={setCase}
            selection={{ move: move ?? null, target: target ?? null, format }}
            draftLabel={defaulted ? "the shown plan" : move ? "the chosen move" : "the current setup"}
            ready={Boolean(draft)}
          />
          <NextSteps
            inline
            steps={[{ label: "Back to the plan", href: tourHref(`/cases/${encodeURIComponent(id)}/plan`, tour) }]}
          />
        </div>
        {/* From lg up the letter scrolls in its own box beside the form (18 Oct). */}
        <div className="letter-scroll min-w-0 lg:sticky lg:top-[150px] lg:max-h-[calc(100vh-170px)] lg:overflow-y-auto lg:pb-6 lg:pr-3">
          {error ? <ErrorBox error={error} onRetry={load} /> : null}
          {!draft && !error && <Spinner label="Drafting the letter…" />}
          {draft && (
            <>
            <ReviewNotes notes={notes} />
            <ReviewNote place="draft" />
            <article
              className="letter mx-auto max-w-[760px] border border-line bg-white px-7 py-10 text-[15px] leading-[1.7] text-ink shadow-[0_1px_2px_rgba(19,33,58,0.08),0_14px_36px_rgba(19,33,58,0.12)] sm:px-16 sm:py-16"
              style={{ fontFamily: "Georgia, 'Times New Roman', Times, serif" }}
            >
              {/* The 177.2 letter has its own [FILL IN: date] line. */}
              {!/\[FILL IN: date\]/.test(body) && <p className="mb-10 text-right text-[14px] text-dim">{dated}</p>}
              <ReactMarkdown components={letterComponents}>{body.replace(fillIn, (m) => "`" + m + "`")}</ReactMarkdown>
              {/* The letter's body usually opens with the same notice; print it once. */}
              {/* No disclaimer in the letter itself: it's in the notes above, on screen only. */}
            </article>
            </>
          )}
        </div>
      </Main>
      <Footer>
        <Next>{readOnly ? "You can preview this letter. Printing or downloading records the export, which needs editor access." : "Fill in the requester, then print or download the letter and send it to your customs broker."}</Next>
        <div className="ml-auto flex flex-wrap gap-2.5">
          <Button variant="ghost" onClick={download} disabled={!draft || readOnly} title={readOnly ? VIEWER_REASON : undefined}>
            Download Markdown
          </Button>
          <Button onClick={async () => {
              if (await record()) window.print();
            }} disabled={!draft || readOnly} title={readOnly ? VIEWER_REASON : undefined}>
            Print the letter
          </Button>
        </div>
      </Footer>
    </>
  );
}

// Kept here for existing imports; it lives in ../letter.
export { ReviewNotes };
