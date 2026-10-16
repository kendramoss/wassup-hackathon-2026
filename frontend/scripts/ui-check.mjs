// Screenshots every page in light and dark, desktop and mobile, and runs axe
// on each. Use it after any UI change and look at the pictures.
//
//   npm run ui-check                         # against http://localhost:3000
//   npm run ui-check -- https://<url> /accuracy  # another site or other paths
//
// Writes PNGs and report.json to .ui-check/ and exits 1 if axe finds
// violations or a page logs console errors. Uses an installed Chrome or Edge
// (no browser download); set BROWSER=msedge or BROWSER=chrome to pick one.
import { mkdir, writeFile } from "node:fs/promises";
import { chromium } from "playwright-core";
import { AxeBuilder } from "@axe-core/playwright";

const args = process.argv.slice(2);
const base = (args.find((a) => a.startsWith("http")) ?? "http://localhost:3000").replace(/\/$/, "");
const paths = args.filter((a) => a.startsWith("/"));
if (paths.length === 0) paths.push("/", "/cases", "/accuracy");
const out = ".ui-check";

const viewports = { desktop: { width: 1280, height: 860 }, mobile: { width: 390, height: 844 } };

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

    for (const path of paths) {
      current = path;
      await page.goto(base + path, { waitUntil: "networkidle" });
      // Hide the Next.js dev-mode badge so it doesn't show up in screenshots.
      await page.addStyleTag({ content: "nextjs-portal { display: none !important; }" });
      const name = path === "/" ? "home" : path.slice(1).replace(/\//g, "-");
      await page.screenshot({ path: `${out}/${name}-${vp}-${scheme}.png`, fullPage: true });

      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
      if (overflow) problems.push({ page: path, vp, scheme, layout: "page scrolls horizontally" });

      const { violations } = await new AxeBuilder({ page }).analyze();
      for (const v of violations) {
        problems.push({ page: path, vp, scheme, axe: v.id, impact: v.impact, help: v.help, nodes: v.nodes.slice(0, 3).map((n) => n.target.join(" ")) });
      }
    }
    await ctx.close();
  }
}

await browser.close();
await writeFile(`${out}/report.json`, JSON.stringify(problems, null, 2));
console.log(`Screenshots and report.json in ${out}/ (${paths.join(", ")} x light/dark x desktop/mobile)`);
if (problems.length) {
  console.log(`${problems.length} problem(s):`);
  for (const p of problems) console.log(" ", JSON.stringify(p));
  process.exit(1);
}
console.log("No axe violations, console errors or horizontal scroll.");
