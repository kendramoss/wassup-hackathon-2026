"use client";

// The business view (/business): seven chapters, one per scroll, beside a
// fixed panel that keeps what Unde is in view (the left fifth of the page).
// Numbers come from docs/NUMBERS.md: duty ≈41.7% vs ≈16.7% (DUTY-SOURCES,
// product-specific), 340 of 377 held-out rulings right, 96.0% when sure,
// p50 4.6 s. The money row is the illustrative fixture ($500k / $180k).
import Link from "next/link";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { BusinessBar } from "./business-bar";
import { useDemoLinks } from "../demo-links";
import "./business.css";

// Plain, literal words (DESIGN.md principle 5): one fast read, headlines of
// 8 words or fewer, customs never the subject, Trust and Brokers kept.
const chapters = ["The problem", "The main parts", "The change", "The savings", "Why trust it", "For brokers", "Try it"];

export function BusinessStory() {
  const story = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState(0);
  // Both "Open the heat gun example" links go straight to the pinned case's
  // plan tour, as on the landing (#892).
  const { tour } = useDemoLinks();

  useEffect(() => {
    const root = story.current;
    if (!root) return;
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) if (e.isIntersecting) setActive(Number((e.target as HTMLElement).dataset.i));
      },
      { root, threshold: 0.6 },
    );
    root.querySelectorAll("[data-i]").forEach((el) => io.observe(el));
    return () => io.disconnect();
  }, []);

  return (
    <div className="biz">
      <BusinessBar />

      <div className="biz-shell">
        <aside aria-label="About Unde" className="biz-side">
          <span className="biz-side-label">How it works</span>
          <nav aria-label="Chapters" className="biz-chapters">
            {chapters.map((t, i) => (
              <a key={t} href={`#ch${i + 1}`} aria-current={active === i ? "step" : undefined}>
                <span className="n mono">0{i + 1}</span>
                <span>{t}</span>
              </a>
            ))}
          </nav>
          <h1>Find ways to pay less import tax</h1>
          <p className="for">Where your parts are made decides the tax. Unde shows which parts to move. Full answers for the U.S. Rule checks for 6 more markets</p>
          <dl>
            <div>
              <dt>Right on past official decisions</dt>
              <dd className="mono">340 of 377</dd>
            </div>
            <div>
              <dt>Time for one check</dt>
              <dd className="mono">about 5 seconds</dd>
            </div>
          </dl>
          <p className="fine">An estimate. A customs broker confirms it.</p>
        </aside>

        <main ref={story} className="biz-story" id="main">
          <Chapter i={0}>
            <Text n={1} title="This heat gun pays China’s import tax" sub="It is put together in Thailand. But its main parts come from China, so it counts as made in China">
              <Actions next="Next: the main parts" secondary={<Link href={tour} className="biz-link">Open the heat gun example</Link>} />
            </Text>
            <figure className="biz-visual" style={{ margin: 0 }}>
              <HeatGun after={false} />
              <figcaption className="biz-figcap">
                <span>Put together in Thailand</span>
                <span>
                  <span className="big mono" style={{ color: "var(--fail)" }}>≈41.7%</span>import tax
                </span>
              </figcaption>
            </figure>
          </Chapter>

          <Chapter i={1} stack>
            <div style={{ display: "flex", alignItems: "flex-end", gap: 40, flexWrap: "wrap" }}>
              <Text n={2} title="The main parts decide the country" sub="Not where it is put together. 4 of its 6 parts come from China" wide />
              <Actions next="Next: the change" />
            </div>
            <section aria-label="Heat gun parts" className="biz-card biz-tray">
              <div className="biz-tray-head">
                <span>The heat gun’s parts</span>
                <span>Counts as: <span className="biz-tag cn">Made in China</span></span>
              </div>
              <div className="biz-parts">
                {parts.map((p) => (
                  <figure key={p.name} className={p.home ? "quiet" : undefined}>
                    <svg width="120" height="120" viewBox="0 0 120 120" aria-hidden="true" fill="none" stroke={p.home ? "var(--edge)" : "var(--ink)"} strokeWidth="1.5">{p.icon}</svg>
                    <figcaption>
                      {p.name}
                      <span className={`biz-tag ${p.home ? "th" : "cn"}`}>{p.home ? "TH" : "CN"}</span>
                    </figcaption>
                  </figure>
                ))}
              </div>
              <div className="biz-tray-foot">
                <span>Import tax today</span>
                <span className="mono">≈41.7%</span>
              </div>
            </section>
          </Chapter>

          <Chapter i={2}>
            <Text n={3} title="Make 4 parts in Thailand" sub="Then it likely counts as made in Thailand. A past official decision on a similar product says so">
              <Actions next="Next: the savings" />
            </Text>
            <figure className="biz-visual" style={{ margin: 0 }}>
              <HeatGun after />
              <figcaption className="biz-figcap">
                <span>After the change</span>
                <span>
                  <span className="big mono" style={{ color: "var(--ok)" }}>≈16.7%</span>import tax
                </span>
              </figcaption>
            </figure>
          </Chapter>

          <Chapter i={3}>
            <Text n={4} title="Import tax drops from ≈41.7% to ≈16.7%" sub="25 points less tax on each heat gun sold in the U.S.">
              <dl className="biz-figs">
                <dt>Tax saved per year</dt>
                <dd className="mono">$500k</dd>
                <dt>Extra cost per year</dt>
                <dd className="mono">$180k</dd>
                <dt>You keep per year</dt>
                <dd className="mono key">$320k</dd>
              </dl>
              <Actions next="Next: why trust it" />
              <p className="biz-fine">Example amounts. Enter your own. A customs broker confirms the tax rate</p>
            </Text>
            <figure className="biz-visual" style={{ margin: 0 }}>
              <DutyRuler />
            </figure>
          </Chapter>

          <Chapter i={4}>
            <Text n={5} title="The real maker did this, and it worked" sub="2022: counted as made in China. Then the maker moved most of these parts. 2026: counted as made in Thailand.">
              <dl className="biz-figs">
                <dt>Right on past official decisions</dt>
                <dd className="mono">340 of 377</dd>
                <dt>Right when Unde is sure</dt>
                <dd className="mono">96%</dd>
              </dl>
              <Actions next="Next: for brokers" secondary={<Link href="/accuracy" className="biz-link">See the full test</Link>} />
            </Text>
            <div className="biz-visual biz-letters">
              <Letter cls="a" refText="Official decision N328954 · 2022" fact="Motor, circuit board, switch and cord from China" stamp="MADE IN: CHINA" color="var(--fail)" />
              <Letter cls="b" refText="Official decision N357771 · 2026" fact="Motor, fan, circuit board and cord now made in Thailand" stamp="MADE IN: THAILAND" color="var(--ok)" />
            </div>
          </Chapter>

          <Chapter i={5}>
            <Text n={6} title="Each team gets its answer" sub="Your customs broker gets a ready request to send">
              <Actions next="Next: try it" />
              <p className="biz-fine">Made for these roles. No customers yet.</p>
            </Text>
            <section aria-label="The heat gun check, by role" className="biz-visual biz-card biz-roles">
              <header>
                <span>Heat gun check</span>
                <span className="mono" style={{ fontSize: 12 }}>1 answer</span>
              </header>
              <Role n="01" who="Supply chain manager" gets="Tax saved vs extra cost" show={<span className="mono" style={{ fontSize: 15 }}><s style={{ color: "var(--dim)" }}>≈41.7%</s>  ≈16.7%</span>} />
              <Role n="02" who="Trade compliance manager" gets="The likely country, and changes past decisions support" show={<span className="biz-stamp" style={{ color: "var(--ok)", fontSize: 12, padding: "4px 10px", borderWidth: 1.5 }}>MADE IN THAILAND</span>} />
              <Role n="03" who="U.S. importer" gets="Avoids paying for a change that saves nothing" show={<span style={{ fontSize: 13, color: "var(--fail)" }}>Warning: this change saves nothing</span>} />
              <Role n="04" who="Factory in Thailand" gets="Which parts to start making" show={<span className="mono" style={{ fontSize: 15 }}>4 parts to start making</span>} />
              <footer>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--dim)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                  <path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z" />
                  <path d="M14 2v4a2 2 0 0 0 2 2h4" />
                </svg>
                <span>Your broker gets a ready request to send</span>
              </footer>
            </section>
          </Chapter>

          <Chapter i={6} stack>
            <div style={{ display: "flex", alignItems: "flex-end", gap: 40, flexWrap: "wrap" }}>
              <Text n={7} title="Check your product" sub="Pick where you sell. List your parts. See your country and how to change it" wide />
              <div className="biz-actions">
                <Link href={tour} className="biz-link">Open the heat gun example</Link>
                <Link href="/business/check" className="biz-btn accent">Check my product</Link>
              </div>
            </div>
            <MovePreview />
          </Chapter>
        </main>
      </div>
    </div>
  );
}

