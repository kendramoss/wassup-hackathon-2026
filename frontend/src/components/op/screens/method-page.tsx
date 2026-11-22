import type { ReactNode } from "react";
import Link from "next/link";
import { Main } from "../chrome";
import { Reproduce } from "../failure-cases";
import { Stability } from "../method-stability";
import { copy } from "@/lib/copy";
import { ARMS, AXIS, BANDS, LEAK, PLAN_COST, PLAN_FAMILIES, PLAN_POST, ROUTER_V3, TIMELINE, blob, decisionHref, type Arm, type Band, type PlanBar } from "@/lib/method";

// "How it's built and tested" (review follow-up): the questions a sceptical
// technical judge asks, answered in order. Process facts are plain text;
// measured findings get a chart with their numbers (docs/NUMBERS.md and
// GET /v1/eval/summary only, no new figures).

// ui.tsx is a client module, so this server component keeps its own cx.
const cx = (...c: (string | false | null | undefined)[]) => c.filter(Boolean).join(" ");

const QUESTIONS = [
  { id: "model", q: "What is the model, and is anything trained?" },
  { id: "plans", q: "How are plans searched and checked?" },
  { id: "data", q: "What data does it see, and where does it come from?" },
  { id: "choices", q: "How were the prompts, thresholds and calibration chosen?" },
  { id: "clean", q: "How is the test set kept clean?" },
  { id: "negative", q: "What did we try that didn't work?" },
  { id: "stable", q: "How stable is it from run to run?" },
  { id: "check", q: "How can I check it myself?" },
];

function Ext({ href, children, className }: { href: string; children: ReactNode; className?: string }) {
  return (
    <a href={href} target="_blank" rel="noreferrer" className={cx("text-ok underline underline-offset-2", className)}>
      {children}
    </a>
  );
}

function D({ id }: { id: string }) {
  return (
    <Ext href={decisionHref(id)} className="font-mono text-[0.92em]">
      {id}
    </Ext>
  );
}

function Refs({ ids }: { ids: string[] }) {
  return (
    <>
      {ids.map((id, i) => (
        <span key={id}>
          {i > 0 ? ", " : ""}
          <D id={id} />
        </span>
      ))}
    </>
  );
}

function Section({ id, n, children }: { id: string; n: number; children: ReactNode }) {
  const q = QUESTIONS.find((x) => x.id === id)!;
  return (
    <section id={id} aria-labelledby={`${id}-h`} className="mt-12 scroll-mt-6 border-t border-line pt-6">
      <p className="eyebrow">Question {n} of {QUESTIONS.length}</p>
      <h2 id={`${id}-h`} className="mt-1 text-[22px] leading-tight tracking-[-0.04em]">
        {q.q}
      </h2>
      <div className="read mt-3 flex flex-col gap-3 text-[14.5px] text-body [&>p]:max-w-[72ch] [&>ul]:max-w-[72ch]">{children}</div>
    </section>
  );
}

// A chart panel: the measured findings on this page.
function Figure({ title, note, children, label }: { title: string; note: string; children: ReactNode; label: string }) {
  return (
    <figure aria-label={label} className="mt-5 max-w-[960px] border border-edge bg-sheet px-4 py-4 shadow-[4px_4px_0_var(--shadow)] sm:px-5">
      <figcaption className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <span className="eyebrow">{title}</span>
        <span className="text-[11px] text-dim sm:ml-auto">{note}</span>
      </figcaption>
      {children}
    </figure>
  );
}

const pct = (v: number) => `${(v * 100).toFixed(1)}%`;
const day = (iso: string) => Date.parse(`${iso}T00:00:00Z`);

function Windows() {
  const a = day(AXIS.from);
  const span = day(AXIS.to) - a;
  const at = (iso: string) => ((day(iso) - a) / span) * 100;
  const fill: Record<Band["tone"], string> = {
    pool: "bg-wash border border-edge",
    train: "border border-dashed border-dim bg-white",
    dev: "bg-ok/35",
    test: "bg-ok",
  };
  return (
    <Figure
      title="Where the rulings sit in time"
      note="by CBP ruling date · freeze dates in the notes"
      label="Timeline of the ruling corpus and the splits, January 2023 to December 2026"
    >
      <ul className="mt-4 flex flex-col gap-4">
        {BANDS.map((b) => (
          <li key={b.key} className="grid gap-x-4 gap-y-1 text-[12.5px] md:grid-cols-[210px_minmax(0,1fr)]">
            <div>
              <p className="font-medium text-ink">
                {b.label} <span className="tabular-nums font-normal text-dim">· {b.n}</span>
              </p>
              <p className="text-[11.5px] leading-snug text-dim">{b.note}</p>
            </div>
            <div className="relative h-4 self-center bg-paper">
              <i
                aria-hidden="true"
                className={cx("absolute inset-y-0 min-w-[3px]", fill[b.tone])}
                style={{ left: `${at(b.from)}%`, width: `${Math.max(at(b.to) - at(b.from), 0.6)}%` }}
              />
              <span className="sr-only">
                {b.from} to {b.to}
              </span>
            </div>
          </li>
        ))}
      </ul>
      <div className="mt-2 grid gap-x-4 text-[11px] text-dimension md:grid-cols-[210px_minmax(0,1fr)]">
        <span className="max-md:hidden" />
        <span className="relative h-4">
          {["2023", "2024", "2025", "2026"].map((y) => (
            <span key={y} className="absolute -translate-x-0 tabular-nums" style={{ left: `${at(`${y}-01-01`)}%` }}>
              {y}
            </span>
          ))}
        </span>
      </div>
    </Figure>
  );
}

