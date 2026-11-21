import ReactMarkdown, { type Components } from "react-markdown";

// The broker letter as Markdown, shared by the Draft step and the broker
// review page. The text is server-generated; react-markdown renders no raw
// HTML and drops unsafe link protocols. Links open in a new tab with no
// referrer, so the review page never sends its address to a ruling site.

// The 19 CFR 177 letter marks what the case doesn't know as [FILL IN: …]
// (D66). Wrapped as inline code so the letter can highlight them.
export const fillIn = /\[FILL IN(?::[^\]]*)?\]/g;

export const letterComponents: Components = {
  h1: (p) => <h1 className="mb-6 text-center text-[17px] font-bold" {...p} />,
  h2: (p) => <h2 className="mb-2 mt-7 text-[15px] font-bold" {...p} />,
  h3: (p) => <h3 className="mb-2 mt-5 text-[15px] font-bold italic" {...p} />,
  p: (p) => <p className="mt-4" {...p} />,
  ul: (p) => <ul className="mt-2 list-disc pl-5" {...p} />,
  ol: (p) => <ol className="mt-2 list-decimal pl-5" {...p} />,
  a: (p) => <a className="text-ok underline underline-offset-2" {...p} target="_blank" rel="noreferrer noopener" />,
  code: ({ children }) => {
    const t = String(children);
    return t.startsWith("[FILL IN") ? (
      <mark className="rounded-[2px] bg-ask-wash px-1 font-sans text-[13px] text-ask">{t}</mark>
    ) : (
      <code className="font-mono text-[12.5px]">{t}</code>
    );
  },
};

export function LetterMarkdown({ body }: { body: string }) {
  return <ReactMarkdown components={letterComponents}>{body.replace(fillIn, (m) => "`" + m + "`")}</ReactMarkdown>;
}

// The reviewer notes beside the letter (screen only, never printed). Each note
// is its own paragraph so the "Legal framework not confirmed" warning is not
// run together with the standing disclaimer; the words are the server's, as
// sent. The below-bar note is the amber one.
export function ReviewNotes({ notes }: { notes: string[] }) {
  if (!notes.length) return null;
  return (
    <div className="no-print mx-auto mb-4 flex max-w-[760px] flex-col gap-2">
      {notes.map((n, i) =>
        /below the confidence bar/i.test(n) ? (
          <p key={i} role="note" className="border border-ask-line bg-ask-wash px-4 py-3 text-[13px] text-ask">
            {n}
          </p>
        ) : /^legal framework not confirmed/i.test(n) ? (
          <p key={i} role="note" className="border border-ask-line bg-ask-wash px-4 py-2.5 text-[12.5px] text-ask">
            {n}
          </p>
        ) : (
          <p key={i} role="note" className="border border-line bg-sheet px-4 py-2.5 text-[12.5px] text-dim">
            {n}
          </p>
        ),
      )}
    </div>
  );
}
