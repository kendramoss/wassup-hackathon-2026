"use client";

import { Fragment, useEffect, useId, useRef, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { copy } from "@/lib/copy";

type Pos = { left: number; top: number; width: number; above: boolean };

// A term a first-time reader may not know (#232): dotted underline, and a
// short explanation on hover, keyboard focus or tap. Screen readers get it as
// the term's description. The bubble is portalled to the body so sheets with
// overflow: hidden (the landing figures) don't clip it.
export function Term({ children, explain }: { children: ReactNode; explain: string }) {
  const id = useId();
  const ref = useRef<HTMLSpanElement>(null);
  const [pos, setPos] = useState<Pos | null>(null);

  const show = () => {
    const r = ref.current?.getBoundingClientRect();
    if (!r) return;
    const width = Math.min(280, innerWidth - 16);
    const above = r.bottom + 110 > innerHeight && r.top > 120;
    setPos({ left: Math.min(Math.max(8, r.left), innerWidth - width - 8), top: above ? r.top - 6 : r.bottom + 6, width, above });
  };
  const hide = () => setPos(null);

  useEffect(() => {
    if (!pos) return;
    const esc = (e: KeyboardEvent) => e.key === "Escape" && hide();
    addEventListener("keydown", esc);
    addEventListener("scroll", hide, true);
    addEventListener("resize", hide);
    return () => {
      removeEventListener("keydown", esc);
      removeEventListener("scroll", hide, true);
      removeEventListener("resize", hide);
    };
  }, [pos]);

  return (
    <>
      <span
        ref={ref}
        tabIndex={0}
        aria-describedby={id}
        className="cursor-help underline decoration-dim decoration-dotted decoration-1 underline-offset-[3px] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ok"
        onMouseEnter={show}
        onMouseLeave={hide}
        onFocus={show}
        onBlur={hide}
        // A tap focuses and clicks at once, so a click only opens; tapping
        // elsewhere (blur) or Escape closes it.
        onClick={(e) => {
          e.stopPropagation();
          show();
        }}
      >
        {children}
      </span>
      <span id={id} className="sr-only">
        {explain}
      </span>
      {pos &&
        createPortal(
          <span
            aria-hidden="true"
            className="read pointer-events-none fixed z-[70] block border border-edge bg-sheet px-3 py-2 text-left text-[12.5px] font-normal normal-case leading-snug tracking-normal text-ink shadow-[3px_3px_0_var(--shadow)]"
            style={{ left: pos.left, top: pos.top, width: pos.width, transform: pos.above ? "translateY(-100%)" : undefined }}
          >
            {explain}
          </span>,
          document.body,
        )}
    </>
  );
}

export type TermKey = keyof typeof copy.terms;

// Wraps the first occurrence of each named term in text. Use once per term
// per screen, at its first visible place.
export function Explained({ text, terms }: { text: string; terms: TermKey[] }) {
  const hits = terms
    .map((k) => {
      const m = copy.terms[k].match.exec(text);
      return m ? { k, at: m.index, len: m[0].length } : null;
    })
    .filter((h): h is { k: TermKey; at: number; len: number } => h !== null)
    .sort((a, b) => a.at - b.at)
    .filter((h, i, a) => i === 0 || h.at >= a[i - 1].at + a[i - 1].len);
  if (!hits.length) return <>{text}</>;
  const out: ReactNode[] = [];
  let i = 0;
  for (const h of hits) {
    out.push(<Fragment key={`t${h.at}`}>{text.slice(i, h.at)}</Fragment>);
    out.push(
      <Term key={h.k} explain={copy.terms[h.k].text}>
        {text.slice(h.at, h.at + h.len)}
      </Term>,
    );
    i = h.at + h.len;
  }
  out.push(<Fragment key="end">{text.slice(i)}</Fragment>);
  return <>{out}</>;
}