function Leak() {
  const share = LEAK.heldOutSlots / LEAK.slots;
  return (
    <Figure
      title="One leak path, measured"
      note="first held-out split, official run · claimscheck recomputes these"
      label={`${LEAK.heldOutSlots} of ${LEAK.slots} retrieved precedents are earlier held-out rulings, in ${LEAK.casesTouched} of ${LEAK.cases} cases. ${LEAK.afterCase} are dated on or after their case.`}
    >
      <div className="mt-4 text-[12.5px]">
        <div className="relative h-5 border border-edge bg-wash" aria-hidden="true">
          <i className="absolute inset-y-0 left-0 bg-ok" style={{ width: `${share * 100}%` }} />
        </div>
        <ul className="mt-3 grid gap-x-6 gap-y-1.5 sm:grid-cols-2">
          <li className="flex items-center gap-2">
            <i aria-hidden="true" className="inline-block size-2.5 bg-ok" />
            <span>
              <b className="font-medium tabular-nums">{LEAK.heldOutSlots.toLocaleString("en-US")}</b> of {LEAK.slots.toLocaleString("en-US")} precedents shown to the model are earlier
              held-out rulings
            </span>
          </li>
          <li className="flex items-center gap-2">
            <i aria-hidden="true" className="inline-block size-2.5 border border-edge bg-wash" />
            <span>the rest are other earlier rulings</span>
          </li>
          <li className="tabular-nums">
            <b className="font-medium">{LEAK.casesTouched}</b> of {LEAK.cases} held-out cases see at least one
          </li>
          <li className="tabular-nums">
            <b className="font-medium">{LEAK.afterCase}</b> precedents dated on or after their case
          </li>
        </ul>
      </div>
    </Figure>
  );
}

// Development accuracy of each arm, dot and 95% interval, on one scale.
const LO = 0.83;
const HI = 0.95;
const x = (v: number) => `${((v - LO) / (HI - LO)) * 100}%`;

function ArmRow({ r }: { r: Arm }) {
  return (
    <li className="grid gap-x-4 gap-y-1 border-b border-dashed border-line py-2 text-[12.5px] md:grid-cols-[220px_minmax(0,1fr)_250px]">
      <span className={cx(r.ref ? "font-medium text-ink" : "text-body")}>{r.label}</span>
      <span className="relative h-5 self-center" aria-hidden="true">
        <i className="absolute inset-x-0 top-1/2 h-px bg-line" />
        {r.lo !== undefined && r.hi !== undefined && <i className="absolute top-1/2 h-px -translate-y-px bg-ink" style={{ left: x(r.lo), width: `calc(${x(r.hi)} - ${x(r.lo)})`, height: 2 }} />}
        {r.runs && r.runs.length > 1 && (
          <>
            <i
              className="absolute top-1/2 -translate-y-px bg-ink"
              style={{ left: x(Math.min(...r.runs)), width: `calc(${x(Math.max(...r.runs))} - ${x(Math.min(...r.runs))})`, height: 2 }}
            />
            {r.runs.map((v, i) => (
              <i key={i} className="absolute top-1/2 size-1.5 -translate-x-1/2 -translate-y-1/2 rounded-full bg-ink" style={{ left: x(v) }} />
            ))}
          </>
        )}
        <i className={cx("absolute top-1/2 size-2.5 -translate-x-1/2 -translate-y-1/2 rotate-45", r.ref ? "bg-ok" : "border border-ink bg-white")} style={{ left: x(r.v) }} />
      </span>
      <span className="text-[12px] text-dim md:text-right">
        <b className="font-medium tabular-nums text-ink">{pct(r.v)}</b>
        {r.lo !== undefined && r.hi !== undefined ? <span className="tabular-nums"> ({(r.lo * 100).toFixed(1)}–{(r.hi * 100).toFixed(1)})</span> : null}
        {r.runs ? <span className="block tabular-nums">runs {r.runs.map((v) => pct(v)).join(", ")}</span> : null}
        {r.vs ? <span className="block tabular-nums">{r.vs}</span> : null}
        <span className="block">
          {r.note} · <Refs ids={r.refs} />
        </span>
      </span>
    </li>
  );
}

