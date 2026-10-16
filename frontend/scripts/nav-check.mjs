// Moves between case tabs with client-side navigation and checks the visible
// action bar always belongs to the page on screen (Next keeps the previous
// page mounted but hidden, which once let its actions take over the bar).
//
//   node scripts/nav-check.mjs [http://localhost:8100] [case_id]
import { chromium } from "playwright-core";
const base = (process.argv[2] ?? "http://localhost:8100").replace(/\/$/, "");
const C = process.argv[3] ?? "case_1b48a0abe7dd";
// What each screen's own action bar must contain.
const expect = { product: /Likely origin/, origin: /Plan a move/, scenarios: /Search plans|Next/, plan: /Draft|Search moves/, draft: /Print the letter/ };
const b = await chromium.launch({ channel: process.env.BROWSER ?? "chrome" });
const p = await (await b.newContext({ viewport: { width: 1366, height: 900 } })).newPage();
await p.goto(`${base}/cases/${C}/origin`, { waitUntil: "networkidle" });
const order = ["plan", "origin", "scenarios", "product", "draft", "plan", "origin"];
let bad = 0;
for (const tab of order) {
  await p.locator(`nav[aria-label=Case] a[href$="/${tab}"]`).first().click();
  await p.waitForURL(new RegExp(`/${tab}$`));
  await p.waitForTimeout(1200);
  const bars = await p.evaluate(() => [...document.querySelectorAll("[role=region][aria-label=Actions]")].filter((r) => r.getClientRects().length > 0).map((r) => r.textContent.replace(/\s+/g, " ")));
  const ok = bars.length === 1 && expect[tab].test(bars[0]);
  if (!ok) bad++;
  console.log(`${ok ? "ok " : "BAD"} ${tab}: ${bars.length} visible bar(s) · ${bars.map((t) => t.slice(0, 60)).join(" | ")}`);
}
await b.close();
process.exit(bad ? 1 : 0);
