"use client";

// Landing page (DESIGN.md, locked 17 Oct, frame p-landing-3d.html): the hero,
// then one stage pinned on wide screens whose three figures scroll sideways
// as you scroll down (D93): fig.1, the heat gun as a 3D blueprint that
// explodes and moves its Chinese parts to Thailand, a caption box naming
// each phase; fig.2, one case as four sheets of real output with the facts
// drawn through them; fig.3, a test sheet for the 377 held-out rulings,
// ending on the way into the cases. Phones keep the figures stacked. After
// "Who it's for", fig.4 tells the impact as one illustrative story. Every
// number comes from docs/NUMBERS.md.
import Link from "next/link";
import { Explained } from "../term";
import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { ButtonLink, RulingLink } from "../ui";
import { startLanding } from "./motion";
import "./landing.css";

const L = copy.landing;
const S = L.story;
const K = copy.stakeholders;
const flag = (c: "China" | "Thailand") => `/flags/${c === "China" ? "cn" : "th"}.svg`;
const Flag = ({ c, h }: { c: "China" | "Thailand"; h?: number }) => (
  // eslint-disable-next-line @next/next/no-img-element
  <img className="flag" alt="" src={flag(c)} style={h ? { height: h } : undefined} />
);

// The demo links: straight to the pinned heat-gun case when there is one,
// otherwise the demo loaded into a new case, and /cases if the API is down.
// The pinned comparison case is a separate demo (its comparison is recorded).
type DemoLinks = { demo: string; heat: string | null; compare: string };
function useDemoLinks(): DemoLinks {
  const [links, setLinks] = useState<DemoLinks>({ demo: "/cases", heat: null, compare: "/cases" });
  useEffect(() => {
    let live = true;
    (async () => {
      try {
        const [cases, demos] = await Promise.all([api.listCases(), api.demos()]);
        // The 2022-setup case tells the landing story (CBP held China; the
        // plan moves work to Thailand); other heat-gun demos are variants (#149).
        const heat = cases.cases.filter((c) => c.pinned && /heat gun|kobalt/i.test(`${c.display_name ?? ""} ${c.product_name ?? ""}`));
        const pinned = heat.find((c) => /2022 setup/i.test(c.display_name ?? "")) ?? heat.find((c) => !/compare/i.test(c.display_name ?? ""));
        const cmp = heat.find((c) => /compare/i.test(c.display_name ?? ""));
        const demo = (demos.demos ?? []).find((d) => /heat/i.test(`${d.id ?? ""} ${d.title ?? ""}`));
        if (!live) return;
        const heatCase = pinned ? `/cases/${encodeURIComponent(pinned.id)}` : null;
        setLinks({
          demo: heatCase ? `${heatCase}/origin` : demo?.id ? `/cases/new?demo=${encodeURIComponent(demo.id)}` : "/cases",
          heat: heatCase,
          compare: cmp ? `/cases/${encodeURIComponent(cmp.id)}/compare` : "/cases",
        });
      } catch {
        // keep /cases, where the demo cards are
      }
    })();
    return () => {
      live = false;
    };
  }, []);
  return links;
}

const C = L.can;
// Where each "What you can do" line leads. Heat-gun steps fall back to the
// demo link (a new case from the demo, or /cases) when no pinned case loads.
function canHref(key: string, l: DemoLinks): string {
  switch (key) {
    case "origin":
      return l.demo;
    case "import":
      return "/cases/new";
    case "plan":
      return l.heat ? `${l.heat}/plan` : l.demo;
    case "compare":
      return l.compare;
    case "draft":
      return l.heat ? `${l.heat}/draft` : l.demo;
    case "mcp":
      return "/developers#mcp";
    case "method":
      return "/method";
    default:
      return "/accuracy#misses";
  }
}

