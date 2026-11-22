"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { ButtonLink, RulingLink } from "../ui";
import { JudgeImpact, JudgeProof } from "./judge-proof";
import { JudgePersonas } from "./judge-tabs";
import { JudgeWorkflow } from "./judge-workflow";
import { startHeatGun } from "./motion";
import "./landing.css";

const L = copy.landing;
const flag = (country: "China" | "Thailand") => `/flags/${country === "China" ? "cn" : "th"}.svg`;

// The demo links: straight to the pinned heat-gun case when there is one,
// otherwise the demo loaded into a new case, and /cases if the API is down.
// The pinned comparison case is a separate demo (its comparison is recorded).
// The hero's button opens the tour on the plan (judge panel round 4): the
// origin step, which opens on "needs more facts", is one click from there.
type DemoLinks = { demo: string; tour: string; compare: string };
function useDemoLinks(): DemoLinks {
  const [links, setLinks] = useState<DemoLinks>({ demo: "/cases", tour: "/cases", compare: "/cases" });
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
        const fallback = demo?.id ? `/cases/new?demo=${encodeURIComponent(demo.id)}` : "/cases";
        setLinks({
          demo: heatCase ? `${heatCase}/origin` : fallback,
          tour: heatCase ? `${heatCase}/plan?tour=1` : fallback,
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

const heatGunPhases = [
  { label: "2022 setup", headline: "China origin", status: "CBP ruling", tone: "verified", text: "Assembled in Thailand with key parts from China." },
  { label: "Parts", headline: "Where the parts began", status: "", tone: "", text: "Thailand made the housing and heater. China made the motor, circuit board and electrical parts." },
  { label: "Proposed move", headline: "Make the key parts in Thailand", status: "Needs broker review", tone: "review", text: "Move the motor-fan assembly, circuit board, switch and cord. An earlier ruling supports the proposal." },
  { label: "2026 ruling", headline: "Thailand origin", status: "CBP ruling", tone: "verified", text: "The manufacturer moved six parts to Thailand. The switch, wires and fasteners stayed in China." },
] as const;

function VerificationPill({ id, label }: { id: string; label: string }) {
  return (
    <div className="lp-ruling-reference">
      <RulingLink id={id} className="lp-ruling-pill" />
      <span>{label}</span>
    </div>
  );
}

function HeatGunCase() {
  const [phase, setPhase] = useState(0);
  const visual = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (visual.current) return startHeatGun(visual.current, flag);
  }, []);

  return (
    <section className="lp-section lp-case" id="heat-gun" aria-labelledby="heat-gun-title">
      <div className="lp-section-head">
        <span className="lp-figure-label">Fig. 1</span>
        <h2 id="heat-gun-title">Kobalt heat-gun case study</h2>
      </div>
      <div className="lp-case-panel">
        <ol className="lp-case-steps" aria-label="Heat-gun case steps">
          {heatGunPhases.map((step, index) => (
            <li key={step.label}>
              <button type="button" aria-pressed={phase === index} aria-controls="heat-gun-caption" onClick={() => setPhase(index)}>
                <span className="lp-step-number">{index + 1}</span>
                {step.label}
              </button>
            </li>
          ))}
        </ol>
        <div className="lp-case-board">
          <div className="lp-case-visual" ref={visual} data-phase={phase} aria-hidden="true">
            <canvas />
            <svg />
            <div data-labels />
            <span className="lp-visual-note">Schematic from published ruling facts</span>
          </div>
          <div className="lp-case-story">
            <div className="lp-case-caption" id="heat-gun-caption" aria-live="polite" aria-atomic="true">
              {heatGunPhases[phase].status && (
                <p className="lp-case-tag" data-tone={heatGunPhases[phase].tone}>{heatGunPhases[phase].status}</p>
              )}
              <h3>{heatGunPhases[phase].headline}</h3>
              <p>{heatGunPhases[phase].text}</p>
            </div>
            <div className="lp-ruling-references" role="group" aria-label="Verify the CBP rulings">
              <VerificationPill id="N328954" label="2022 origin" />
              <VerificationPill id="N345938" label="Plan precedent" />
              <VerificationPill id="N357771" label="2026 origin" />
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

export function Landing() {
  const links = useDemoLinks();

  return (
    <div className="lp">
      <main id="main">
        <section className="lp-hero" aria-labelledby="landing-title">
          <p className="lp-figure-label">U.S. customs origin</p>
          <h1 id="landing-title">{L.title}</h1>
          <p className="lp-hero-prop">{L.sub}</p>
          <div className="lp-hero-actions">
            <ButtonLink href={links.tour}>{L.demo}</ButtonLink>
            <ButtonLink href="/cases/new" variant="ghost">{copy.cases.newCase}</ButtonLink>
            <a href="#proof" className="lp-hero-jump">See the benchmarks ↓</a>
          </div>
        </section>

        <HeatGunCase />
        <JudgeWorkflow />
        <JudgeProof />
        <JudgeImpact />
        <JudgePersonas demoHref={links.demo} compareHref={links.compare} />
      </main>

      <footer className="lpfoot">
        <p>{L.footerDisclaimer}</p>
        <nav aria-label="More about Unde Labs">
          <a href={L.repo} target="_blank" rel="noreferrer">GitHub</a>
          <a href={`${L.repo}/blob/main/docs/ROADMAP.md`} target="_blank" rel="noreferrer">{L.roadmap}</a>
          <Link href="/method">{copy.method.footer}</Link>
          <Link href="/developers">{copy.nav.api}</Link>
          <Link href="/privacy">{copy.legal.footer.privacy}</Link>
          <Link href="/terms">{copy.legal.footer.terms}</Link>
        </nav>
      </footer>
    </div>
  );
}