// The app preview in chapter 7. The switches work: a part switched on is
// made in Thailand. Only the full plan (all four China parts) is backed by an
// earlier ruling (N345938), so only that shows the Thailand label; any other
// mix says it isn't backed yet rather than guessing.
function MovePreview() {
  const movable = parts.filter((p) => !p.home).map((p) => p.name);
  const [moved, setMoved] = useState<Set<string>>(() => new Set(movable));
  const all = movable.every((n) => moved.has(n));
  const none = moved.size === 0;
  const flip = (name: string) =>
    setMoved((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  return (
    <section aria-label="Unde case preview" className="biz-card biz-app">
      <div className="biz-app-head">
        <span style={{ fontWeight: 500 }}>Heat gun, 12.5 A</span>
        <span style={{ color: "var(--dim)" }}>Put together in Thailand</span>
        <span className="mono" style={{ marginLeft: "auto", fontSize: 12, color: "var(--dim)" }}>Example · checked in 4.6 seconds</span>
      </div>
      <div className="biz-app-body">
        <div className="biz-rows">
          <div className="biz-row head"><span>Part</span><span>Made in</span><span style={{ textAlign: "right" }}>Move to Thailand</span></div>
          {parts.map((p) => {
            const th = p.home || moved.has(p.name);
            return (
              <div key={p.name} className="biz-row">
                <span>{p.name}</span>
                <span><span className={`biz-tag ${th ? "th" : "cn"}`}>{th ? "TH" : "CN"}</span></span>
                {p.home ? (
                  <span style={{ justifySelf: "end", fontSize: 12, color: "var(--dim)" }}>Already in Thailand</span>
                ) : (
                  <button type="button" role="switch" aria-checked={moved.has(p.name)} aria-label={`Move ${p.name.toLowerCase()} to Thailand`} onClick={() => flip(p.name)} className={`biz-switch${moved.has(p.name) ? " on" : ""}`} />
                )}
              </div>
            );
          })}
        </div>
        <div className="biz-result" aria-live="polite">
          {all ? (
            <>
              <span style={{ fontSize: 13, color: "var(--dim)" }}>Likely country after the change</span>
              <span className="biz-stamp" style={{ color: "var(--ok)", fontSize: 20, alignSelf: "flex-start" }}>MADE IN THAILAND</span>
              <span style={{ display: "flex", alignItems: "baseline", gap: 12 }}>
                <s className="mono" style={{ fontSize: 15, color: "var(--dim)" }}>≈41.7%</s>
                <span className="mono" style={{ fontSize: 32 }}>≈16.7%</span>
                <span style={{ fontSize: 13, color: "var(--dim)" }}>import tax</span>
              </span>
              <span style={{ fontSize: 14, lineHeight: "20px" }}>Backed by a past decision on a similar product</span>
            </>
          ) : none ? (
            <>
              <span style={{ fontSize: 13, color: "var(--dim)" }}>Counts as today</span>
              <span className="biz-stamp" style={{ color: "var(--fail)", fontSize: 20, alignSelf: "flex-start" }}>MADE IN CHINA</span>
              <span style={{ display: "flex", alignItems: "baseline", gap: 12 }}>
                <span className="mono" style={{ fontSize: 32 }}>≈41.7%</span>
                <span style={{ fontSize: 13, color: "var(--dim)" }}>import tax</span>
              </span>
              <span style={{ fontSize: 14, lineHeight: "20px" }}>The main parts still come from China</span>
            </>
          ) : (
            <>
              <span style={{ fontSize: 13, color: "var(--dim)" }}>Likely country with this mix</span>
              <span className="biz-stamp" style={{ color: "var(--dim)", fontSize: 20, alignSelf: "flex-start" }}>NOT CHECKED YET</span>
              <span style={{ fontSize: 14, lineHeight: "20px" }}>No past decision backs this mix. Check it to see the likely country</span>
              <Link href="/business/check" className="biz-link" style={{ fontSize: 14 }}>Check this mix</Link>
            </>
          )}
        </div>
      </div>
    </section>
  );
}

function Chapter({ i, stack, children }: { i: number; stack?: boolean; children: ReactNode }) {
  return (
    <section id={`ch${i + 1}`} data-i={i} aria-label={chapters[i]} className={`biz-ch${stack ? " stack" : ""}`}>
      {children}
    </section>
  );
}

function Text({ n, title, sub, wide, children }: { n: number; title: ReactNode; sub: string; wide?: boolean; children?: ReactNode }) {
  return (
    <div className="biz-text" style={wide ? { flex: 1, minWidth: 320 } : undefined}>
      <span className="biz-eyebrow">
        <span className="mono">0{n}</span>
        {chapters[n - 1]}
      </span>
      <h2>{title}</h2>
      <p className="biz-sub">{sub}</p>
      {children}
    </div>
  );
}

// "Next" scrolls the story to the following chapter.
function Actions({ next, secondary }: { next: string; secondary?: ReactNode }) {
  return (
    <div className="biz-actions">
      <button
        type="button"
        className="biz-btn accent"
        onClick={(e) => {
          const ch = (e.currentTarget as HTMLElement).closest(".biz-ch");
          (ch?.nextElementSibling as HTMLElement | null)?.scrollIntoView({ block: "start" });
        }}
      >
        {next}
      </button>
      {secondary}
    </div>
  );
}

function Role({ n, who, gets, show }: { n: string; who: string; gets: string; show: ReactNode }) {
  return (
    <div className="biz-role">
      <span className="mono" style={{ fontSize: 12, color: "var(--dim)" }}>{n}</span>
      <span className="who">{who}</span>
      {show}
      <span className="gets">{gets}</span>
    </div>
  );
}

function Letter({ cls, refText, fact, stamp, color }: { cls: string; refText: string; fact: string; stamp: string; color: string }) {
  return (
    <article aria-label={refText} className={`biz-letter ${cls}`}>
      <span className="ref mono">{refText}</span>
      {[70, 90, 82, 60].map((w) => <span key={w} className="line" style={{ width: `${w}%` }} />)}
      <span className="fact">{fact}</span>
      {[88, 76].map((w) => <span key={w} className="line" style={{ width: `${w}%` }} />)}
      <span className="biz-stamp" style={{ color, transform: "rotate(-3deg)" }}>{stamp}</span>
    </article>
  );
}

const parts: { name: string; home?: boolean; icon: ReactNode }[] = [
  { name: "Motor and fan", icon: <><circle cx="60" cy="60" r="40" /><circle cx="60" cy="60" r="8" /><path d="M60 52 C60 34 76 28 84 36 M68 60 C86 60 92 76 84 84 M60 68 C60 86 44 92 36 84 M52 60 C34 60 28 44 36 36" /></> },
  { name: "Circuit board", icon: <><rect x="16" y="28" width="88" height="64" rx="4" /><rect x="30" y="42" width="24" height="20" rx="2" /><rect x="64" y="42" width="14" height="14" rx="2" /><path d="M30 76 h60 M64 66 h26 M86 42 v14" /></> },
  { name: "Switch", icon: <><rect x="36" y="40" width="48" height="40" rx="6" /><rect x="48" y="48" width="24" height="12" rx="3" /><path d="M44 80 v12 M76 80 v12" /></> },
  { name: "Power cord", icon: <><path d="M20 90 C20 60 50 60 50 80 C50 100 80 100 80 70 C80 44 60 40 60 28" /><rect x="52" y="12" width="16" height="16" rx="3" /><path d="M56 12 v-6 M64 12 v-6" /></> },
  { name: "Housing", home: true, icon: <><path d="M14 48 a12 12 0 0 1 12 -12 h56 v48 h-56 a12 12 0 0 1 -12 -12z" /><path d="M82 46 h22 v28 h-22" /><path d="M40 84 l-8 26 h18 l8 -26" /></> },
  { name: "Final assembly", home: true, icon: <><path d="M30 90 l40 -40" /><path d="M66 46 l12 -12 l8 8 l-12 12z" /><path d="M24 96 l8 -8 l6 6 l-8 8z" /></> },
];

// The heat gun as a blueprint, with its steel rating plate. Before the move
// four parts are tagged CN and the plate reads China; after, all read TH.
function HeatGun({ after }: { after: boolean }) {
  const c = after ? "var(--ok)" : "var(--fail)";
  const tag = (x: number, y: number, home = false) => {
    const th = after || home;
    return (
      <g>
        <rect x={x} y={y} width="26" height="16" rx="4" fill={th ? "var(--ok-wash)" : "var(--fail-wash)"} />
        <text x={x + 13} y={y + 12} textAnchor="middle" fill={th ? "var(--ok)" : "var(--fail)"}>{th ? "TH" : "CN"}</text>
      </g>
    );
  };
  return (
    <svg viewBox="0 0 560 400" width="100%" role="img" aria-label={after ? "Heat gun blueprint. Its rating plate reads Made in Thailand and every part is tagged TH" : "Heat gun blueprint. Its rating plate reads Made in China; motor, circuit board, switch and cord are tagged CN, the housing TH"}>
      <defs>
        <linearGradient id={`steel-${after ? "b" : "a"}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stopColor="#f6f8fb" />
          <stop offset="1" stopColor="#cfd8e4" />
        </linearGradient>
      </defs>
      <g fill="var(--sheet)" stroke="var(--ink)" strokeWidth="1.5" strokeLinejoin="round">
        <path d="M40 110 h300 a16 16 0 0 1 16 16 v108 a16 16 0 0 1 -16 16 h-300 a16 16 0 0 1 -16 -16 v-108 a16 16 0 0 1 16 -16z" />
        <path d="M356 140 h120 v80 h-120z" />
        <path d="M476 150 h40 v60 h-40z" />
        <path d="M150 250 l-28 120 h76 l28 -120z" />
      </g>
      <g fill="none" stroke="var(--guide)" strokeWidth="1" strokeDasharray="4 4">
        <circle cx="88" cy="180" r="40" />
        <path d="M170 262 h28 v22 h-28z" />
      </g>
      <g transform="translate(160 128) rotate(-2)">
        <rect width="168" height="96" rx="6" fill={`url(#steel-${after ? "b" : "a"})`} stroke="var(--edge)" strokeWidth="1" />
        {[[10, 10], [158, 10], [10, 86], [158, 86]].map(([x, y]) => <circle key={`${x}${y}`} cx={x} cy={y} r="2.5" fill="var(--edge)" />)}
        <text x="20" y="30" fontFamily="var(--font-paper-mono), monospace" fontSize="11" fill="var(--dim)">HEAT GUN · 12.5 A</text>
        <rect x="18" y="44" width="132" height="34" rx="4" fill="none" stroke={c} strokeWidth="2" />
        <text x="84" y="66" textAnchor="middle" fontFamily="var(--font-paper-mono), monospace" fontSize={after ? 12.5 : 14} fontWeight="500" fill={c} letterSpacing="1">{after ? "MADE IN THAILAND" : "MADE IN CHINA"}</text>
      </g>
      <g fontFamily="var(--font-paper-mono), monospace" fontSize="11" fill="var(--dim)">
        <path d="M88 140 V60 H60" fill="none" stroke="var(--guide)" />
        <text x="24" y="54">motor-fan</text>{tag(94, 44)}
        <path d="M300 128 V60 H330" fill="none" stroke="var(--guide)" />
        <text x="334" y="54">circuit board</text>{tag(430, 44)}
        <path d="M198 273 H300" fill="none" stroke="var(--guide)" />
        <text x="306" y="277">switch</text>{tag(356, 266)}
        <path d="M136 370 H60" fill="none" stroke="var(--guide)" />
        <text x="18" y="392">cord</text>{tag(54, 380)}
        <path d="M420 220 V320 H450" fill="none" stroke="var(--guide)" />
        <text x="454" y="324">housing</text>{tag(510, 313, true)}
      </g>
    </svg>
  );
}

// Duty drawn like a dimension on a drawing: 0 to 50%, y = 520 − pct × 9.2.
function DutyRuler() {
  const ticks = [0, 10, 20, 30, 40, 50];
  return (
    <svg viewBox="0 0 520 600" width="100%" role="img" aria-label="Import tax on a scale from 0 to 50 percent. Made in China about 41.7 percent, Made in Thailand about 16.7 percent, about 25 points apart">
      <g fontFamily="var(--font-paper-mono), monospace" fontSize="11" fill="var(--dim)">
        <path d="M60 60 V520" stroke="var(--ink)" strokeWidth="1.5" />
        {ticks.map((t) => (
          <g key={t}>
            <path d={`M52 ${520 - t * 9.2} h8`} stroke="var(--ink)" strokeWidth="1.5" />
            <text x="44" y={524 - t * 9.2} textAnchor="end">{t === 50 ? "50%" : t}</text>
          </g>
        ))}
      </g>
      <path d="M60 136 H300" stroke="var(--fail)" strokeWidth="3" />
      <path d="M60 366 H300" stroke="var(--ok)" strokeWidth="3" />
      <g stroke="var(--dimension)" fill="var(--dimension)">
        <path d="M230 142 V360" fill="none" />
        <path d="M230 140 l-5 10 h10z" />
        <path d="M230 362 l-5 -10 h10z" />
      </g>
      <text x="242" y="256" fontFamily="var(--font-paper-mono), monospace" fontSize="15" fill="var(--dimension)">≈25 points</text>
      <text x="316" y="112" fontSize="14" fill="var(--dim)">Made in China</text>
      <text x="312" y="160" fontFamily="var(--font-paper-mono), monospace" fontSize="56" fontWeight="300" fill="var(--fail)">≈41.7%</text>
      <text x="316" y="342" fontSize="14" fill="var(--dim)">Made in Thailand</text>
      <text x="312" y="390" fontFamily="var(--font-paper-mono), monospace" fontSize="56" fontWeight="300" fill="var(--ok)">≈16.7%</text>
    </svg>
  );
}
