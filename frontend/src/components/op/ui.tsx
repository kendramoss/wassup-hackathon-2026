"use client";

// Drawing-sheet primitives (DESIGN.md step 3): spec-dialog panels with hard
// offset shadows, 2 px corners, the confidence ruler with the 0.92 bar, flags.
import Link from "next/link";
import { useEffect, useRef, type ComponentProps, type ReactNode } from "react";
import { flagCodes } from "@/lib/countries";
import { BAR, conf } from "@/lib/format";

export function cx(...c: (string | false | null | undefined)[]) {
  return c.filter(Boolean).join(" ");
}

// Flags are decorative (alt=""): the country name is always written next to
// them. "Indonesia/Malaysia" shows both; unknown names show none.
export function Flag({ country, size = 11, className }: { country: string | null | undefined; size?: number; className?: string }) {
  const codes = flagCodes(country);
  if (codes.length === 0) return null;
  return (
    <span className={cx("inline-flex shrink-0 gap-1 align-[-1px]", className)}>
      {codes.map((c) => (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          key={c}
          src={`/flags/${c}.svg`}
          alt=""
          height={size}
          width={Math.round((size * 4) / 3)}
          style={{ height: size, width: "auto" }}
          className="rounded-[2px] shadow-[0_0_0_1px_rgba(0,0,0,0.12)]"
        />
      ))}
    </span>
  );
}

export function Country({ name, size, className }: { name: string | null | undefined; size?: number; className?: string }) {
  if (!name) return <span className={className}>—</span>;
  return (
    <span className={cx("inline-flex items-center gap-1.5 whitespace-nowrap", className)}>
      <Flag country={name} size={size} />
      {name}
    </span>
  );
}

type Tone = "ok" | "ask" | "none";

// Confidence ruler: 0, .5, 1, a diamond at the value and a dashed blue line at
// the 0.92 bar.
export function Ruler({
  value,
  label = `${BAR} bar`,
  tone,
  width = 240,
  caption,
}: {
  value: number | null | undefined;
  label?: string;
  tone?: Tone;
  width?: number;
  caption?: ReactNode;
}) {
  const v = value ?? 0;
  const t = tone ?? (v >= BAR ? "ok" : "ask");
  return (
    <div>
      <div
        className="relative mt-4 h-[34px] max-w-full"
        style={{ width }}
        role="img"
        aria-label={`Confidence ${conf(value)} against the ${BAR} bar`}
      >
        <div className="absolute inset-x-0 top-[14px] border-t border-ink" />
        {[0, 0.5, 1].map((x) => (
          <div key={x} className="absolute top-[9px] h-[10px] border-l border-ink" style={{ left: `${x * 100}%` }}>
            <span className="absolute left-[-10px] top-[13px] text-[11px] text-dim">{x === 0.5 ? ".5" : x}</span>
          </div>
        ))}
        {value !== null && value !== undefined && (
          <div
            className={cx("absolute top-[9px] size-[10px] -translate-x-[5px] rotate-45", t === "ok" ? "bg-ok" : t === "ask" ? "border-[1.5px] border-ink bg-sheet" : "bg-dim")}
            style={{ left: `${v * 100}%` }}
          />
        )}
        <div className="absolute top-0 h-[28px] border-l-[1.5px] border-dashed border-ok" style={{ left: `${BAR * 100}%` }}>
          <b className="absolute right-1 top-[-16px] whitespace-nowrap text-[11px] font-medium text-ok">{label}</b>
        </div>
      </div>
      {caption && <div className="mt-0.5 text-xs text-dim">{caption}</div>}
    </div>
  );
}

