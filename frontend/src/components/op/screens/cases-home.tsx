"use client";

import { watchView } from "@/lib/watch-state";
import { badgeText } from "@/lib/ruling-alerts";
import Link from "next/link";
import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { api, type CaseResponse, type CaseSummary, type DemoList } from "@/lib/api/client";
import { VIEWER_REASON, caseReadOnly } from "@/lib/roles";
import { copy } from "@/lib/copy";
import { BAR, conf, formatDate, originConf, originConfCaption, stepIndex, stepNames } from "@/lib/format";
import { currentOrigin } from "../case-shell";
import { netText } from "../ops-view";
import { backtestMatch, checkWords, movePhrase, type ActualChangeExtras } from "../plan-bits";
import { ErrorBox, Main } from "../chrome";
import { Button, ButtonLink, Chip, Country, ScrollX, Spinner, StatusChip, cx } from "../ui";
import { WorkspacePanel } from "../workspace-panel";

const caseHref = (c: Pick<CaseSummary, "id" | "step">) =>
  `/cases/${encodeURIComponent(c.id)}/${["product", "product", "origin", "plan", "draft"][stepIndex[c.step] ?? 2]}`;

// Cases home (DESIGN.md, locked 16 Oct, option 2): pinned demos as cards,
// then "Your cases" as a table with a ⋯ menu (Open, Rename, Delete).
export function CasesHome() {
  const [cases, setCases] = useState<CaseSummary[] | null>(null);
  const [demos, setDemos] = useState<DemoList["demos"]>([]);
  const [error, setError] = useState<unknown>(null);

  const load = () => {
    setError(null);
    api.listCases().then((r) => setCases(r.cases), setError);
  };
  useEffect(() => {
    api.listCases().then((r) => setCases(r.cases), setError);
    api.demos().then((r) => setDemos(r.demos ?? []), () => setDemos([]));
  }, []);

  const pinned = cases?.filter((c) => c.pinned) ?? [];
  const yours = cases?.filter((c) => !c.pinned) ?? [];
  // #412: trust the server's demo_id when present (older seeds only expose
  // the display name, so keep that as a fallback rather than hiding cards).
  const pinnedTitles = new Set(pinned.map((c) => c.display_name));
  const unseeded =
    pinned.length > 0
      ? (demos ?? []).filter(
          (d) =>
            d.title &&
            !pinned.some((c) =>
              c.demo_id ? d.id === c.demo_id : pinnedTitles.has(d.title ?? ""),
            ),
        )
      : [];

  return (
    <Main>
      <div className="flex flex-wrap items-end gap-3">
        <div>
          <h1 className="text-[28px] tracking-[-0.05em]">{copy.cases.title}</h1>
          <p className="duo mt-1 text-dim">Predict the origin CBP would likely assign, then plan a move that earlier rulings support.</p>
        </div>
        <div className="ml-auto flex flex-wrap gap-2">
          <ButtonLink href="/triage" variant="ghost">
            Triage a portfolio
          </ButtonLink>
        </div>
      </div>

      {error ? (
        <div className="mt-6">
          <ErrorBox error={error} onRetry={load} />
        </div>
      ) : null}
      {cases === null && !error && (
        <div className="mt-8">
          <Spinner label="Loading cases…" />
        </div>
      )}

      {pinned.length > 0 && (
        <section className="mt-8" aria-labelledby="demos-h">
          <h2 id="demos-h" className="eyebrow mb-3">
            Demos · real CBP facts
          </h2>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {pinned.map((c) => (
              <DemoCard key={c.id} c={c} demo={c.demo_id ? demos?.find((d) => d.id === c.demo_id) : undefined} />
            ))}
          </div>
          <p className="read mt-3 max-w-[90ch] text-[12.5px] text-dim">
            {copy.cases.demosNote}{" "}
            <Link href="/accuracy#misses" className="text-ok underline underline-offset-2">
              {copy.cases.demosMisses}
            </Link>
          </p>
          {/* Demos with no pinned case yet (a newer product family, judge
              journey): offered to load and run, so they are not hidden
              behind the seeded ones. */}
          {unseeded.length > 0 && (
            <div className="mt-6">
              <h3 className="eyebrow mb-1">{copy.cases.moreDemos}</h3>
              <p className="read mb-3 max-w-[90ch] text-[12.5px] text-dim">{copy.cases.moreDemosNote} {copy.cases.recorded.selection}</p>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
                {unseeded.map((d) => (
                  <LoadDemoCard key={d.id} d={d} />
                ))}
              </div>
            </div>
          )}
        </section>
      )}

      {cases && pinned.length === 0 && demos && demos.length > 0 && (
        <section className="mt-8" aria-labelledby="load-h">
          <h2 id="load-h" className="eyebrow mb-3">
            {copy.cases.demos}
          </h2>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {demos.map((d) => (
              <LoadDemoCard key={d.id} d={d} />
            ))}
          </div>
          <p className="read mt-3 max-w-[90ch] text-[12.5px] text-dim">
            {copy.cases.demosNote}{" "}
            <Link href="/accuracy#misses" className="text-ok underline underline-offset-2">
              {copy.cases.demosMisses}
            </Link>
          </p>
        </section>
      )}
      {cases && (
        <section className="mt-12" aria-labelledby="yours-h">
          <h2 id="yours-h" className="eyebrow mb-3">
            Your cases {yours.length ? `· ${yours.length}` : ""}
          </h2>
          {yours.length === 0 ? (
            <p className="duo border border-dashed border-guide bg-sheet px-5 py-4 text-dim">
              {copy.cases.empty}{" "}
              <Link href="/cases/new" className="text-ok underline underline-offset-2">
                {copy.cases.newCase}
              </Link>
              {pinned.length > 0 || (demos?.length ?? 0) > 0 ? ", or open a demo above." : null}
            </p>
          ) : (
            <YourCases cases={yours} onChange={(next) => setCases([...pinned, ...next])} />
          )}
        </section>
      )}
      <WorkspacePanel />
    </Main>
  );
}

