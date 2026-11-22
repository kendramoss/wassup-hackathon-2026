"use client";

// "What your broker sees" (D220): on a pinned demo no review link can be made,
// so the Draft step shows the /review page itself, filled from the draft on
// screen. No token exists, nothing is sent and no decision is recorded; the
// same components render it, so it can't drift from the real page.
import { useEffect, useRef, useState } from "react";
import type { CaseResponse, ExportResponse, Plan } from "@/lib/api/client";
import { DISCLAIMER, type BrokerPacket } from "@/lib/broker-review";
import { Packet } from "./screens/broker-review-page";
import { Button } from "./ui";

// The packet the server would freeze for this draft, built from the case as
// shown: the plan's prediction and parts after the move when a plan is
// drafted, the cited rulings with what the case knows about them.
export function previewPacket(c: CaseResponse, draft: ExportResponse, plan: Plan | undefined, label: string, now = new Date()): BrokerPacket {
  const pred = plan?.prediction ?? c.prediction ?? {};
  const plants = new Map((c.product?.plants ?? []).map((p) => [p.id, p.country]));
  const moved = new Map((plan?.move ?? []).map((m) => [m.group, plants.get(m.to_plant ?? "")]));
  const components = (c.product?.components ?? []).map((x) => ({ name: x.name, made_in: (x.group && moved.get(x.group)) || x.made_in }));
  const cited = new Set(pred.cited_rulings ?? []);
  const known = new Map((c.precedents_detail ?? []).map((r) => [r.id, r]));
  const ids = [plan?.verdict?.supporting_ruling, plan?.verdict?.counter_ruling, ...cited].filter((x): x is string => Boolean(x));
  const rulings = [...new Set(ids)].map((id) => {
    const r = known.get(id);
    return { id, url: r?.url, date: r?.date ?? undefined, subject: r?.subject ?? undefined, origin: r?.origin ?? undefined };
  });
  const later = new Date(now.getTime() + 14 * 864e5);
  return {
    review: { id: "preview", status: "pending", subject: plan ? "plan" : "setup", draft_label: label, format: draft.format, created_at: now.toISOString(), expires_at: later.toISOString() },
    product_name: c.product?.name ?? c.display_name ?? "",
    product: c.product ? { name: c.product.name, components } : null,
    prediction: pred,
    cited_rulings: rulings,
    plan: plan
      ? {
          description: plan.description,
          predicted_origin: plan.prediction?.origin_country,
          supporting_ruling: plan.verdict?.supporting_ruling ?? undefined,
          counter_ruling: plan.verdict?.counter_ruling ?? undefined,
          check_confidence: plan.verdict?.confidence ?? undefined,
          below_bar: !plan.shown && plan.rejection_kind === "below_confidence_bar",
        }
      : null,
    draft: { format: draft.format, content: draft.content, notes: draft.notes },
    disclaimer: DISCLAIMER,
  };
}

export function BrokerPreview({ c, draft, plan, label }: { c: CaseResponse; draft: ExportResponse | null; plan: Plan | undefined; label: string }) {
  const [open, setOpen] = useState(false);
  const close = useRef<HTMLButtonElement>(null);
  const opener = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    close.current?.focus();
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        return;
      }
      if (e.key !== "Tab" || !panel.current) return;
      // The same containment Drawer gives every other modal (#534): with
      // aria-modal=true, Tab must not reach the page the overlay covers.
      const f = [
        ...panel.current.querySelectorAll<HTMLElement>('button:not([disabled]), a[href], input:not([disabled]), textarea:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'),
      ];
      if (f.length === 0) return;
      if (!panel.current.contains(document.activeElement)) {
        e.preventDefault();
        f[0].focus();
      } else if (e.shiftKey && document.activeElement === f[0]) {
        e.preventDefault();
        f[f.length - 1].focus();
      } else if (!e.shiftKey && document.activeElement === f[f.length - 1]) {
        e.preventDefault();
        f[0].focus();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
    };
  }, [open]);
  useEffect(() => {
    if (!open) opener.current?.focus({ preventScroll: true });
  }, [open]);
  return (
    <div className="flex flex-col gap-1.5">
      <Button ref={opener} variant="ghost" disabled={!draft} onClick={() => setOpen(true)}>
        Preview what your broker sees
      </Button>
      {open && draft && (
        <div ref={panel} role="dialog" aria-modal="true" aria-label="Preview of the broker review page" className="fixed inset-0 z-50 overflow-y-auto bg-paper">
          <div className="sticky top-0 z-10 flex min-h-14 flex-wrap items-center gap-x-4 gap-y-1 border-b border-line bg-paper px-4 py-2 sm:px-10">
            <span className="text-[13px] font-medium">Preview · what your broker sees</span>
            <span className="text-xs text-dim">No link exists for this demo. Nothing here is sent or recorded.</span>
            <Button ref={close} variant="ghost" className="ml-auto" onClick={() => setOpen(false)}>
              Close preview
            </Button>
          </div>
          <div className="mx-auto w-full max-w-[880px] px-4 pb-20 pt-8 sm:px-10">
            <Packet p={previewPacket(c, draft, plan, label)} token="" preview onPacket={() => {}} onGone={() => {}} />
          </div>
        </div>
      )}
    </div>
  );
}