export function Landing() {
  const root = useRef<HTMLDivElement>(null);
  const links = useDemoLinks();
  const demoHref = links.demo;
  useEffect(() => startLanding(root.current!, { flag }), []);

  const hero = (
    <>
      <span className="dimn">{L.eyebrow}</span>
      <h1>{L.title}</h1>
      <p className="sub read">{L.sub}</p>
      <div className="ctas">
        <ButtonLink href={demoHref}>{L.demo}</ButtonLink>
        <ButtonLink href="/cases/new" variant="ghost">
          {copy.cases.newCase}
        </ButtonLink>
      </div>
      <p className="also read">
        {L.also} <a href="#can">{L.alsoLink}</a>
      </p>
    </>
  );

  return (
    <div className="lp" ref={root}>
      <main id="main">
        <section className="hero">{hero}</section>

        {/* Wide screens: the stage stays pinned and the track moves sideways
            as the page scrolls down; phones stack the three panes. */}
        <div className="hz" data-k="hz">
          <div className="hzpin" data-k="hzpin">
            <div className="track" data-k="track">
              <div className="pane p1" data-k="pane">
                <div className="scroller" data-k="scroller">
                  <figure className="sheet" aria-label={L.fig1Label} data-k="gunsheet">
                    <canvas data-k="canvas" aria-hidden="true" />
                    <div className="overlay" aria-hidden="true">
                      <svg data-k="leaders" />
                      <div data-k="labels" />
                    </div>
                    <div className="sheethead">
                      <div>
                        <div className="eyebrow">{L.fig1}</div>
                        <div className="dimn">{L.fig1Sub}</div>
                          <div className="note">{L.schematic}</div>
                      </div>
                    </div>
                    <div className="stamp" data-k="stamp" aria-live="polite" />
                    <div className="plannote" data-k="plannote">
                      {L.planNote}
                      <br />
                      {L.planMove}
                    </div>
                    <div className="tblock" aria-hidden="true">
                      <span>{copy.name}</span>
                      <span>Heat gun · Kobalt 12.5-amp · model 5208257</span>
                      <span>fig.1</span>
                      <span data-k="tbstate">{L.sheetStates[0]}</span>
                    </div>
                    <div className="caption" data-k="caption">
                      {L.steps.map((st, i) => (
                        <div key={st.title} className="cap" data-k="cap" hidden={i > 0}>
                          <span className="eyebrow">
                            {L.stepOf(i + 1)} · {st.title}
                          </span>
                          <p className="read">
                            {i === 2 ? <Explained text={st.body} terms={["pcba", "check", "bar"]} /> : st.body}
                            {"ruling" in st && st.ruling ? (
                              <>
                                {" "}
                                (<RulingLink id={st.ruling} />).
                              </>
                            ) : null}
                            {"for" in st && st.for ? (
                              <>
                                {" "}
                                (<RulingLink id={st.for} /> for; no counter-ruling found).
                              </>
                            ) : null}
                          </p>
                        </div>
                      ))}
                      <div className="legend" aria-hidden="true">
                        <span>
                          <i className="cn" />
                          {L.legendCn}
                        </span>
                        <span>
                          <i />
                          {L.legendTh}
                        </span>
                        <span>{L.drag}</span>
                      </div>
                    </div>
                  </figure>
                </div>
              </div>

              <section className="pane f2" data-k="f2" aria-labelledby="f2h">
                <div className="pin">
                  <div className="head">
                    <div>
                      <h2 id="f2h" className="eyebrow">
                        {L.fig2}
                      </h2>
                      <span className="dimn">{L.fig2Sub}</span>
                    </div>
                    <div className="now duo" data-k="now" />
                  </div>
                  <div className="stage" data-k="stage">
                    <svg className="path" data-k="pathsvg" aria-hidden="true">
                      <path data-k="wire" fill="none" stroke="var(--ok)" strokeWidth="1.5" />
                    </svg>
                    <article className="mini" data-k="mini">
                      <div className="hd">
                        <span>
                          <span className="ball">1</span>Product
                        </span>
                        <span>sheet 1/4</span>
                      </div>
                      <div className="bd">
                        <div className="real">
                          <table>
                            <tbody>
                              {(
                                [
                                  ["Main housing", "Thailand"],
                                  ["Heating element", "Thailand"],
                                  ["Motor-fan assembly", "China"],
                                  ["PCBA", "China"],
                                  ["Switch, cord, minor", "China"],
                                ] as const
                              ).map(([p, c], i) => (
                                <tr key={p}>
                                  <td className={i === 2 ? "hl" : undefined}>{p}</td>
                                  <td>
                                    <Flag c={c} /> {c}
                                  </td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                          <p className="duo small">Final assembly in Thailand.</p>
                        </div>
                      </div>
                      <div className="ft">Router · Gemini 3.8 Flash</div>
                    </article>
                    <article className="mini" data-k="mini">
                      <div className="hd">
                        <span>
                          <span className="ball">2</span>Origin
                        </span>
                        <span>sheet 2/4</span>
                      </div>
                      <div className="bd">
                        <div className="real">
                          <span className="eyebrow">Likely origin</span>
                          <div className="big">
                            <Flag c="China" h={22} />
                            China
                          </div>
                          <p className="duo body">Read against earlier CBP rulings, each one linked.</p>
                          <p className="duo small">
                            CBP held China (<RulingLink id="N328954" />
                            ).
                          </p>
                        </div>
                      </div>
                      <div className="ft">Origin engine · Gemini 4 Argon</div>
                    </article>
                    <article className="mini" data-k="mini">
                      <div className="hd">
                        <span>
                          <span className="ball">3</span>Plan
                        </span>
                        <span>sheet 3/4</span>
                      </div>
                      <div className="bd">
                        <div className="real">
                          <p className="duo">Make the motor-fan assembly, the PCBA and the switch, cord and minor parts in Thailand</p>
                          <p className="gap">
                            <span className="chip">Recorded 18 Oct plan · check 0.96, above the 0.92 bar</span>
                          </p>
                          <p className="small">
                            For <RulingLink id="N345938" /> · against: none found
                          </p>
                          <p className="small">
                            Recorded 18 Oct production run (docs/DEMO.md, section 6). The pinned case on this site may instead show no plan above the bar.
                          </p>
                          <p className="duo small">
                            Matches CBP&apos;s 2026 change (<RulingLink id="N357771" />), all but the outer shells.
                          </p>
                        </div>
                      </div>
                      <div className="ft">Planner + verifier agent</div>
                    </article>
                    <article className="mini" data-k="mini">
                      <div className="hd">
                        <span>
                          <span className="ball">4</span>Draft
                        </span>
                        <span>sheet 4/4</span>
                      </div>
                      <div className="bd">
                        <div className="real">
                          <div className="letter duo">
                            Request for a binding ruling on the country of origin of a heat gun…
                            <i />
                            <i style={{ width: "80%" }} />
                            <i style={{ width: "90%" }} />
                            <i style={{ width: "60%" }} />
                          </div>
                        </div>
                      </div>
                      <div className="ft">For a licensed customs broker</div>
                    </article>
                  </div>
                  <div className="foot">
                    <span>{L.fig2Foot}</span>
                  </div>
                </div>
              </section>

              <div className="pane f3wrap" data-k="f3pane">
                <section className="test" data-k="f3" aria-labelledby="f3h">
                  <span className="reg tl">
                    <i />
                  </span>
                  <span className="reg tr">
                    <i />
                  </span>
                  <span className="reg bl">
                    <i />
                  </span>
                  <span className="reg br">
                    <i />
                  </span>
                  <span className="crop tl" />
                  <span className="crop tr" />
                  <span className="crop bl" />
                  <span className="crop br" />
                  <div className="testhead">
                    <div>
                      <div className="eyebrow">{L.fig3}</div>
                      <h2 id="f3h">{L.fig3Title}</h2>
                    </div>
                    <div>
                      <div className="meta">{L.fig3Meta}</div>
                      <div className="swatches" aria-hidden="true">
                        {["--ink", "--ok", "--ask", "--fail", "--edge", "--wash"].map((v) => (
                          <i key={v} style={{ background: `var(${v})` }} />
                        ))}
                      </div>
                    </div>
                  </div>
                  <div className="panels">
                    <div className="panel">
                      <div className="ph">
                        <span className="eyebrow">{L.scaleHead}</span>
                        <span className="dimn">brackets = 95% interval</span>
                      </div>
                      <div className="meas" aria-hidden="true">
                        <span className="mv" data-k="mv">
                          90.2%
                        </span>
                        <span className="ml">
                          <i />
                          <b>86.8–92.8 · 95% interval</b>
                          <i />
                        </span>
                      </div>
                      <svg
                        className="ruler"
                        data-k="ruler"
                        viewBox="0 0 700 190"
                        role="img"
                        aria-label="Origin accuracy 90.2% (86.8 to 92.8) against 63.4% for the best simple rule we recomputed, the first country named. Auto-accepted cases, 79.6% of all, 96.0% correct (93.1 to 97.7)."
                      />
                      <div className="claims">
                        <p className="duo">
                          <b>90.2%</b> {L.claimAccuracy}
                        </p>
                        <p className="duo">
                          <b>96.0%</b> {L.claimAuto}
                        </p>
                      </div>
                    </div>
                    <div className="panel">
                      <div className="ph">
                        <span className="eyebrow">{L.squaresHead}</span>
                        <span className="dimn">sorted by outcome</span>
                      </div>
                      <div
                        className="field"
                        data-k="field"
                        role="img"
                        aria-label="377 held-out rulings: 288 auto-accepted and right, 12 auto-accepted and wrong, 52 sent to review and right, 25 sent to review and wrong."
                      />
                      <div className="key">
                        <span>
                          <i className="ka" />
                          auto-accepted, right <b>288</b>
                        </span>
                        <span>
                          <i className="kb" />
                          wrong <b>12</b>
                        </span>
                        <span>
                          <i className="kc" />
                          to review, right <b>52</b>
                        </span>
                        <span>
                          <i className="kd" />
                          wrong <b>25</b>
                        </span>
                      </div>
                    </div>
                  </div>
                  <div className="forag">
                    <div className="s">
                      <span className="rul duo">
                        For · <RulingLink id="N345938" />
                      </span>
                      <span className="beam" />
                    </div>
                    <div className="mid">
                      <span className="dimn">{L.bothWays}</span>
                      <b>{L.planMove}</b>
                    </div>
                    <div className="s r">
                      <span className="rul ag duo">Against · none found</span>
                      <span className="beam" />
                    </div>
                  </div>
                  <div className="testfoot">
                    <span>{copy.name} · test sheet 01</span>
                    <span className="endcta" data-k="endcta">
                      <Link href="/accuracy">{L.more}</Link>
                      <ButtonLink href="/cases">{L.cta}</ButtonLink>
                    </span>
                  </div>
                </section>
              </div>
            </div>
          </div>
        </div>

        {/* What you can do (judge journey): every built capability with a
            way in, so the newer ones are found without reading the docs. */}
        <section className="can" id="can" aria-labelledby="canh">
          <h2 id="canh" className="eyebrow">
            {C.title}
          </h2>
          <p className="read canlead">{C.lead}</p>
          <dl>
            {C.items.map((it) => (
              <div key={it.key} className="canrow">
                <dt className="read">{it.what}</dt>
                <dd>
                  <p className="read">{it.body}</p>
                  {"code" in it && it.code ? (
                    <pre className="cancode">
                      <code>{it.code}</code>
                    </pre>
                  ) : null}
                  <Link href={canHref(it.key, links)}>{it.link}</Link>
                </dd>
              </div>
            ))}
          </dl>
        </section>

        <section className="who" aria-labelledby="whoh">
          <h2 id="whoh" className="eyebrow">
            {L.whoTitle}
          </h2>
          <div className="whobox">
            <p className="read">{L.who}</p>
            <p className="read">{L.whoFact}</p>
          </div>
          {/* Who uses it and who benefits (review follow-up): roles the
              product is designed for, plain text with no numbers. */}
          <section className="roles" aria-labelledby="rolesh">
            <h3 id="rolesh" className="eyebrow">
              {K.title}
            </h3>
            <p className="read rolelead">{K.lead}</p>
            <dl>
              {K.roles.map((r) => (
                <div key={r.role} className="role">
                  <dt>
                    <span className="eyebrow">{r.role}</span>
                    <span className="read">{r.who}</span>
                  </dt>
                  <dd className="read">{r.gets}</dd>
                </div>
              ))}
            </dl>
          </section>
        </section>

        {/* fig.4, the impact told as one story (review follow-up). It sits
            after "Who it's for" so the reader meets the user first and then
            follows one of them through a decision. Measured figures get bars
            with their values on them; system properties are plain text. The
            firm is a labelled composite. */}
        <section className="story" aria-labelledby="storyh">
          <div className="storysheet">
            <div className="storyhead">
              <div>
                <div className="eyebrow">{S.fig}</div>
                <h2 id="storyh">{S.title}</h2>
              </div>
              <p className="illus">{S.label}</p>
            </div>
            <ol className="beats">
              <li className="beat">
                <span className="bn">
                  <b>1</b>
                  {S.beats.decision}
                </span>
                <div className="bc">
                  {S.decision.map((t) => (
                    <p key={t} className="read">
                      {t}
                    </p>
                  ))}
                </div>
              </li>
              <li className="beat">
                <span className="bn">
                  <b>2</b>
                  {S.beats.oldWay}
                </span>
                <div className="bc">
                  <p className="read">{S.oldWayLead}</p>
                  <div className="cmp">
                    <div className="cmphead dimn" aria-hidden="true">
                      <span>
                        <i className="bw" />
                        {S.keyBefore}
                      </span>
                      <span>
                        <i className="bu" />
                        {S.keyAfter}
                      </span>
                    </div>
                    {S.rows.map((r) => (
                      <div key={r.label} className="cmprow">
                        <span className="eyebrow">{r.label}</span>
                        <div className="bar">
                          <span className="bt" aria-hidden="true">
                            <i className="bw" />
                          </span>
                          <span className="val">
                            <b>{r.before}</b> {r.beforeNote}
                          </span>
                        </div>
                        <div className="bar">
                          <span className="bt" aria-hidden="true">
                            <i className="bu" style={{ width: `max(2px, ${(r.ratio * 100).toFixed(6)}%)` }} />
                          </span>
                          <span className="val">
                            <b className="ok">{r.after}</b> {r.afterNote}
                          </span>
                        </div>
                      </div>
                    ))}
                    <div className="dimn">{S.minBar}</div>
                  </div>
                  <p className="read">{S.oldWayRisk}</p>
                </div>
              </li>
              <li className="beat">
                <span className="bn">
                  <b>3</b>
                  {S.beats.withUnde}
                </span>
                <div className="bc">
                  {S.withUnde.map((t, i) => (
                    <p key={t} className="read">
                      {t}
                      {i === 1 ? (
                        <>
                          {" "}
                          {S.withUndeRuling} <RulingLink id="N345938" />.
                        </>
                      ) : null}
                    </p>
                  ))}
                </div>
              </li>
              <li className="beat">
                <span className="bn">
                  <b>4</b>
                  {S.beats.outcome}
                </span>
                <div className="bc">
                  <p className="read">{S.outcomeLead}</p>
                  <div className="cmp">
                    <div className="cmphead dimn" aria-hidden="true">
                      {S.dutyHead}
                    </div>
                    {S.duty.map((d) => (
                      <div key={d.country} className="cmprow">
                        <span className="eyebrow">
                          <Flag c={d.country} /> {d.country} origin · {d.rate}
                        </span>
                        <div className="bar">
                          <span className="bt" aria-hidden="true">
                            <i className={d.country === "China" ? "bw" : "bu"} style={{ width: `${(d.share / S.duty[0].share) * 100}%` }} />
                          </span>
                          <span className="val">
                            <b className={d.country === "China" ? undefined : "ok"}>{d.value}</b> {d.sum}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                  <ul className="sums read">
                    <li>{S.dutyGap}</li>
                    <li>{S.dutyCost}</li>
                    <li>
                      <b>{S.dutyNet}</b>
                    </li>
                  </ul>
                </div>
              </li>
              <li className="beat">
                <span className="bn">
                  <b>5</b>
                  {S.beats.broker}
                </span>
                <div className="bc">
                  <ul className="todo read">
                    {S.broker.map((t) => (
                      <li key={t}>{t}</li>
                    ))}
                  </ul>
                </div>
              </li>
            </ol>
            <div className="storyfoot">
              <span>{S.sources}</span>
              <span>{copy.name} · sheet 04</span>
            </div>
          </div>
        </section>
      </main>
      <footer className="lpfoot">
        <span>{L.footerDisclaimer}</span>
        <span>{L.built}</span>
        <a href={L.repo} target="_blank" rel="noreferrer">
          GitHub
        </a>
        <a href={`${L.repo}/blob/main/docs/ROADMAP.md`} target="_blank" rel="noreferrer">
          {L.roadmap}
        </a>
        <Link href="/method">{copy.method.footer}</Link>
        <Link href="/developers">{copy.nav.api}</Link>
        <Link href="/privacy">{copy.legal.footer.privacy}</Link>
        <Link href="/terms">{copy.legal.footer.terms}</Link>
      </footer>
    </div>
  );
}