// A demo not yet run here (no pinned case): the same card shape as DemoCard,
// with what CBP held as the headline instead of a prediction.
export function LoadDemoCard({ d }: { d: NonNullable<DemoList["demos"]>[number] }) {
  // cbp_held reads "China (N328954)" or "Thailand (N359371, scenario I)".
  const held = d.cbp_held ?? "";
  const cut = held.indexOf(" (");
  const m = held ? [held, cut < 0 ? held : held.slice(0, cut), cut < 0 ? "" : held.slice(cut + 2).replace(/\)$/, "")] : null;
  // Not one big link: the card holds its own link to the committed run, and
  // a link inside a link is invalid HTML. The title link stretches over the
  // card (after:inset-0) so the whole card still opens the demo; the run link
  // sits above it (relative z-10).
  return (
    <article className="relative flex min-w-0 flex-col gap-2.5 rounded-[2px] border border-edge bg-sheet p-4 text-sm shadow-[4px_4px_0_var(--shadow)] hover:bg-white">
      <span className="flex items-start justify-between gap-2">
        <Link
          href={`/cases/new?demo=${encodeURIComponent(d.id ?? "")}`}
          className="font-medium leading-snug after:absolute after:inset-0 after:rounded-[2px] focus-visible:outline-none focus-visible:after:outline-2 focus-visible:after:outline-offset-2 focus-visible:after:outline-accent"
        >
          {d.title}
        </Link>
        <Chip className="shrink-0">Demo</Chip>
      </span>
      {m && (
        <span className="flex flex-col gap-1">
          <span className="eyebrow">CBP held</span>
          <span className="text-[26px] leading-none tracking-[-0.05em]">
            <Country name={m[1]} size={18} />
          </span>
          {m[2] && <span className="text-xs text-dim">{m[2]}</span>}
        </span>
      )}
      {d.description && <span className="duo line-clamp-2 text-xs text-dim" title={d.description}>{d.description}</span>}
      {d.recorded && <RecordedRun r={d.recorded} />}
      <span className="mt-auto flex flex-wrap items-center justify-between gap-2 border-t border-line pt-2 text-xs">
        <span className="text-ok">Load demo →</span>
        <Chip tone="ask" className="whitespace-normal">
          {copy.cases.recorded.unseeded}
        </Chip>
        {d.has_plan && <Chip>{copy.cases.recorded.planRuns}</Chip>}
      </span>
    </article>
  );
}