export function MiniRuler({ value }: { value: number | null | undefined }) {
  if (value === null || value === undefined) return <span className="text-dim">—</span>;
  return (
    <div className="relative mt-1 h-4 w-[120px]" aria-hidden="true">
      <div className="absolute inset-x-0 top-2 border-t border-ink" />
      <div className="absolute top-0 h-4 border-l-[1.5px] border-dashed border-ok" style={{ left: `${BAR * 100}%` }} />
      <div className={cx("absolute top-1 size-2 -translate-x-1 rotate-45", value >= BAR ? "bg-ok" : "border-[1.5px] border-ink bg-sheet")} style={{ left: `${value * 100}%` }} />
    </div>
  );
}

export function Panel({ className, tone, ...props }: ComponentProps<"section"> & { tone?: "ask" }) {
  return (
    <section
      className={cx(
        "rounded-[2px] border bg-sheet p-5",
        tone === "ask" ? "border-ask-line shadow-[4px_4px_0_var(--ask-shadow)]" : "border-edge shadow-[4px_4px_0_var(--shadow)]",
        className,
      )}
      {...props}
    />
  );
}

export function PanelHead({ children, right }: { children: ReactNode; right?: ReactNode }) {
  return (
    <div className="mb-3.5 flex justify-between gap-3 text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
      <h2 className="font-medium">◆ {children}</h2>
      {right && <span className="text-ok">{right}</span>}
    </div>
  );
}

// An accordion for details people may want but don't need to act: a ▸ row
// with a title and a short count or status, ruled off from its neighbours.
export function Fold({ title, meta, open, id, children }: { title: ReactNode; meta?: ReactNode; open?: boolean; id?: string; children: ReactNode }) {
  return (
    <details id={id} open={open} className="group border-b border-line first-of-type:border-t">
      <summary className="flex min-h-11 list-none items-center gap-2.5 text-[13px] text-ink hover:text-ok [&::-webkit-details-marker]:hidden">
        <span aria-hidden="true" className="inline-block w-3 text-dim transition-transform group-open:rotate-90">
          ▸
        </span>
        <span className="font-medium">{title}</span>
        {meta ? <span className="text-dim">· {meta}</span> : null}
      </summary>
      <div className="pb-4 pl-5.5">{children}</div>
    </details>
  );
}

const btn = {
  primary: "bg-ink text-paper border-ink shadow-[3px_3px_0_var(--shadow)] hover:bg-body",
  ghost: "bg-sheet text-ink border-line hover:bg-wash",
  off: "bg-wash text-dim border-line cursor-not-allowed",
};

type BtnProps = { variant?: "primary" | "ghost"; className?: string };

export function Button({ variant = "primary", className, disabled, ...props }: ComponentProps<"button"> & BtnProps) {
  // aria-disabled looks disabled but stays focusable, so a keyboard or
  // screen-reader user can reach it and hear why (#100).
  const off = disabled || props["aria-disabled"] === true || props["aria-disabled"] === "true";
  return (
    <button
      type="button"
      disabled={disabled}
      className={cx(
        "inline-flex min-h-10 items-center justify-center gap-1.5 rounded-[2px] border px-4 py-2 text-sm font-medium",
        off ? btn.off : btn[variant],
        className,
      )}
      {...props}
    />
  );
}

export function ButtonLink({ variant = "primary", className, ...props }: ComponentProps<typeof Link> & BtnProps) {
  return (
    <Link
      className={cx(
        "inline-flex min-h-10 items-center justify-center gap-1.5 rounded-[2px] border px-4 py-2 text-sm font-medium",
        btn[variant],
        className,
      )}
      {...props}
    />
  );
}

export function Chip({ tone, children, className }: { tone?: "ok" | "ask" | "fail"; children: ReactNode; className?: string }) {
  return (
    <span
      className={cx(
        "inline-flex items-center gap-1 rounded-[2px] border px-2 py-0.5 text-xs",
        !className?.includes("whitespace-") && "whitespace-nowrap",
        tone === "ok" && "border-ok bg-ok-wash text-ok",
        tone === "ask" && "border-ask-line bg-ask-wash text-ask",
        tone === "fail" && "border-fail bg-fail-wash text-fail",
        !tone && "border-line bg-sheet",
        className,
      )}
    >
      {children}
    </span>
  );
}

