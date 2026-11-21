// Screenshots every page in light and dark, desktop and mobile, and runs axe
// on each. Use it after any UI change and look at the pictures.
//
//   npm run ui-check                         # against http://localhost:3000
//   npm run ui-check -- https://<url> /accuracy  # another site or other paths
//
// Writes PNGs and report.json to .ui-check/ and exits 1 if axe finds
// violations or a page logs console errors. Uses an installed Chrome or Edge
// (no browser download); set BROWSER=msedge or BROWSER=chrome to pick one.
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { chromium } from "playwright-core";
import { AxeBuilder } from "@axe-core/playwright";

const args = process.argv.slice(2);
const base = (args.find((a) => a.startsWith("http")) ?? "http://localhost:3000").replace(/\/$/, "");
// #522 and #534: use the recorded pinned case so these states do not depend
// on a live backend (or spend a prediction) and are checked on every run.
const reflowId = "case_878d76540b91";
const reflowPath = `/cases/${reflowId}/scenarios`;
const draftPath = `/cases/${reflowId}/draft`;
const paths = args.filter((a) => a.startsWith("/"));
if (paths.length === 0) paths.push("/", "/cases", "/accuracy", "/triage", draftPath);
const out = ".ui-check";

const viewports = { desktop: { width: 1280, height: 860 }, mobile: { width: 390, height: 844 }, narrow: { width: 320, height: 720 } };

const reflowFixture = JSON.parse(await readFile(new URL("../../eval/results/demo-seed-heatgun-scenarios-vertex-prod.json", import.meta.url), "utf8"));

async function launch() {
  // PW_CHROME: a browser binary to use instead (e.g. Playwright's headless shell).
  if (process.env.PW_CHROME) return chromium.launch({ executablePath: process.env.PW_CHROME });
  const channels = process.env.BROWSER ? [process.env.BROWSER] : ["chrome", "msedge"];
  for (const channel of channels) {
    try {
      return await chromium.launch({ channel });
    } catch {}
  }
  throw new Error(`No browser found (tried ${channels.join(", ")}). Install Chrome or Edge, or set BROWSER.`);
}

await mkdir(out, { recursive: true });
const browser = await launch();
const problems = [];