// A demo's recorded development run (D191): the engine's answer beside CBP's
// holding, shown whether or not it agreed, so a disagreement is visible
// before anyone runs the demo here.
function RecordedRun({ r }: { r: NonNullable<NonNullable<DemoList["demos"]>[number]["recorded"]> }) {
  const T = copy.cases.recorded;
  return (
    <span className="flex flex-col gap-1 border-t border-line pt-2 text-xs" title={`${r.run}. ${r.file}`}>
      <span className="eyebrow">{T.label}</span>
      <span className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span>
          {T.engine} <Country name={r.engine_origin} />
        </span>
        <span className="text-dim">{originConfCaption(originConf({ confidence: r.stated_confidence, calibrated_confidence: r.calibrated_confidence }))}</span>
        <Chip tone={r.agrees_with_cbp ? "ok" : "fail"}>{r.agrees_with_cbp ? T.agrees : T.disagrees}</Chip>
      </span>
      {r.best_plan ? (
        <span className="text-dim">
          {T.plan(r.best_plan_words ? r.best_plan_words.replace(/^Move /, "move ") : r.best_plan, conf(r.plan_check), r.plan_ruling ?? "")}
          {r.best_plan_includes?.length ? <span className="mt-0.5 block">{T.includes(r.best_plan_includes)}</span> : null}
        </span>
      ) : (
        <span className="text-dim">{T.noPlan}</span>
      )}
      {r.file && (
        <a
          href={`https://github.com/kendramoss/wassup-hackathon-2026/blob/main/${encodeURI(r.file)}`}
          target="_blank"
          rel="noreferrer"
          className="relative z-10 self-start text-ok underline underline-offset-2"
        >
          {T.recordLink}
        </a>
      )}
    </span>
  );
}

// The card's demo entry, when known, gives what CBP held for a demo whose
// case has no current origin recorded (the dryer latches, judge panel round 3).
const heldOf = (s?: string | null) => {
  const h = (s ?? "").trim();
  const cut = h.indexOf(" (");
  return h ? { country: cut < 0 ? h : h.slice(0, cut), ruling: cut < 0 ? undefined : h.slice(cut + 2).replace(/\)$/, "") } : undefined;
};

export function DemoCard({ c, demo }: { c: CaseSummary; demo?: NonNullable<DemoList["demos"]>[number] }) {
  const [full, setFull] = useState<CaseResponse | null>(null);
  useEffect(() => {
    api.getCase(c.id).then(setFull, () => {});
  }, [c.id]);
  // The demo's record of CBP's real change, so the card's "matches" reads
  // the kept-in-place parts the same way the plan step does (#519).
  const [change, setChange] = useState<ActualChangeExtras | null>(null);
  useEffect(() => {
    if (!demo?.id || !demo.has_plan) return;
    let live = true;
    api.demo(demo.id).then(
      (d) => {
        if (live) setChange((d?.plan?.actual_change as ActualChangeExtras | undefined) ?? null);
      },
      () => {},
    );
    return () => {
      live = false;
    };
  }, [demo?.id, demo?.has_plan]);
  const own = full ? currentOrigin(full) : undefined;
  const cur = own?.country ? own : heldOf(demo?.cbp_held);
  const best = full?.plan?.plans?.[0];
  const match = backtestMatch(full?.plan?.backtest, { change, product: full?.product, move: best?.move });
  const compared = (full as (CaseResponse & { targets?: { ranked?: unknown[] } }) | null)?.targets?.ranked?.length ?? 0;
  const differs = Boolean(cur?.country && c.predicted_origin && cur.country.toLowerCase() !== c.predicted_origin.toLowerCase());
  return (
    <Link href={caseHref(c)} className="flex flex-col gap-2.5 rounded-[2px] border border-edge bg-sheet p-4 text-sm shadow-[4px_4px_0_var(--shadow)] hover:bg-white">
      <span className="flex items-start justify-between gap-2">
        <b className="font-medium leading-snug">{c.display_name}</b>
        <Chip className="shrink-0">Demo</Chip>
      </span>
      <AlertBadge n={c.ruling_alerts} />
      <span className="text-[26px] leading-none tracking-[-0.05em]">
        <Country name={c.predicted_origin} size={18} />
      </span>
      <span className="flex flex-wrap items-center gap-2">
        <StatusChip route={c.triage_route} />
        <span className="text-xs text-dim">{originConfCaption(originConf(c))}</span>
      </span>
      {cur?.country && (
        <span className="text-xs text-dim">
          CBP held <Country name={cur.country} />
          {cur.ruling ? `, ${cur.ruling}` : ""}
          {differs ? ". Unde's prediction differs; the plan starts from CBP's holding." : ""}
        </span>
      )}
      {/* What the prediction read (D220): the same setup can give two answers
          from CBP's text and from the parts list. */}
      {full?.facts_source && (
        <span className="text-xs text-dim">
          {full.facts_source === "pasted"
            ? "Unde read CBP's own text for this setup; scenarios and plans read its parts list."
            : "Unde read the structured parts list, not CBP's text. From CBP's text the same setup can get another answer."}
        </span>
      )}
      {c.date && <span className="text-xs text-dim">{copy.cases.demoDate(formatDate(c.date))}</span>}
      <span className="duo mt-auto border-t border-line pt-2 text-xs">
        {best ? (
          <>
            <span className="text-ok">Plan:</span> move {movePhrase(best)} to {full?.plan?.target} · check {conf(best.verdict?.confidence)} ({checkWords(best.verdict?.confidence)}) ·{" "}
            {full?.plan_economics_stale ? (
              copy.stale.title.toLowerCase()
            ) : (
              <>
                {netText(best)}
                {best.operations ? "" : "/yr"} {copy.plan.illustrativeShort}
                {best.operations ? " · modelled terms only" : ""}
              </>
            )}
            {match.matches ? <> · matches CBP&apos;s real change{match.ruling ? ` (${match.ruling})` : ""}</> : null}
          </>
        ) : full?.plan ? (
          <span className="text-ask">No plan cleared the {BAR} precedent-check bar</span>
        ) : compared > 0 ? (
          <span className="text-ok">Country comparison recorded: {compared} countries</span>
        ) : full && !full.product ? (
          <span className="text-dim">{copy.cases.recorded.originOnly}</span>
        ) : (
          <span className="text-dim">No plan search yet</span>
        )}
      </span>
    </Link>
  );
}