export function StatusChip({ route }: { route: string | null | undefined }) {
  if (route === "auto") return <Chip tone="ok">Auto-accepted</Chip>;
  if (route === "needs_review") return <Chip tone="ask">△ Needs expert review</Chip>;
  if (route === "needs_info") return <Chip tone="ask">△ Needs more facts</Chip>;
  return <Chip>Draft</Chip>;
}

export function Dimension({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cx("text-[11px] tracking-[0.02em] text-dimension", className)}>{children}</span>;
}

// A horizontally scrolling table wrapper that keyboard users can scroll.
export function ScrollX({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div role="region" aria-label={label} tabIndex={0} className="relative max-w-full overflow-x-auto">
      {children}
    </div>
  );
}

export function Spinner({ label }: { label: string }) {
  return (
    <span role="status" className="inline-flex items-center gap-2 text-sm text-dim">
      <span className="size-2 animate-pulse rotate-45 bg-ok" aria-hidden="true" />
      {label}
    </span>
  );
}

export function RulingLink({ id, className }: { id: string; className?: string }) {
  return (
    <a
      href={`https://rulings.cbp.gov/ruling/${encodeURIComponent(id)}`}
      target="_blank"
      rel="noreferrer"
      className={cx("font-mono text-ok underline decoration-1 underline-offset-2", className)}
    >
      {id}
    </a>
  );
}

// A right-hand drawer that is a real modal: full height above everything
// (no half-covered action bar), a dimmed backdrop that closes it, focus kept
// inside while open and returned to the opener on close (WCAG 2.4.3, 2.1.2).
export function Drawer({ label, onClose, width = 540, header, footer, children }: { label: string; onClose: () => void; width?: number; header: ReactNode; footer?: ReactNode; children: ReactNode }) {
  const panel = useRef<HTMLDivElement>(null);
  // Screens pass a new onClose each render; keep the latest without
  // re-running the open/close effect (which moves focus).
  const close = useRef(onClose);
  useEffect(() => {
    close.current = onClose;
  });
  useEffect(() => {
    const opener = document.activeElement as HTMLElement | null;
    const first = panel.current?.querySelector<HTMLElement>("button, a[href], input, textarea, select");
    first?.focus();
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") close.current();
      if (e.key !== "Tab" || !panel.current) return;
      const f = [...panel.current.querySelectorAll<HTMLElement>("button:not([disabled]), a[href], input:not([disabled]), textarea, select, summary")];
      if (f.length === 0) return;
      if (e.shiftKey && document.activeElement === f[0]) {
        e.preventDefault();
        f[f.length - 1].focus();
      } else if (!e.shiftKey && document.activeElement === f[f.length - 1]) {
        e.preventDefault();
        f[0].focus();
      }
    };
    document.addEventListener("keydown", key);
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", key);
      document.body.style.overflow = "";
      opener?.focus();
    };
  }, []);
  return (
    <div className="fixed inset-0 z-50">
      <button type="button" aria-label="Close" tabIndex={-1} className="absolute inset-0 h-full w-full cursor-default bg-ink/30" onClick={onClose} />
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-label={label}
        className="absolute inset-y-0 right-0 flex w-full flex-col border-l border-edge bg-sheet shadow-[-4px_0_0_var(--shadow)]"
        style={{ maxWidth: width }}
      >
        <div className="flex items-center gap-3 border-b border-dashed border-guide px-5 py-3.5 text-[11px] uppercase tracking-[0.08em] text-dim">
          {header}
          <button type="button" aria-label="Close" className="ml-auto inline-grid size-8 place-items-center text-base text-dim hover:text-ink" onClick={onClose}>
            ✕
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto">{children}</div>
        {footer}
      </div>
    </div>
  );
}
