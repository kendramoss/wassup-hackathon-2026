"use client";

import { useSearchParams } from "next/navigation";
import { useEffect, useId, useMemo, useState } from "react";
import ReactMarkdown from "react-markdown";
import { api, type ExportResponse } from "@/lib/api/client";
import { CaseShell, type CaseCtx } from "../case-shell";
import { ErrorBox, Footer, Main, Next, NextSteps } from "../chrome";
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
  { key: "cfr177", label: "Binding ruling request (19 CFR 177)" },
];

// The 19 CFR 177 letter marks what the case doesn't know as [FILL IN: …]
// (D66). Wrapped as inline code so the letter can highlight them.
const fillIn = /\[FILL IN(?::[^\]]*)?\]/g;

function Draft({ id, c, setCase }: CaseCtx) {
  const params = useSearchParams();
  const move = params.get("move") ?? undefined;
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
      if (c.step !== "export") setCase({ ...c, step: "export" });
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
  const notice = notes.find((n) => /below the confidence bar/i.test(n)) ?? null;
  const lead = notes.filter((n) => n !== notice).join(" ") || null;
  const dated = new Date().toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric" });

  const record = () => void api.exportDraft(id, { ...pick, format }).catch(() => {});

  const download = () => {
    record();
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
            <p className="eyebrow">Step 4 of 4 · Draft for broker</p>
            <h1 className="mt-1 text-xl tracking-[-0.04em]">Ruling request</h1>
            <p className="duo mt-1 text-sm text-dim">
              A draft binding-ruling request under 19 C.F.R. Part 177 {move ? "for the chosen move" : "for the current setup"}. Your broker or counsel confirms and expands it before anything is sent to CBP.
            </p>
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
          <NextSteps
            inline
            steps={[{ label: "Back to the plan", href: `/cases/${encodeURIComponent(id)}/plan` }]}
          />
        </div>
        {/* From lg up the letter scrolls in its own box beside the form (18 Oct). */}
        <div className="letter-scroll min-w-0 lg:sticky lg:top-[150px] lg:max-h-[calc(100vh-170px)] lg:overflow-y-auto lg:pb-6 lg:pr-3">
          {error ? <ErrorBox error={error} onRetry={load} /> : null}
          {!draft && !error && <Spinner label="Drafting the letter…" />}
          {draft && (
            <>
            {(notice || lead) && (
              <div className="no-print mx-auto mb-4 flex max-w-[760px] flex-col gap-2">
                {notice && (
                  <p role="note" className="border border-ask-line bg-ask-wash px-4 py-3 text-[13px] text-ask">
                    {notice}
                  </p>
                )}
                {lead && (
                  <p role="note" className="border border-line bg-sheet px-4 py-2.5 text-[12.5px] text-dim">
                    {lead}
                  </p>
                )}
              </div>
            )}
            <article
              className="letter mx-auto max-w-[760px] border border-line bg-white px-7 py-10 text-[15px] leading-[1.7] text-ink shadow-[0_1px_2px_rgba(19,33,58,0.08),0_14px_36px_rgba(19,33,58,0.12)] sm:px-16 sm:py-16"
              style={{ fontFamily: "Georgia, 'Times New Roman', Times, serif" }}
            >
              {/* The 177.2 letter has its own [FILL IN: date] line. */}
              {!/\[FILL IN: date\]/.test(body) && <p className="mb-10 text-right text-[14px] text-dim">{dated}</p>}
              <ReactMarkdown
                components={{
                  h1: (p) => <h1 className="mb-6 text-center text-[17px] font-bold" {...p} />,
                  h2: (p) => <h2 className="mb-2 mt-7 text-[15px] font-bold" {...p} />,
                  h3: (p) => <h3 className="mb-2 mt-5 text-[15px] font-bold italic" {...p} />,
                  p: (p) => <p className="mt-4" {...p} />,
                  ul: (p) => <ul className="mt-2 list-disc pl-5" {...p} />,
                  ol: (p) => <ol className="mt-2 list-decimal pl-5" {...p} />,
                  a: (p) => <a className="text-ok underline underline-offset-2" target="_blank" rel="noreferrer" {...p} />,
                  code: ({ children }) => {
                    const t = String(children);
                    return t.startsWith("[FILL IN") ? (
                      <mark className="rounded-[2px] bg-ask-wash px-1 font-sans text-[13px] text-ask">{t}</mark>
                    ) : (
                      <code className="font-mono text-[12.5px]">{t}</code>
                    );
                  },
                }}
              >
                {body.replace(fillIn, (m) => "`" + m + "`")}
              </ReactMarkdown>
              {/* The letter's body usually opens with the same notice; print it once. */}
              {/* No disclaimer in the letter itself: it's in the notes above, on screen only. */}
            </article>
            </>
          )}
        </div>
      </Main>
      <Footer>
        <Next>Fill in the requester, then print or download the letter and send it to your customs broker.</Next>
        <div className="ml-auto flex flex-wrap gap-2.5">
          <Button variant="ghost" onClick={download} disabled={!draft}>
            Download Markdown
          </Button>
          <Button onClick={() => {
              record();
              window.print();
            }} disabled={!draft}>
            Print the letter
          </Button>
        </div>
      </Footer>
    </>
  );
}