function Negatives() {
  const all = ARMS.flatMap((g) => g.rows);
  return (
    <Figure
      title="Tried on the development set and not adopted"
      note="463 development rulings · 95% interval where recorded, or each run as a dot · paired test against Argon on Vertex AI unless marked"
      label={`Development accuracy of ${all.length} configurations: ${all.map((r) => `${r.label} ${pct(r.v)}`).join("; ")}.`}
    >
      {ARMS.map((g) => (
        <div key={g.group} className="mt-4">
          <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-dim">{g.group}</p>
          <ul>
            {g.rows.map((r) => (
              <ArmRow key={r.label} r={r} />
            ))}
          </ul>
        </div>
      ))}
      <div className="mt-2 grid gap-x-4 text-[11px] text-dimension md:grid-cols-[220px_minmax(0,1fr)_250px]">
        <span className="max-md:hidden" />
        <span className="relative h-4">
          {[0.84, 0.88, 0.92].map((t) => (
            <span key={t} className="absolute -translate-x-1/2 tabular-nums" style={{ left: x(t) }}>
              {Math.round(t * 100)}%
            </span>
          ))}
        </span>
      </div>
      <div className="mt-5 border-t border-line pt-4 text-[12.5px]">
        <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-dim">Router, general rulings kept on the general engine</p>
        <ul className="mt-2 flex flex-col gap-2" aria-label={`router-v2 ${ROUTER_V3.v2} of ${ROUTER_V3.n}, router-v3 ${ROUTER_V3.v3} of ${ROUTER_V3.n}`}>
          {(
            [
              ["router-v2", ROUTER_V3.v2],
              ["router-v3", ROUTER_V3.v3],
            ] as const
          ).map(([k, v]) => (
            <li key={k} className="grid items-center gap-x-4 gap-y-1 md:grid-cols-[220px_minmax(0,1fr)_250px]">
              <span>{k}</span>
              <span className="relative h-3 bg-wash" aria-hidden="true">
                <i className={cx("absolute inset-y-0 left-0", k === "router-v2" ? "bg-ok" : "bg-dim")} style={{ width: `${(v / ROUTER_V3.n) * 100}%` }} />
              </span>
              <span className="tabular-nums text-dim md:text-right">
                <b className="font-medium text-ink">{pct(v / ROUTER_V3.n)}</b> ({v} of {ROUTER_V3.n})
              </span>
            </li>
          ))}
        </ul>
        <p className="mt-2 text-[12px] text-dim">
          router-v3 routed specialist rulings better but sent more ordinary rulings away, past the {ROUTER_V3.limit} its rule allowed (<D id="D231" />). router-v4, defined before its run, passed
          and is in production (<D id="D233" />). One caveat: its &quot;trade programs&quot; cue was written after reading the post-cutoff misses, so the post-cutoff router figures are
          not clean held-out evidence for it. Without the 6 cue rulings the routed path scores 90/101 for router-v4 against 91/101 for router-v2.
        </p>
      </div>
    </Figure>
  );
}

// The plan check on later rulings (judge panel round 4): a measured finding,
// so bars with their numbers (docs/NUMBERS.md, D170 and D192).
function PlanBarRow({ r }: { r: PlanBar }) {
  const v = r.k / r.n;
  return (
    <li className="grid gap-x-4 gap-y-1 border-b border-dashed border-line py-2 text-[12.5px] md:grid-cols-[240px_minmax(0,1fr)_170px]">
      <span>{r.label}</span>
      <span className="relative h-5 self-center" aria-hidden="true">
        <i className="absolute inset-x-0 top-1/2 h-2 -translate-y-1/2 border border-edge bg-wash" />
        <i className={cx("absolute left-0 top-1/2 h-2 -translate-y-1/2", r.lowerIsBetter ? "bg-fail" : "bg-ok")} style={{ width: `${v * 100}%` }} />
        <i className="absolute top-[calc(50%+6px)] h-1.5 border border-b-0 border-dimension" style={{ left: `${r.lo}%`, width: `${r.hi - r.lo}%` }} />
      </span>
      <span className="tabular-nums text-dim md:text-right">
        <b className="font-medium text-ink">
          {r.k} of {r.n}
        </b>{" "}
        ({r.lo}–{r.hi})
      </span>
    </li>
  );
}

