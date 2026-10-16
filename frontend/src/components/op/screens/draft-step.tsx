"use client";

import { useSearchParams } from "next/navigation";
import { useEffect, useId, useMemo, useState } from "react";
import ReactMarkdown from "react-markdown";
import { api, type ExportResponse } from "@/lib/api/client";
import { CaseShell, type CaseCtx } from "../case-shell";
import { ErrorBox, Footer, Main } from "../chrome";
import { Button, ButtonLink, Panel, PanelHead, Spinner } from "../ui";

export function DraftStep() {
  return <CaseShell step={4}>{(ctx) => <Draft {...ctx} />}</CaseShell>;
}

// The backend's draft leaves bracketed placeholders for the requester; the
// form fills them in the live letter. Everything else is shown as returned.
const fields = [
  { key: "[REQUESTER NAME]", label: "Requester (company)", hint: "Named in the opening paragraph" },
  { key: "[NAME]", label: "Signed by" },
  { key: "[TITLE]", label: "Title" },
  { key: "[COMPANY, ADDRESS, CONTACT]", label: "Company, address, contact", area: true },
] as const;

function Draft({ id, c, setCase }: CaseCtx) {
  const params = useSearchParams();
  const move = params.get("move") ?? undefined;
  const target = params.get("target") ?? undefined;
  const [draft, setDraft] = useState<ExportResponse | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [values, setValues] = useState<Record<string, string>>({});
  const formId = useId();

  const fetchDraft = () =>
    api.exportDraft(id, move ? { move_key: move, target } : {}).then((d) => {
      setDraft(d);
      if (c.step !== "export") setCase({ ...c, step: "export" });
    }, setError);
  const load = () => {
    setError(null);
    setDraft(null);
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

  // The below-bar notice (D31) leads the draft; shown as an amber banner.
  const paras = text.split(/\n\s*\n/);
  const noticeAt = paras.findIndex((p) => /below the confidence bar/i.test(p));
  const notice = noticeAt >= 0 ? paras[noticeAt].replace(/^[>*_\s]+|[*_\s]+$/g, "") : null;
  const body = noticeAt >= 0 ? paras.filter((_, i) => i !== noticeAt).join("\n\n") : text;

  const download = () => {
    const blob = new Blob([text], { type: "text/markdown" });
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = `ruling-request-${id}.md`;
    a.click();
    URL.revokeObjectURL(a.href);
  };

  return (
    <>
      <Main className="grid gap-10 lg:grid-cols-[360px_minmax(0,1fr)]">
        <div className="no-print flex flex-col gap-5">
          <div>
            <p className="eyebrow">Step 4 · Draft for broker</p>
            <h1 className="mt-1 text-[26px] tracking-[-0.05em]">Ruling request</h1>
            <p className="duo mt-1 text-sm text-dim">
              A draft binding-ruling request under 19 C.F.R. Part 177 {move ? "for the chosen move" : "for the current setup"}. Your broker or counsel confirms and expands it before anything is sent to CBP.
            </p>
          </div>
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
        </div>
        <div className="min-w-0">
          {error ? <ErrorBox error={error} onRetry={load} /> : null}
          {!draft && !error && <Spinner label="Drafting the letter…" />}
          {draft && (
            <article className="letter duo rounded-[2px] border border-edge bg-white px-6 py-8 text-[13.5px] leading-relaxed shadow-[4px_4px_0_var(--shadow)] sm:px-12">
              {notice && (
                <p role="note" className="mb-6 border border-ask-line bg-ask-wash px-4 py-3 text-[13px] text-ask">
                  {notice}
                </p>
              )}
              <ReactMarkdown
                components={{
                  h1: (p) => <h1 className="mb-4 text-lg font-medium uppercase tracking-[0.04em]" {...p} />,
                  h2: (p) => <h2 className="mb-2 mt-6 text-[13px] font-medium uppercase tracking-[0.06em]" {...p} />,
                  h3: (p) => <h3 className="mb-2 mt-4 text-[13px] font-medium" {...p} />,
                  p: (p) => <p className="mt-3" {...p} />,
                  ul: (p) => <ul className="mt-2 list-disc pl-5" {...p} />,
                  ol: (p) => <ol className="mt-2 list-decimal pl-5" {...p} />,
                  a: (p) => <a className="text-ok underline underline-offset-2" target="_blank" rel="noreferrer" {...p} />,
                }}
              >
                {body}
              </ReactMarkdown>
              <p className="mt-8 border-t border-line pt-3 text-xs text-dim">{draft.disclaimer}</p>
            </article>
          )}
        </div>
      </Main>
      <Footer>
        <ButtonLink variant="ghost" href={`/cases/${encodeURIComponent(id)}/plan`}>
          ← Back to plan
        </ButtonLink>
        <div className="ml-auto flex flex-wrap gap-2.5">
          <Button variant="ghost" onClick={download} disabled={!draft}>
            Download Markdown
          </Button>
          <Button onClick={() => window.print()} disabled={!draft}>
            Print the letter
          </Button>
        </div>
      </Footer>
    </>
  );
}
