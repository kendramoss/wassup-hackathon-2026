// Small-interaction audit: keyboard focus hidden under sticky bars
// (WCAG 2.4.11), sticky bar height on a phone, and whether menus return
// focus to their button when closed with Esc.
//
//   node scripts/interaction-check.mjs [http://localhost:8100] [case_id]
import { chromium } from "playwright-core";
const base = (process.argv[2] ?? "http://localhost:8100").replace(/\/$/, "");
const C = process.argv[3] ?? "case_354b8919d82e";
const pages = ["/", "/cases/new", `/cases/${C}/origin`, `/cases/${C}/scenarios`, `/cases/${C}/plan`, `/cases/${C}/draft`, "/accuracy"];
const b = await chromium.launch({ channel: process.env.BROWSER ?? "chrome" });
const issues = [];
for (const [vp, viewport] of Object.entries({ desktop: { width: 1366, height: 768 }, mobile: { width: 390, height: 844 } })) {
  const p = await (await b.newContext({ viewport })).newPage();
  for (const path of pages) {
    await p.goto(base + path, { waitUntil: "networkidle" });
    // Sticky chrome height after scrolling a little.
    await p.mouse.wheel(0, 400);
    await p.waitForTimeout(150);
    const sticky = await p.evaluate(() => {
      let h = 0;
      for (const el of document.querySelectorAll("body *")) {
        const cs = getComputedStyle(el);
        if (cs.position !== "sticky" && cs.position !== "fixed") continue;
        const r = el.getBoundingClientRect();
        if (r.top <= 1 && r.height > 0 && r.width > innerWidth / 2) h = Math.max(h, r.bottom);
      }
      return Math.round(h);
    });
    if (sticky / viewport.height > 0.15) issues.push(`${vp} ${path}: sticky bars take ${sticky}px (${Math.round((100 * sticky) / viewport.height)}% of the screen)`);
    // Tab through the page, shift-tab back, and check what's hidden.
    await p.evaluate(() => window.scrollTo(0, document.documentElement.scrollHeight));
    await p.waitForTimeout(100);
    for (let i = 0; i < 60; i++) {
      await p.keyboard.press("Shift+Tab");
      const hidden = await p.evaluate(() => {
        const el = document.activeElement;
        if (!el || el === document.body) return null;
        const r = el.getBoundingClientRect();
        for (const bar of document.querySelectorAll("body *")) {
          const cs = getComputedStyle(bar);
          if ((cs.position !== "sticky" && cs.position !== "fixed") || bar.contains(el)) continue;
          const s = bar.getBoundingClientRect();
          if (s.height === 0 || s.top > 1) continue;
          if (r.top < s.bottom - 2 && r.bottom > s.top) return `${el.tagName.toLowerCase()} "${(el.getAttribute("aria-label") || el.textContent || "").trim().slice(0, 40)}" under the sticky bar`;
        }
        return null;
      });
      if (hidden) issues.push(`${vp} ${path}: focus ${hidden}`);
    }
  }
  // Menus: Esc closes and focus returns to the button.
  for (const [path, btn] of [["/", 'button[aria-label^="Actions for"]'], [`/cases/${C}/origin`, "nav[aria-label=Case] button[aria-expanded]"]]) {
    await p.goto(base + path, { waitUntil: "networkidle" });
    const t = p.locator(btn).last();
    if (!(await t.count())) continue;
    await t.focus();
    await p.keyboard.press("Enter");
    await p.waitForTimeout(150);
    await p.keyboard.press("Tab");
    await p.keyboard.press("Escape");
    await p.waitForTimeout(150);
    const back = await t.evaluate((el) => el === document.activeElement);
    if (!back) issues.push(`${vp} ${path}: Esc closes the menu but focus doesn't return to its button`);
  }
}
await b.close();
for (const i of [...new Set(issues)]) console.log(i);
console.log(issues.length ? `${new Set(issues).size} issue(s)` : "No interaction issues.");
