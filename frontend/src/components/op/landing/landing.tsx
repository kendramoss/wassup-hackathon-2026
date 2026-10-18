"use client";

// Landing page (DESIGN.md, locked 17 Oct, frame p-landing-3d.html): hero and
// fig.1, the heat gun as a 3D blueprint that explodes and moves its Chinese
// parts to Thailand as you scroll; fig.2, one case as four sheets of real
// output with the facts drawn through them; fig.3, a test sheet for the 377
// held-out rulings. Every number comes from docs/NUMBERS.md.
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { ButtonLink, RulingLink } from "../ui";
import { startLanding } from "./motion";
import "./landing.css";

const L = copy.landing;
const flag = (c: "China" | "Thailand") => `/flags/${c === "China" ? "cn" : "th"}.svg`;
const Flag = ({ c, h }: { c: "China" | "Thailand"; h?: number }) => (
  // eslint-disable-next-line @next/next/no-img-element
  <img className="flag" alt="" src={flag(c)} style={h ? { height: h } : undefined} />
);

// The demo link: straight to the pinned heat-gun case when there is one,
// otherwise the demo loaded into a new case, and /cases if the API is down.
function useDemoHref() {
  const [href, setHref] = useState("/cases");
  useEffect(() => {
    let live = true;
    (async () => {
      try {
        const [cases, demos] = await Promise.all([api.listCases(), api.demos()]);
        // The 2022-setup case tells the landing story (CBP held China; the
        // plan moves work to Thailand); other heat-gun demos are variants (#149).
        const heat = cases.cases.filter((c) => c.pinned && /heat gun|kobalt/i.test(`${c.display_name ?? ""} ${c.product_name ?? ""}`));
        const pinned = heat.find((c) => /2022 setup/i.test(c.display_name ?? "")) ?? heat[0];
        const demo = (demos.demos ?? []).find((d) => /heat/i.test(`${d.id ?? ""} ${d.title ?? ""}`));
        if (!live) return;
        if (pinned) setHref(`/cases/${encodeURIComponent(pinned.id)}/origin`);
        else if (demo?.id) setHref(`/cases/new?demo=${encodeURIComponent(demo.id)}`);
      } catch {
        // keep /cases, where the demo cards are
      }
    })();
    return () => {
      live = false;
    };
  }, []);
  return href;
}

export function Landing() {
  const root = useRef<HTMLDivElement>(null);
  const demoHref = useDemoHref();
  useEffect(() => startLanding(root.current!, { flag }), []);

  const hero = (
    <>
      <span className="dimn">{L.eyebrow}</span>
      <h1>{L.title}</h1>
      <p className="sub duo">{L.sub}</p>
      <div className="ctas">
        <ButtonLink href={demoHref}>{L.demo}</ButtonLink>
        <ButtonLink href="/cases/new" variant="ghost">
          {copy.cases.newCase}
        </ButtonLink>
      </div>
    </>
  );

  return (
    <div className="lp" ref={root}>
      <main id="main">
        <div className="hero-m">{hero}</div>

        {/* Hero + fig.1: the sheet stays put while the story steps scroll past. */}
        <div className="scrolly" data-k="scrolly">
          <div className="stick">
            <div className="left">
              <div className="hero-d">{hero}</div>
              <ol className="steps" aria-label="The heat gun, step by step">
                {L.steps.map((s, i) => (
                  <li key={s.title} data-k="step">
                    <span className="n">{i + 1}</span>
                    <div>
                      <b>{s.title}</b>
                      <p className="duo">
                        {s.body}
                        {"ruling" in s && s.ruling ? (
                          <>
                            {" "}(<RulingLink id={s.ruling} />).
                          </>
                        ) : null}
                        {"for" in s && s.for ? (
                          <>
                            {" "}(<RulingLink id={s.for} /> for; no counter-ruling found).
                          </>
                        ) : null}
                      </p>
                    </div>
                  </li>
                ))}
              </ol>
            </div>
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
                  </div>
                </div>
                <div className="stamp" data-k="stamp" aria-live="polite" />
                <div className="plannote" data-k="plannote">
                  {L.planNote}
                  <br />
                  {L.planMove}
                </div>
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
                <div className="note">{L.schematic}</div>
                <div className="tblock" aria-hidden="true">
                  <span>{copy.name}</span>
                  <span>Heat gun · Kobalt 12.5-amp · model 5208257</span>
                  <span>fig.1</span>
                  <span data-k="tbstate">{L.sheetStates[0]}</span>
                </div>
                <p className="caption duo" data-k="caption" />
              </figure>
            </div>
          </div>
        </div>

        {/* The gun's centre line runs on and becomes fig.2's wire. */}
        <div className="bridge" data-k="bridge" aria-hidden="true">
          <svg>
            <path data-k="bridgepath" fill="none" stroke="var(--dimension)" strokeWidth="1.5" strokeDasharray="14 5" />
          </svg>
          <p className="duo">{L.bridge}</p>
        </div>

        <section className="f2" data-k="f2" aria-labelledby="f2h">
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
                <path data-k="lead" fill="none" stroke="var(--dimension)" strokeWidth="1.5" strokeDasharray="14 5" />
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
                    <p className="duo">Make the motor-fan assembly in Thailand</p>
                    <p className="gap">
                      <span className="chip">Closest move · check 0.90, under the 0.92 bar</span>
                    </p>
                    <p className="small">
                      For <RulingLink id="N325810" /> · against: none found
                    </p>
                    <p className="duo small">Below the bar, so it goes to a broker with the evidence instead of being shown as a plan.</p>
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

        <div className="f3wrap">
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
                  aria-label="Origin accuracy 90.2% (86.8 to 92.8) against 60.5% for the best simple rule and 40.6% for final assembly. Auto-accepted cases, 79.6% of all, 96.0% correct (93.1 to 97.7)."
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
                <div className="field" data-k="field" role="img" aria-label="377 held-out rulings: 288 auto-accepted and right, 12 auto-accepted and wrong, 52 sent to review and right, 25 sent to review and wrong." />
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
                  For · <RulingLink id="N325810" />
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
              <Link href="/accuracy">{L.more}</Link>
            </div>
          </section>
        </div>

        <section className="who" aria-labelledby="whoh">
          <h2 id="whoh" className="eyebrow">
            {L.whoTitle}
          </h2>
          <div className="whobox">
            <p className="duo">{L.who}</p>
            <p className="duo">{L.whoFact}</p>
          </div>
        </section>
      </main>
      <footer className="lpfoot">
        <span>{L.footerDisclaimer}</span>
        <span>{L.built}</span>
        <a href={L.repo} target="_blank" rel="noreferrer">
          GitHub
        </a>
      </footer>
    </div>
  );
}