export function YourCases({ cases, onChange }: { cases: CaseSummary[]; onChange: (c: CaseSummary[]) => void }) {
  const [renaming, setRenaming] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<string | null>(null);
  const [rowError, setRowError] = useState<{ id: string; error: unknown } | null>(null);
  // #605: the ⋯ trigger unmounts with the row it replaces, so remember which
  // one to focus again when the confirmation is cancelled.
  const restoreFocusRef = useRef<string | null>(null);
  useEffect(() => {
    if (confirming || !restoreFocusRef.current) return;
    const label = `Actions for ${restoreFocusRef.current}`;
    restoreFocusRef.current = null;
    [...document.querySelectorAll<HTMLButtonElement>('button[aria-label^="Actions for "]')]
      .find((button) => button.getAttribute("aria-label") === label)
      ?.focus();
  }, [confirming]);

  const rename = async (id: string, name: string) => {
    const old = cases.find((c) => c.id === id)?.display_name;
    setRenaming(null);
    if (!name.trim() || name.trim() === old) return;
    onChange(cases.map((c) => (c.id === id ? { ...c, display_name: name.trim() } : c)));
    try {
      await api.rename(id, name.trim());
    } catch (e) {
      onChange(cases.map((c) => (c.id === id ? { ...c, display_name: old ?? c.display_name } : c)));
      setRowError({ id, error: e });
    }
  };
  const remove = async (id: string) => {
    try {
      await api.deleteCase(id);
      setConfirming(null);
      onChange(cases.filter((c) => c.id !== id));
    } catch (e) {
      setRowError({ id, error: e });
    }
  };

  // Below 640 px each row stacks (judge panel round 10, N7): the header row
  // is hidden, every cell gives its column name, and nothing scrolls
  // sideways. From 640 px up it is the table it was.
  const col = copy.cases.columns;
  return (
    <ScrollX label="Your cases">
      <table data-stacks-below="sm" className="w-full border-collapse border border-edge bg-sheet text-[13px] shadow-[4px_4px_0_var(--shadow)] max-sm:block sm:min-w-[760px]">
        <thead className="max-sm:hidden">
          <tr>
            {[...copy.cases.columns, ""].map((h, i) => (
              <th key={i} scope="col" className="border-b border-line bg-paper px-3.5 py-2.5 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
                {h || <span className="sr-only">Actions</span>}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="max-sm:block">
          {cases.map((c) => {
            const n = stepIndex[c.step] ?? 1;
            if (confirming === c.id)
              return (
                <tr
                  key={c.id}
                  className="border-b border-line bg-fail-wash max-sm:block"
                  onKeyDown={(e) => {
                    if (e.key === "Escape") {
                      e.preventDefault();
                      setConfirming(null);
                    }
                  }}
                >
                  <td colSpan={7} className="px-3.5 py-3 max-sm:block">
                    <span className="duo text-fail">
                      Delete &ldquo;{c.display_name}&rdquo;? Its prediction, plans and drafts are removed.
                    </span>
                    <span className="ml-3 inline-flex gap-2">
                      <Button className="min-h-8 border-fail bg-fail px-3 py-1 text-[12.5px] text-white shadow-none" onClick={() => remove(c.id)}>
                        Delete case
                      </Button>
                      <Button variant="ghost" autoFocus className="min-h-8 px-3 py-1 text-[12.5px]" onClick={() => setConfirming(null)}>
                        Cancel
                      </Button>
                    </span>
                  </td>
                </tr>
              );
            return (
              <tr key={c.id} className="border-b border-line last:border-0 hover:bg-paper max-sm:relative max-sm:grid max-sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] max-sm:gap-x-3 max-sm:gap-y-2 max-sm:px-3.5 max-sm:py-3">
                <td className="px-3.5 py-3 max-sm:col-span-2 max-sm:p-0 max-sm:pr-10">
                  {renaming === c.id ? (
                    <RenameField value={c.display_name} onDone={(v) => (v === null ? setRenaming(null) : rename(c.id, v))} />
                  ) : (
                    <Link href={caseHref(c)} className="block font-medium hover:underline">
                      {c.display_name}
                    </Link>
                  )}
                  <span className="text-xs text-dim">Case date {formatDate(c.date)}</span>
                  <AlertBadge n={c.ruling_alerts} />
                  {rowError?.id === c.id && (
                    <span role="alert" className="block text-xs text-fail">
                      {(rowError.error as Error).message}
                    </span>
                  )}
                </td>
                <td data-label={col[1]} className={cx(stack, "text-[17px] tracking-[-0.03em]")}>
                  {c.predicted_origin ? <Country name={c.predicted_origin} size={13} /> : <span className="text-sm text-dim">{copy.cases.notAnalysed}</span>}
                </td>
                <td data-label={col[2]} className={cx(stack, "max-sm:[&_span]:whitespace-normal")}>
                  <StatusChip route={c.triage_route} />
                </td>
                <td data-label={col[3]} className={cx(stack, "text-xs", watchOf(c).attention ? "font-medium text-ask" : c.watch_status ? "text-ok" : "text-dim")}>
                  {watchOf(c).label}
                </td>
                <td data-label={col[4]} className={stack}>{c.scenario_count || "—"}</td>
                <td data-label={col[5]} className={cx(stack, "text-dim")}>
                  Step {n} of 4 · {stepNames[n - 1]}
                </td>
                <td className="px-2 py-3 text-right max-sm:absolute max-sm:right-1.5 max-sm:top-2 max-sm:p-0">
                  <RowMenu
                    c={c}
                    onRename={() => setRenaming(c.id)}
                    onDelete={() => {
                      restoreFocusRef.current = c.display_name;
                      setConfirming(c.id);
                    }}
                  />
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </ScrollX>
  );
}

// A data cell: a table cell from 640 px up; below it a block that names its
// column above the value (the header row is hidden there).
const stack =
  "px-3.5 py-3 max-sm:block max-sm:p-0 max-sm:before:block max-sm:before:text-[11px] max-sm:before:font-medium max-sm:before:uppercase max-sm:before:tracking-[0.08em] max-sm:before:text-dim max-sm:before:content-[attr(data-label)]";

function RenameField({ value, onDone }: { value: string; onDone: (v: string | null) => void }) {
  const [v, setV] = useState(value);
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    ref.current?.select();
  }, []);
  return (
    <input
      ref={ref}
      aria-label="Case name (Enter saves, Esc cancels)"
      maxLength={200}
      className="w-full rounded-[2px] border border-ok bg-white px-2 py-1 text-[13px]"
      value={v}
      onChange={(e) => setV(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === "Enter") onDone(v);
        if (e.key === "Escape") onDone(null);
      }}
      onBlur={() => onDone(v)}
    />
  );
}

function RowMenu({ c, onRename, onDelete }: { c: CaseSummary; onRename: () => void; onDelete: () => void }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  // #553: a menu opened from the keyboard must land focus on its first item,
  // but a mouse click should leave the pointer user's focus alone.
  const keyboardOpen = useRef(false);
  useEffect(() => {
    if (!open) {
      keyboardOpen.current = false;
      return;
    }
    if (keyboardOpen.current) menuRef.current?.querySelector<HTMLElement>('[role="menuitem"]:not([disabled])')?.focus();
    const close = (e: MouseEvent | KeyboardEvent) => {
      if (e instanceof KeyboardEvent ? e.key === "Escape" : !ref.current?.contains(e.target as Node)) {
        setOpen(false);
        // Esc returns keyboard focus to the ⋯ button.
        if (e instanceof KeyboardEvent) ref.current?.querySelector<HTMLButtonElement>("button[aria-expanded]")?.focus();
      }
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", close);
    };
  }, [open]);
  // #553: arrow/Home/End navigation for a role=menu, skipping disabled items.
  const onMenuKeyDown = (e: ReactKeyboardEvent) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp" && e.key !== "Home" && e.key !== "End") return;
    e.preventDefault();
    const items = [...(menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([disabled])') ?? [])];
    if (!items.length) return;
    const active = document.activeElement;
    const i = active ? items.indexOf(active as HTMLElement) : -1;
    const next = e.key === "Home" ? 0 : e.key === "End" ? items.length - 1 : i < 0 ? 0 : (i + (e.key === "ArrowDown" ? 1 : -1) + items.length) % items.length;
    items[next]?.focus();
  };
  return (
    <div
      className="relative inline-block"
      ref={ref}
      // #539: close the ⋯ menu when keyboard focus leaves the whole widget,
      // not only on Escape or an outside mouse click.
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOpen(false);
      }}
    >
      <button
        type="button"
        aria-label={`Actions for ${c.display_name}`}
        aria-expanded={open}
        className="px-2 py-1 text-base text-dim hover:text-ink"
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") keyboardOpen.current = true;
          if (e.key === "ArrowDown" && !open) {
            e.preventDefault();
            keyboardOpen.current = true;
            setOpen(true);
          }
        }}
        onClick={() => setOpen((o) => !o)}
      >
        ⋯
      </button>
      {open && (
        <div
          role="menu"
          aria-label={`Actions for ${c.display_name}`}
          ref={menuRef}
          onKeyDown={onMenuKeyDown}
          className="absolute right-0 z-20 mt-1 w-40 border border-edge bg-sheet py-1 text-left text-[13px] shadow-[4px_4px_0_var(--shadow)]"
        >
          <Link role="menuitem" href={caseHref(c)} className="block px-3 py-1.5 hover:bg-wash">
            Open
          </Link>
          <button role="menuitem" type="button" disabled={caseReadOnly(c)} title={caseReadOnly(c) ? VIEWER_REASON : undefined} className="block w-full px-3 py-1.5 text-left hover:bg-wash disabled:text-dim disabled:hover:bg-transparent" onClick={() => (setOpen(false), onRename())}>
            Rename
          </button>
          <button role="menuitem" type="button" disabled={caseReadOnly(c)} title={caseReadOnly(c) ? VIEWER_REASON : undefined} className="block w-full px-3 py-1.5 text-left text-fail hover:bg-fail-wash disabled:text-dim disabled:hover:bg-transparent" onClick={() => (setOpen(false), onDelete())}>
            Delete case…
          </button>
        </div>
      )}
    </div>
  );
}

// CBP changed a ruling the case relies on (D480): one line under the name.
function AlertBadge({ n }: { n?: { final: number; proposed: number } | null }) {
  const b = badgeText(n);
  if (!b) return null;
  return <span className={cx("block text-xs", b.attention ? "font-medium text-ask" : "text-dim")}>{b.text}</span>;
}

// The list row's watch reading; the same rule as the case page (watch-state).
function watchOf(c: { watch_status: string | null; watch_basis?: string | null; watch_route?: string | null }) {
  return watchView(c.watch_status, c.watch_basis, c.watch_route as "auto" | "needs_review" | "needs_info" | null);
}