for (const scheme of ["light", "dark"]) {
  for (const [vp, viewport] of Object.entries(viewports)) {
    const ctx = await browser.newContext({ viewport, colorScheme: scheme });
    const page = await ctx.newPage();
    let current = "";
    page.on("console", (m) => m.type() === "error" && problems.push({ page: current, vp, scheme, console: m.text() }));
    page.on("pageerror", (e) => problems.push({ page: current, vp, scheme, console: String(e) }));
    await page.route(`**/api/v1/cases/${reflowId}`, (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(reflowFixture) }),
    );
    await page.route(`**/api/v1/cases/${reflowId}/export`, (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ content: "# Draft\n\nSample letter.", notes: [], format: "markdown" }) }),
    );
    // A pinned demo cannot create review links; the local backend has no
    // review list for it either. Keep the deterministic state console-clean.
    await page.route(`**/api/v1/cases/${reflowId}/reviews`, (route) =>
      route.request().method() === "GET"
        ? route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ case_id: reflowId, reviews: [] }) })
        : route.continue(),
    );
    const checkPaths = vp === "narrow" ? [...new Set([...paths, reflowPath])] : paths;

    for (const path of checkPaths) {
      current = path;
      await page.goto(base + path, { waitUntil: "networkidle" });
      // Hide the Next.js dev-mode badge so it doesn't show up in screenshots.
      await page.addStyleTag({ content: "nextjs-portal { display: none !important; }" });
      const name = path === "/" ? "home" : path.slice(1).replace(/\//g, "-");
      await page.screenshot({ path: `${out}/${name}-${vp}-${scheme}.png`, fullPage: true });

      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
      if (overflow) problems.push({ page: path, vp, scheme, layout: "page scrolls horizontally" });

      // fig.2's caption explains the four sheets. It must never be blank,
      // including on phones where the sheets stack (#507).
      const fig2Now = await page.evaluate(() => {
        const el = document.querySelector(".lp .f2 .now");
        return el ? (el.textContent ?? "").trim() : null;
      });
      if (fig2Now !== null && !fig2Now) problems.push({ page: path, vp, scheme, layout: "fig.2 caption is empty" });

      // #516: the wide-layout sheets are a fixed height, so extra copy can
      // print out of the bottom of a card. Measure with the reveal transform
      // neutralised, so an unreached sheet is checked the same as a reached one.
      const sheetOverflow = await page.evaluate(() => {
        const minis = [...document.querySelectorAll(".lp .mini")];
        if (!minis.length) return [];
        const style = document.createElement("style");
        style.textContent = ".lp .mini .real > * { opacity: 1 !important; transform: none !important; }";
        document.head.appendChild(style);
        const bad = minis.flatMap((m, i) => {
          const mb = m.getBoundingClientRect();
          return [...m.children]
            .filter((c) => c.getBoundingClientRect().bottom > mb.bottom + 1)
            .map((c) => `sheet ${i + 1} .${c.className}`);
        });
        style.remove();
        return bad;
      });
      for (const b of sheetOverflow) problems.push({ page: path, vp, scheme, layout: `fig.2 ${b} overflows its card` });

      const { violations } = await new AxeBuilder({ page }).analyze();
      for (const v of violations) {
        problems.push({ page: path, vp, scheme, axe: v.id, impact: v.impact, help: v.help, nodes: v.nodes.slice(0, 3).map((n) => n.target.join(" ")) });
      }

      // #520: the inline "Cases" link below a completed triage only exists in
      // the results state. Axe must see that state too, or a colour-only link
      // there passes every base-state check.
      if (path === "/triage") {
        // Stub the one heavy batch call so this state is deterministic and
        // does not spend four of the backend's 4/min triage requests.
        await page.route("**/api/v1/cases/batch", (route) =>
          route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({
              summary: { total: 1, failed: 0, auto: 0, needs_info: 0, needs_review: 1, out_of_scope: 0 },
              rows: [{ case_id: "case_uicheck", name: "Sample product", predicted_origin: "China", confidence: 0.6, calibrated_confidence: 0.6, triage_route: "needs_review", reason: "scope unavailable", cost_usd: 0.001, out_of_scope: false, error: "" }],
            }),
          }),
        );
        await page.getByRole("button", { name: /Try the sample/i }).click();
        await page.getByRole("button", { name: /Triage 5 products/i }).click();
        await page.getByRole("heading", { name: /Needs a look first/i }).waitFor({ timeout: 120000 });
        await page.addStyleTag({ content: "nextjs-portal { display: none !important; }" });
        await page.screenshot({ path: `${out}/triage-results-${vp}-${scheme}.png`, fullPage: true });
        const { violations: resultViolations } = await new AxeBuilder({ page }).analyze();
        for (const v of resultViolations) {
          problems.push({ page: `${path} (results)`, vp, scheme, axe: v.id, impact: v.impact, help: v.help, nodes: v.nodes.slice(0, 3).map((n) => n.target.join(" ")) });
        }
      }

      // #534: the broker preview declares aria-modal=true, so Tab must not
      // reach the page behind it. Check that real state, not just the base.
      if (path === draftPath) {
        await page.getByRole("button", { name: /Preview what your broker sees/i }).click();
        const dialog = page.getByRole("dialog", { name: /Preview of the broker review page/i });
        await dialog.waitFor();
        await page.addStyleTag({ content: "nextjs-portal { display: none !important; }" });
        await page.screenshot({ path: `${out}/draft-preview-modal-${vp}-${scheme}.png`, fullPage: true });
        const { violations: modalViolations } = await new AxeBuilder({ page }).analyze();
        for (const v of modalViolations) {
          problems.push({ page: `${path} (modal)`, vp, scheme, axe: v.id, impact: v.impact, help: v.help, nodes: v.nodes.slice(0, 3).map((n) => n.target.join(" ")) });
        }
        for (let i = 0; i < 12; i++) {
          await page.keyboard.press("Tab");
          const active = await page.evaluate(() => {
            const a = document.activeElement;
            return { inside: Boolean(a?.closest?.('[role="dialog"]')), tag: a?.tagName?.toLowerCase(), text: (a?.textContent ?? "").trim().slice(0, 80) };
          });
          if (!active.inside) {
            problems.push({ page: `${path} (modal)`, vp, scheme, focus: "Tab left the modal", ...active });
            break;
          }
        }
        await page.keyboard.press("Escape");
        const restored = await page.evaluate(() => {
          const a = document.activeElement;
          return { tag: a?.tagName?.toLowerCase(), text: (a?.textContent ?? "").trim().slice(0, 80) };
        });
        if (!/Preview what your broker sees/.test(restored.text)) {
          problems.push({ page: `${path} (modal)`, vp, scheme, focus: "Escape did not return focus to the opener", ...restored });
        }
      }
    }
    await ctx.close();
  }
}

await browser.close();
await writeFile(`${out}/report.json`, JSON.stringify(problems, null, 2));
console.log(`Screenshots and report.json in ${out}/ (${[...paths, reflowPath].join(", ")} x light/dark x desktop/mobile/narrow)`);
if (problems.length) {
  console.log(`${problems.length} problem(s):`);
  for (const p of problems) console.log(" ", JSON.stringify(p));
  process.exit(1);
}
console.log("No axe violations, console errors or horizontal scroll.");
