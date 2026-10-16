// Finds elements that something else covers: every visible control, link,
// field, heading and label is scrolled into view and its centre hit-tested
// (document.elementFromPoint). If the top element there isn't the element,
// its child or its parent, it's covered. Menus and drawers are opened and
// checked the same way.
//
//   node scripts/overlap-check.mjs [http://localhost:8100] [case_id]
import { chromium } from "playwright-core";

const base = (process.argv[2] ?? "http://localhost:8100").replace(/\/$/, "");
const C = process.argv[3] ?? "case_354b8919d82e";
const pages = ["/", "/cases/new", `/cases/${C}/product`, `/cases/${C}/origin`, `/cases/${C}/scenarios`, `/cases/${C}/plan`, `/cases/${C}/compare`, `/cases/${C}/draft`, "/accuracy"];
const viewports = { desktop: { width: 1366, height: 768 }, mobile: { width: 390, height: 844 } };

// Runs in the page: returns "covered" findings for everything currently laid out.
const probe = () => {
  const sel = "a[href], button, input, textarea, select, summary, h1, h2, h3, label, [role=tab], [role=menuitem], td, th, p";
  const out = [];
  const name = (el) => {
    const t = (el.getAttribute("aria-label") || el.textContent || el.getAttribute("placeholder") || "").trim().replace(/\s+/g, " ");
    return `${el.tagName.toLowerCase()} "${t.slice(0, 50)}"`;
  };
  for (const el of document.querySelectorAll(sel)) {
    if (el.closest("[aria-hidden=true], .sr-only")) continue;
    // Content of a folded <details> isn't on screen (only its summary is).
    const fold = el.closest("details:not([open])");
    if (fold && !el.closest("summary")) continue;
    const cs = getComputedStyle(el);
    if (cs.visibility === "hidden" || cs.display === "none") continue;
    const r = el.getBoundingClientRect();
    if (r.width < 4 || r.height < 4) continue;
    el.scrollIntoView({ block: "center", inline: "nearest" });
    const s = el.getBoundingClientRect();
    const x = s.left + s.width / 2, y = s.top + Math.min(s.height / 2, 10);
    if (x < 0 || y < 0 || x > innerWidth || y > innerHeight) continue;
    const hit = document.elementFromPoint(x, y);
    if (!hit || hit === el || el.contains(hit) || hit.contains(el)) continue;
    // Covered: name the covering element's nearest landmark-ish container.
    const cover = hit.closest("[role=dialog], [role=region], footer, header, nav, [role=menu], aside, section") ?? hit;
    out.push(`${name(el)} covered by <${cover.tagName.toLowerCase()} ${cover.getAttribute("aria-label") ?? cover.className.toString().slice(0, 40)}>`);
  }
  return [...new Set(out)];
};

const b = await chromium.launch({ channel: process.env.BROWSER ?? "chrome" });
let total = 0;
for (const [vp, viewport] of Object.entries(viewports)) {
  const page = await (await b.newContext({ viewport })).newPage();
  for (const path of pages) {
    await page.goto(base + path, { waitUntil: "networkidle" });
    await page.waitForTimeout(300);
    const states = [["page", async () => {}]];
    // Open things that float, one at a time, and check underneath and inside them.
    if (path.startsWith("/cases/") && path !== "/cases/new") {
      states.push(["case menu", async () => page.getByRole("button", { expanded: false }).filter({ hasText: "▾" }).first().click()]);
      states.push(["ask drawer", async () => page.getByRole("button", { name: "Ask about this case" }).first().click()]);
    }
    if (path.endsWith("/origin")) states.push(["history drawer", async () => page.getByRole("button", { name: /^History/ }).first().click()]);
    if (path.endsWith("/plan")) states.push(["money drawer", async () => page.getByRole("button", { name: "Edit" }).first().click()]);
    if (path === "/") states.push(["row menu", async () => page.getByRole("button", { name: /^Actions for/ }).first().click()]);
    for (const [state, open] of states) {
      await page.goto(base + path, { waitUntil: "networkidle" });
      await page.waitForTimeout(250);
      try {
        await open();
        await page.waitForTimeout(250);
      } catch {
        continue; // control not present in this state
      }
      // In an open drawer or menu, check only its contents.
      const scope = await page.evaluate(() => Boolean(document.querySelector("[role=dialog], [role=menu]")));
      const found = await page.evaluate(scope ? () => {
        const root = document.querySelector("[role=dialog], [role=menu]");
        const out = [];
        for (const el of root.querySelectorAll("a[href], button, input, textarea, p, h2, h3, li")) {
          el.scrollIntoView({ block: "nearest" });
          const r = el.getBoundingClientRect();
          if (r.width < 4 || r.height < 4 || r.bottom < 0 || r.top > innerHeight) continue;
          const hit = document.elementFromPoint(r.left + r.width / 2, r.top + Math.min(r.height / 2, 8));
          if (hit && hit !== el && !el.contains(hit) && !hit.contains(el) && root.contains(hit) === false) out.push(`${el.tagName.toLowerCase()} "${(el.textContent ?? "").trim().slice(0, 40)}" covered by ${hit.closest("[role=region],nav,header,footer,section,aside")?.getAttribute("aria-label") ?? hit.tagName}`);
        }
        return [...new Set(out)];
      } : probe);
      for (const f of found) console.log(`${vp} ${path} [${state}] ${f}`);
      total += found.length;
    }
  }
}
await b.close();
console.log(total ? `${total} covered element(s)` : "No covered elements.");
process.exit(total ? 1 : 0);