function PlanChecks() {
  return (
    <Figure
      title="The precedent check on later rulings"
      note="18 frozen CBP flip pairs, at least one ruling of each from Sep–Nov 2026 · run 3 times, same figures each time with the starting origin given (D354) · gateway · 95% interval"
      label={PLAN_POST.map((g) => `${g.title}: ${g.rows.map((r) => `${r.label} ${r.k} of ${r.n}`).join("; ")}`).join(". ")}
    >
      {PLAN_POST.map((g) => (
        <div key={g.title} className="mt-4">
          <p className="text-[11px] font-medium uppercase tracking-[0.08em] text-dim">
            {g.title} · {g.note}
          </p>
          <ul>
            {g.rows.map((r) => (
              <PlanBarRow key={r.label} r={r} />
            ))}
          </ul>
        </div>
      ))}
      <p className="mt-3 text-[12px] text-dim">
        A pair is two CBP rulings on one product before and after work moved; in 12 the origin changed, in 6 it did not. Frozen before scoring (<D id="D170" />,{" "}
        <D id="D192" />).
      </p>
    </Figure>
  );
}

export function MethodPage() {
  return (
    <Main>
      <p className="eyebrow">Method</p>
      <h1 className="mt-1 text-[34px] leading-tight tracking-[-0.05em]">How it&apos;s built and tested</h1>
      <div className="read mt-3 flex max-w-[72ch] flex-col gap-3 text-[15px] text-body">
        <p className="border-l-2 border-ok pl-3">
          <b className="font-medium text-ink">{copy.novel.lead}</b> {copy.novel.text} <a href="#plans" className="text-ok underline underline-offset-2">How →</a>
        </p>
        <p>
          Unde is not a trained model. It sends Gemini the facts of a product with the most similar earlier CBP rulings, and asks for the origin CBP would likely assign. What we
          chose by hand are the prompts, the retrieval settings, the confidence bars and a calibration map, all on development rulings.
        </p>
        <p>
          This page answers, in order, the questions we would ask of a project like this. Each answer links to the dated entry in our decision record and to the files on GitHub,
          so you can check it rather than take our word for it.
        </p>
      </div>

      <nav aria-label="On this page" className="mt-6 max-w-[72ch]">
        <ol className="flex flex-col border-t border-line text-[13.5px]">
          {QUESTIONS.map((q, i) => (
            <li key={q.id} className="border-b border-line">
              <a href={`#${q.id}`} className="flex min-h-10 items-center gap-3 py-1.5 text-ink hover:text-ok">
                <span className="w-5 shrink-0 font-mono text-[11px] text-dim">{i + 1}</span>
                {q.q}
              </a>
            </li>
          ))}
        </ol>
      </nav>

      <section aria-labelledby="protocol-h" className="mt-10">
        <h2 id="protocol-h" className="eyebrow">
          The protocol, in order
        </h2>
        <ol className="mt-3 max-w-[90ch] border-l border-dashed border-guide pl-5">
          {TIMELINE.map((s, i) => (
            <li key={i} className="relative pb-4 last:pb-0">
              {s.period && s.period !== TIMELINE[i - 1]?.period ? (
                <p className="mb-2 text-[11px] font-medium uppercase tracking-[0.08em] text-dim">{s.period}</p>
              ) : null}
              <i aria-hidden="true" className="absolute -left-[25px] top-1.5 size-2 rotate-45 bg-ok" />
              <p className="grid gap-x-4 gap-y-0.5 text-[13.5px] sm:grid-cols-[130px_minmax(0,1fr)]">
                <span className="font-mono text-[12px] text-dimension">{s.date}</span>
                <span className="read text-body">
                  {s.what}{" "}
                  <span className="whitespace-nowrap text-[12px] text-dim">
                    {s.refs.length > 0 && <Refs ids={s.refs} />}
                    {s.files?.map((f, j) => (
                      <span key={f.path}>
                        {s.refs.length > 0 || j > 0 ? ", " : ""}
                        <Ext href={blob(f.path)}>{f.label}</Ext>
                      </span>
                    ))}
                  </span>
                </span>
              </p>
            </li>
          ))}
        </ol>
      </section>

      <Section id="model" n={1}>
        <p>
          <b className="font-medium text-ink">No model is trained or fine-tuned for the product.</b> The origin engine is Gemini 4 Argon on Vertex AI, called with a fixed prompt
          (engine-v1), temperature 0 and a fixed seed (<D id="D76" />). A router on Gemini 3.8 Flash decides which set of origin rules applies. The weights are Google&apos;s and
          unchanged.
        </p>
        <p>
          What we built around them: retrieval of the five most similar earlier rulings (BM25 over public CBP rulings), the prompts, confidence bars, a calibration map that turns
          the model&apos;s stated confidence into a probability, and the code that routes, checks and logs each answer.
        </p>
        <p>
          <b className="font-medium text-ink">Rule check.</b> Beside the model sits a deterministic check of the 19 CFR 102.20 tariff-shift rules for 17 HTS chapters
          (<D id="D360" />, <D id="D460" />). It runs only when the parts list gives HTS codes, says &quot;undetermined&quot; rather than guess, and never changes the answer or what is
          auto-accepted (<D id="D363" />). From the facts the model reads it decides almost none of the committed Part 102 rulings, because they rarely give codes: 0 of 72
          development and 1 of 45 held out. From the full ruling text it decides 8 of 72 development and 4 of 32 held-out rulings, each as CBP held (<D id="D362" />, <D id="D461" />). Since parts rarely come with codes, the parts table now suggests up to three per part,
          each marked as from an earlier CBP ruling on a similar part or as the model&apos;s own reading, and fills one in only when you accept it (<D id="D380" />). Where
          CBP states a part&apos;s code, the top suggestion matched it for 20 of 27 development parts. With CBP&apos;s code for the good and the suggested part codes,
          the check decides 16 of 69 development rulings (all 16 agree with CBP) and, held out, 14 of 29 (13 agree); 11 of 24 (10 agree) without the inputs that
          carried a code (<D id="D382" />, <D id="D413" />, <D id="D461" />). With only the codes CBP states it decides 8 and 4. The first seven chapters gave 9 and 7. Run again over every combination of each
          part&apos;s top three suggestions, 15 of the 16 development decisions and 13 of the 14 held-out ones give the same answer under every code; the held-out one
          that disagrees with CBP is the one that turns on a part&apos;s code (<D id="D430" />). The codes are unverified: check them with a broker.{" "}
          <Link href="/examples/rule-check" className="text-ok underline underline-offset-2">
            See a recorded example
          </Link>
          .
        </p>
        <p>
          We did try fine-tuning once. On 16 and 17 Oct we tuned Gemini 3.5 Flash on CBP&apos;s own reasoning from rulings older than the development split. It was worse on the
          development set, so we dropped it, deleted the endpoints, and never scored it on held-out rulings (<D id="D73" />; chart in question 6). When Argon is unavailable a base
          Gemini 3.8 Flash answers instead, and its answers always go to review.
        </p>
        <p className="text-[13.5px] text-dim">
          More: <Ext href={blob("docs/MODEL-CARD.md")}>model card</Ext>, <Ext href={blob("docs/PROMPTS.md")}>prompts</Ext>,{" "}
          <Ext href={blob("docs/ARCHITECTURE.md")}>architecture</Ext>.
        </p>
      </Section>

      <Section id="plans" n={2}>
        <p>
          <b className="font-medium text-ink">The plan search is the part a classifier does not do.</b> It asks which production change would move the origin, and it shows a
          move only when an earlier CBP ruling backs it. Each step:
        </p>
        <ol className="flex list-decimal flex-col gap-2 pl-5">
          <li>
            <b className="font-medium text-ink">Groups of work, from a template.</b> Each of 14 product families has a template, written from CBP rulings, that files the
            product&apos;s parts and steps into groups of work, such as the motor-fan assembly or the PCBA (<D id="D138" />). Final assembly does not move.
          </li>
          <li>
            <b className="font-medium text-ink">Candidate moves.</b> Moves of one, two or three groups to a plant the product already lists. The heat gun has seven. CBP&apos;s real change for it moved three groups at once, so
            three-group moves were added (<D id="D84" />).
          </li>
          <li>
            <b className="font-medium text-ink">The engine predicts the new origin.</b> The same origin engine reads the changed facts with the most similar earlier rulings. A
            move whose predicted origin is not the target country goes no further.
          </li>
          <li>
            <b className="font-medium text-ink">A verifier checks it against earlier rulings.</b> An ADK agent on Gemini searches CBP rulings dated before the case for one that
            supports the move and for one against it, and states a confidence tied to what it read (<D id="D74" />). A move is shown only with a supporting ruling at 0.92 or
            above. A supported move below the bar is listed as below the bar; the closest one can still go to a broker, labelled as such. Shown moves rank in two tiers
            (<D id="D341" />): first those with a clear margin, a check of 0.93 or more and no counter-ruling, then those at the bar, which may not hold on a re-check. Within
            a tier they rank by the net per year on the duty and cost inputs. The move that changes the fewest parts is labelled the smallest supported move. A ruling CBP
            later revoked or modified is never cited for a case dated after the change, and the site marks it &quot;superseded by&quot; (<D id="D320" />, <D id="D370" />). The
            map holds 144 such changes to 143 rulings: 35 read from later rulings in the corpus and 109 from final notices in 28 fetched Customs Bulletins, each
            counted from its effective date (<D id="D400" />, <D id="D401" />). 78 changes CBP has only proposed are shown beside the ruling and never block it. Before
            the guard 23 of 13,166 committed rows had shown a changed ruling and 8 had cited one; the bulletin changes add none. When CBP changes a ruling a saved
            case relies on after the case was decided, the case and the cases list say so and offer a re-check; the saved answer is never changed (<D id="D480" />).
            In the committed results 2 rows rely on a ruling changed on or after their case date, and none on a change that is only proposed. A revoking ruling that cites the ruling it revokes is not alerted on itself (<D id="D490" />).
          </li>
          <li>
            <b className="font-medium text-ink">The tariff-shift rule beside it.</b> When the case is routed to the Part 102 rules, or every part has an HTS code, each move
            also gets the 19 CFR 102.20 rule check of the product after the move, made where its final assembly would be (<D id="D441" />). It says whether the move shifts
            under 102.20, does not, or is undetermined, and why. It never changes which moves are shown or their order. A move an earlier ruling supports but the rule does
            not is flagged for a broker. On the 23 per-family searches, with part codes from the development suggestion run, it decides 25 of 57 moves and agrees with the
            precedent check on 21 of 23; the 2 it flags are below the bar. Those cases are decided under substantial transformation, so this is a comparison, not
            accuracy, and there is no Part 102 plan search with codes to score yet (<D id="D442" />).
          </li>
          <li>
            <b className="font-medium text-ink">Shown, then drafted for a broker.</b> The shown move and its ruling go into the draft ruling request a licensed customs broker
            checks and files. Only CBP&apos;s ruling binds.
          </li>
        </ol>
        <p>
          <b className="font-medium text-ink">Cost and time.</b> The recorded heat-gun search ran {PLAN_COST.engineCalls} engine calls and {PLAN_COST.verifierRuns} verifier runs
          in {PLAN_COST.heatGunSeconds} s for {PLAN_COST.heatGunUsd} (promo price). That is an earlier proxy run of six moves, before three-group moves were
          searched, and it showed a different plan. It is not the production run behind the 0.96 plan on the plan step. The per-family checks averaged {PLAN_COST.perSearchUsd} a search. Before a search the
          site shows its own estimate; a repeat search uses the stored checks and costs nothing.
        </p>
        <PlanChecks />
        <p>
          <b className="font-medium text-ink">Also measured.</b> The heat gun&apos;s shown plan was backtested against what the manufacturer really did: CBP&apos;s later ruling
          N357771 records the change. The comparison is between the parts whose origin that change switched from China to Thailand and the parts the plan moves. The change
          switched 6 parts, and the plan moves all 6 (one production run; the plan step shows it). The plan also moves the switch, wires and fasteners, which the real change
          kept in China, so a smaller move may have been enough. The outer shells, which N357771 also lists as Thai, were already Thai in 2022 and are not counted. This is
          not a blind test: three-group moves were added to the search after the team saw this change (<D id="D84" />). The date cutoff still holds. Also after seeing it,
          the power cord, knobs and covers were split from the switch and wiring, so the search can propose CBP&apos;s smaller move (<D id="D330" />). In two later gateway
          runs that move was shown, with a check of 0.94, but cheaper moves that move fewer parts ranked above it (<D id="D332" />). Re-ranked by tier, it ranks first in
          run 2 and 6th of 7 in run 1, where its check named a counter-ruling; the tier rule was set after seeing these runs (<D id="D341" />). The plan step still shows the stored
          production plan, made before the split, until it is re-pinned. In the per-family checks, 23
          products across the families, {PLAN_FAMILIES.k} of {PLAN_FAMILIES.n} showed a supported plan ({PLAN_FAMILIES.lo}–{PLAN_FAMILIES.hi}); read by hand, the cited ruling
          supports the move in {PLAN_FAMILIES.supports} of those {PLAN_FAMILIES.k}, partly in {PLAN_FAMILIES.partial} and not in {PLAN_FAMILIES.not} (<D id="D171" />). The
          heat-gun pair itself is left out of every flip-pair run, so the check was never fitted to the demo.
        </p>
        <p>What is not measured, or not clean:</p>
        <ul className="flex list-disc flex-col gap-2 pl-5">
          <li>
            Part of the verifier&apos;s prompt was written after seeing failures on the first 2026 pairs (<D id="D74" />), so its earlier results there (<D id="D86" />) are not a
            clean test. The 18 later pairs above are.
          </li>
          <li>Whether a shown plan is the cheapest or the best move. The money is illustrative unless you enter your own costs, and the search only moves work between plants you list.</li>
          <li>
            Whether a lower bar would do as well. A development-only rule, written before the re-computation, picked 0.92 again: the only wrong supported verdicts sit just
            below it. There are too few wrong verdicts to place a lower bar, so that waits for a scored run on new pairs (<D id="D300" />, <D id="D301" />). The
            precedent-check tables on the{" "}
            <a href="/accuracy#plan-checks" className="text-ok underline underline-offset-2">
              accuracy page
            </a>{" "}
            have the rows.
          </li>
          <li>The pairs and the cited rulings were read by one reader, not a customs expert. The flip-pair runs went through a gateway, not Vertex AI, with a local copy of the rulings for search.</li>
        </ul>
      </Section>

      <Section id="data" n={3}>
        <p>
          <b className="font-medium text-ink">Public CBP rulings, and the facts you type.</b> The rulings come from CBP&apos;s public CROSS database (rulings.cbp.gov). Nothing
          else is in the corpus.
        </p>
        <p>
          The engine reads only the facts section of each earlier ruling, cut where the legal analysis starts, with sentences that state the origin conclusion removed. The label for
          testing is CBP&apos;s stated holding: extracted by a rule from the holding sentence, set by hand where the rule failed, and spot-checked. No customs expert reviewed the
          labels.
        </p>
        <p>A case can only see rulings dated before it, never its own ruling or a later one. That holds in testing and on this site.</p>
        <Windows />
        <p className="mt-3 text-[13.5px] text-dim">
          Collection, labelling, known biases and gaps are in the <Ext href={blob("docs/DATASHEET.md")}>datasheet</Ext>.
        </p>
      </Section>

      <Section id="choices" n={4}>
        <p>
          <b className="font-medium text-ink">On development rulings, by rules written down first.</b> The engine development split is CBP rulings from July to December 2025. The
          prompt, the number of precedents, the calibration map and the auto-accept bar were compared there. The router has its own 729-ruling development split (25 Jan 2024 to 31
          Dec 2025); one router cue came from reading later misses, so its post-cutoff figures are qualified below.
        </p>
        <p>
          After an external expert review on 21 Nov 2026, we hardened the protocol, and every experiment from that day follows one pattern. A decision entry, written before the run, names the variants, the pass rule and what happens to the held-out splits. A
          second entry reports the result, including what went worse. A variant that fails on the development set never touches a held-out split. Examples: the model sweep (
          <D id="D152" />, <D id="D153" />), the calibration map and bar (<D id="D160" />, <D id="D161" />), the adjudicator (<D id="D163" />, <D id="D164" />), the router (
          <D id="D230" />, <D id="D231" />, <D id="D233" />) and the repeat runs (<D id="D240" />, <D id="D241" />).
        </p>
        <p>Where we fell short of that, and what we did about it:</p>
        <ul className="flex list-disc flex-col gap-2 pl-5">
          <li>
            The first auto-accept bar (0.92 on stated confidence, 15 Oct) was picked with a rule that looked at development and held-out results together (<D id="D32" />). The bar
            in use now, 0.90 on calibrated confidence, was derived again from development rulings alone by a rule written first. It accepts exactly the same cases (<D id="D160" />,{" "}
            <D id="D161" />).
          </li>
          <li>
            The textile, pharmaceutical and Part 102 engines had their bars set from their first held-out windows (<D id="D51" />, <D id="D61" />, <D id="D62" />). Their clean check
            is the second window, scored once with nothing changed (<D id="D210" />, <D id="D211" />).
          </li>
          <li>
            Part of the plan check&apos;s prompt was written after seeing failures on the 2026 flip pairs (<D id="D74" />), so those pairs are not a clean test for it. The
            post-cutoff pairs on the accuracy page are.
          </li>
          <li>The model-sweep selection rule was written while two of its runs were still going, before any held-out file existed for them (<D id="D152" />).</li>
          <li>
            router-v4&apos;s &quot;trade programs&quot; cue came from reading the misses on the post-cutoff split, not from development (1 of 70 development Part 102 rulings uses
            the phrase). Its post-cutoff gains are not clean held-out evidence: without the 6 cue rulings, router-v4 scores 90/101 on the routed path and router-v2 91/101. The
            adoption decision used the first held-out window, where the phrase is nearly absent (<D id="D233" />).
          </li>
        </ul>
      </Section>

      <Section id="clean" n={5}>
        <p>
          <b className="font-medium text-ink">Split by date, frozen before scoring, scored once.</b>
        </p>
        <ul className="flex list-disc flex-col gap-2 pl-5">
          <li>
            The first held-out split (January to September 2026) was frozen in this repository on 14 Oct 2026. The research prototype before it had already scored the same rulings
            with the same engine design; the frozen engine was then scored on them once on Vertex AI (<D id="D39" />). The second (28 Sep to 13 Nov 2026) was built from rulings
            published after everything else and frozen on 21 Nov before it was scored (<D id="D150" />). Both are pinned by checksum in{" "}
            <Ext href={blob("eval/SHA256SUMS")}>eval/SHA256SUMS</Ext>.
          </li>
          <li>
            The evaluation tool refuses a held-out split without an explicit flag, and says why:
            <code className="mt-1 block border border-line bg-white px-3 py-2 font-mono text-[12px] leading-relaxed">
              the final splits are held out: pass -confirm-final, and do not change prompts, thresholds or retrieval because of its result
            </code>
          </li>
          <li>
            Every run appends its configuration, git revision and a held-out flag to <Ext href={blob("eval/results/RUNLOG.jsonl")}>RUNLOG.jsonl</Ext>, including runs that came
            out worse.
          </li>
          <li>
            Each candidate is scored once per held-out split. The frozen production engine has more than one run on the first split: a research run, a proxy run, the official
            Vertex AI run and two repeats (question 7). None led to a change.
          </li>
        </ul>
        <p>What is not sealed, stated plainly:</p>
        <ul className="flex list-disc flex-col gap-2 pl-5">
          <li>
            <b className="font-medium text-ink">Earlier held-out rulings can be precedents for later ones.</b> The filter is by date, not by split, as it is on this site. A May 2026
            case can read a February 2026 holding. Measured below.
          </li>
          <li>
            <b className="font-medium text-ink">Retrieval statistics use the whole corpus.</b> BM25 counts how common each word is across every ruling, later ones included. That
            leaks word counts, not text or holdings (<D id="D10" />).
          </li>
          <li>
            <b className="font-medium text-ink">Gemini may have seen some rulings in pre-training.</b> CBP rulings are public. We cannot rule this out. The second held-out window,
            published from late September 2026, is our best answer to it.
          </li>
        </ul>
        <Leak />
      </Section>

      <Section id="negative" n={6}>
        <p>
          <b className="font-medium text-ink">Most things.</b> Newer and larger Gemini models, a fine-tuned Flash, a majority vote, a written rule for the biggest error group, a
          second-stage adjudicator and one router prompt (router-v3) were each tried on the development set against the engine in use. None beat it by enough to pass its rule, so none was
          scored on held-out rulings. Every arm is below, including the ones that looked better.
        </p>
        <p>
          Earlier changes to retrieval and prompts (embeddings, re-ranking, voting, CBP&apos;s reasoning shown with each precedent) are in{" "}
          <Ext href={blob("docs/EVALUATION.md#34-retrieval-and-prompt-complexity-against-gain")}>EVALUATION.md §3.4</Ext> (<D id="D22" />). None moved accuracy by more than half a point.
        </p>
        <Negatives />
      </Section>

      <Section id="stable" n={7}>
        <p>
          <b className="font-medium text-ink">Run it again and the answer rarely changes.</b> The same frozen engine was run again with every call fresh: three times on the
          development set, and twice more on each held-out split. The published held-out figures stay the official single runs; the repeats sit beside them.
        </p>
        <Stability />
      </Section>

      <Section id="check" n={8}>
        <p>
          <b className="font-medium text-ink">Rerun the checks. They use committed files only, with no model calls and no credentials.</b> Each step below fails if a
          headline number has drifted from its source rows. Two rows of the chart in question 6, the Gemini 3.5 Flash runs from October, are not recomputed by these steps; their
          rows are committed in <span className="font-mono text-[12.5px]">eval/results/</span> and described in <D id="D44" /> and <D id="D73" />.
        </p>
        <div className="max-w-[960px]">
          <Reproduce />
        </div>
        <ul className="mt-2 grid gap-x-6 gap-y-1.5 text-[13.5px] sm:grid-cols-2">
          {[
            ["Decision record, dated", "docs/DECISIONS.md"],
            ["How the test sets are built", "docs/EVALUATION.md"],
            ["Every number and its source", "docs/NUMBERS.md"],
            ["Every scoring run", "eval/results/RUNLOG.jsonl"],
            ["Results, run by run", "eval/RESULTS.md"],
            ["Claims checked against the rows", "docs/claims-reconciliation.md"],
          ].map(([l, p]) => (
            <li key={p}>
              <Ext href={blob(p)}>{l}</Ext> <span className="font-mono text-[11.5px] text-dim">{p}</span>
            </li>
          ))}
        </ul>
        <p className="text-[13.5px] text-dim">
          The results themselves, with intervals and every held-out miss listed by ruling, are on the{" "}
          <a href="/accuracy" className="text-ok underline underline-offset-2">
            accuracy page
          </a>
          .
        </p>
      </Section>
    </Main>
  );
}
