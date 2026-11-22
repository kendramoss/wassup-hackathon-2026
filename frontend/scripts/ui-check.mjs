// Screenshots every page in light and dark, desktop, mobile and narrow, runs
// axe on each, and checks dialog/tooltip keyboard states. Use it after any UI
// change and look at the pictures.
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
// #522, #534 and #537: use the recorded pinned case and local stubs so these
// states do not depend on a live backend (or spend a prediction) and are
// checked on every run.
const reflowId = "case_878d76540b91";
const reflowPath = `/cases/${reflowId}/scenarios`;
const draftPath = `/cases/${reflowId}/draft`;
const newCasePath = "/cases/new";
const paths = args.filter((a) => a.startsWith("/"));
if (paths.length === 0) {
  paths.push("/", "/cases", "/accuracy", "/triage", newCasePath, draftPath, "/examples/rule-check");
  // #596: the judge-tour entry to that example has a query string; checking
  // it by default also covers the file-safe screenshot slug on Windows.
  paths.push("/examples/rule-check?tour=1&from=case_878d76540b91");
}
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
    // /cases/new probes HTS_SUGGEST with an empty POST; a real 404 is
    // intentional and would log a console error. Answer it locally instead.
    await page.route("**/api/v1/products/hts-suggest", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: "{}" }),
    );
    const checkPaths = vp === "narrow" ? [...new Set([...paths, reflowPath])] : paths;

    for (const path of checkPaths) {
      current = path;
      await page.goto(base + path, { waitUntil: "networkidle" });
      // Hide the Next.js dev-mode badge so it doesn't show up in screenshots.
      await page.addStyleTag({ content: "nextjs-portal { display: none !important; }" });
      // #596: query-string paths contain ?/&/=, which are invalid in Windows
      // filenames. Keep a readable slug made only of file-safe characters.
      const name = (path === "/" ? "home" : path.slice(1)).replace(/[^A-Za-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "") || "page";
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

      // #537: the HTS help tooltip opens on focus and contains a link. Tab
      // must reach that link, and Escape must close the tip and restore focus.
      if (path === newCasePath) {
        await page.getByRole("button", { name: "How to find your HTS subheading" }).focus();
        await page.locator('[role="tooltip"]').waitFor();
        await page.keyboard.press("Tab");
        const reached = await page.evaluate(() => {
          const a = document.activeElement;
          return {
            tag: a?.tagName?.toLowerCase(),
            href: a?.getAttribute("href"),
            text: (a?.textContent ?? "").trim().slice(0, 80),
            tooltip: Boolean(document.querySelector('[role="tooltip"]')),
          };
        });
        if (reached.href !== "https://hts.usitc.gov" || !reached.tooltip) {
          problems.push({ page: `${path} (hts help)`, vp, scheme, focus: "Tab did not reach the HTS help link", ...reached });
        }
        await page.addStyleTag({ content: "nextjs-portal { display: none !important; }" });
        await page.screenshot({ path: `${out}/new-case-hts-help-${vp}-${scheme}.png`, fullPage: true });
        const { violations: tipViolations } = await new AxeBuilder({ page }).analyze();
        for (const v of tipViolations) {
          problems.push({ page: `${path} (hts help)`, vp, scheme, axe: v.id, impact: v.impact, help: v.help, nodes: v.nodes.slice(0, 3).map((n) => n.target.join(" ")) });
        }
        await page.keyboard.press("Escape");
        await page.locator('[role="tooltip"]').waitFor({ state: "detached" });
        const restored = await page.evaluate(() => {
          const a = document.activeElement;
          return {
            tag: a?.tagName?.toLowerCase(),
            label: a?.getAttribute("aria-label"),
            tooltip: Boolean(document.querySelector('[role="tooltip"]')),
          };
        });
        if (!/How to find your HTS subheading/.test(restored.label ?? "") || restored.tooltip) {
          problems.push({ page: `${path} (hts help)`, vp, scheme, focus: "Escape did not close the tip and restore focus", ...restored });
        }

        // #546: on a fresh case the HTS suggest button has no source. Its
        // reason must be focusable and exposed as a description, not hidden
        // in a title on a natively disabled button (the #100 pattern).
        const suggest = page.getByRole("button", { name: "Suggest from earlier rulings" });
        if (await suggest.count()) {
          await suggest.focus();
          const state = await page.evaluate(() => {
            const b = document.activeElement;
            const described = b?.getAttribute("aria-describedby");
            const reason = described ? document.getElementById(described) : null;
            const box = reason?.getBoundingClientRect();
            return {
              disabled: b?.disabled,
              ariaDisabled: b?.getAttribute("aria-disabled"),
              reason: (reason?.textContent ?? "").trim(),
              visible: Boolean(box && box.width > 0 && box.height > 0),
            };
          });
          if (state.disabled !== false || state.ariaDisabled !== "true" || !state.reason || !state.visible) {
            problems.push({ page: `${path} (hts suggest)`, vp, scheme, focus: "HTS suggest reason is not keyboard reachable", ...state });
          }
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

      // #539: a keyboard-opened popup must close when Tab carries focus out
      // of the whole widget, not only on Escape or an outside mouse click.
      // Both the case switcher (top bar of a case page) and the ⋯ row menu on
      // /cases were observed staying open while focus moved on down the page.
      const popups = [
        { path: draftPath, selector: 'button[aria-label$="Switch case"]', name: "case switcher", max: 20 },
        { path: "/cases", selector: 'button[aria-label^="Actions for"]', name: "row menu", max: 10 },
      ];
      for (const popup of popups) {
        if (path !== popup.path) continue;
        const trigger = page.locator(popup.selector).first();
        if (!(await trigger.count())) continue;
        await trigger.focus();
        await page.keyboard.press("Enter");
        if (vp === "desktop" && scheme === "light") {
          await page.screenshot({ path: `${out}/${popup.name.replace(/\s+/g, "-")}-open-${vp}-${scheme}.png`, fullPage: true });
        }
        let left = false;
        for (let i = 0; i < popup.max; i++) {
          await page.keyboard.press("Tab");
          left = await page.evaluate((sel) => {
            const b = document.querySelector(sel);
            return Boolean(b && !b.parentElement?.contains(document.activeElement));
          }, popup.selector);
          if (left) break;
        }
        const expanded = await trigger.getAttribute("aria-expanded");
        const stillOpen = await page.locator('div[role="menu"]').count();
        if (!left || expanded !== "false" || stillOpen) {
          problems.push({ page: `${path} (${popup.name})`, vp, scheme, focus: "popup stayed open after Tab left it", left, expanded, stillOpen });
        }
      }
    }
    await ctx.close();
  }
}

// #542: the signed-in account menu is the third popup that must dismiss when
// Tab leaves it. It needs a session, so check it on its own signed-in pages.
for (const scheme of ["light", "dark"]) {
  for (const [vp, viewport] of Object.entries(viewports)) {
    const ctx = await browser.newContext({ viewport, colorScheme: scheme });
    const page = await ctx.newPage();
    page.on("console", (m) => m.type() === "error" && problems.push({ page: "/cases (signed in)", vp, scheme, console: m.text() }));
    page.on("pageerror", (e) => problems.push({ page: "/cases (signed in)", vp, scheme, console: String(e) }));
    await page.route("**/api/session", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ client_id: "ui-check", email: "ui-check@example.com", name: "UI Check", picture: null, exp: null }) }),
    );
    // A signed-in identity makes use-role fetch the workspace list. Answer it
    // locally so the local backend's 404 is not a console error in this state.
    await page.route("**/api/v1/workspaces/mine", (route) =>
      route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ email: "ui-check@example.com", workspace: null, invites: [] }) }),
    );
    await page.goto(base + "/cases", { waitUntil: "networkidle" });
    await page.addStyleTag({ content: "nextjs-portal { display: none !important; }" });
    const trigger = page.locator('button[aria-label^="Account:"]');
    if (!(await trigger.count())) {
      problems.push({ page: "/cases (signed in)", vp, scheme, focus: "account menu trigger missing" });
    } else {
      await trigger.first().click();
      await page.locator('div[role="menu"]').waitFor();
      if (vp === "desktop" && scheme === "light") {
        await page.screenshot({ path: `${out}/account-menu-open-${vp}-${scheme}.png`, fullPage: true });
      }
      let left = false;
      for (let i = 0; i < 6; i++) {
        await page.keyboard.press("Tab");
        left = await page.evaluate(() => {
          const b = document.querySelector('button[aria-label^="Account:"]');
          return Boolean(b && !b.parentElement?.contains(document.activeElement));
        });
        if (left) break;
      }
      const menus = await page.locator('div[role="menu"]').count();
      if (!left || menus) problems.push({ page: "/cases (signed in)", vp, scheme, focus: "account menu stayed open after Tab left it", left, menus });
    }
    await ctx.close();
  }
}

// #553: the ⋯ row menu must follow the ARIA menu-button keyboard pattern:
// keyboard open focuses the first item, arrows move between items, Escape
// restores focus to the trigger. Stub one writable case so a row exists.
{
  const ctx = await browser.newContext({ viewport: viewports.desktop });
  const page = await ctx.newPage();
  page.on("console", (m) => m.type() === "error" && problems.push({ page: "/cases (row menu)", console: m.text() }));
  page.on("pageerror", (e) => problems.push({ page: "/cases (row menu)", console: String(e) }));
  const summary = {
    id: "case_ui_check0000001",
    product_name: "Row menu probe",
    display_name: "Row menu probe",
    date: "2026-01-01",
    predicted_origin: "CN",
    triage_route: "auto",
    confidence: 0.9,
    step: "origin",
    updated_at: "2026-01-01T00:00:00Z",
    watch_status: null,
    scenario_count: 0,
    pinned: false,
    can_write: true,
  };
  await page.route("**/api/v1/cases", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ cases: [summary] }) }),
  );
  await page.route("**/api/v1/demos", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ demos: [] }) }),
  );
  await page.goto(base + "/cases", { waitUntil: "networkidle" });
  const trigger = page.locator('button[aria-label^="Actions for"]').first();
  if (!(await trigger.count())) {
    problems.push({ page: "/cases (row menu)", focus: "row menu trigger missing" });
  } else {
    await trigger.focus();
    await page.keyboard.press("Enter");
    const opened = await page.evaluate(() => ({
      label: document.querySelector('div[role="menu"]')?.getAttribute("aria-label") ?? null,
      text: document.activeElement?.textContent?.trim(),
      role: document.activeElement?.getAttribute("role"),
    }));
    if (opened.text !== "Open" || opened.role !== "menuitem" || !opened.label)
      problems.push({ page: "/cases (row menu)", focus: "keyboard open did not focus first menuitem", ...opened });
    await page.keyboard.press("ArrowDown");
    const arrowed = await page.evaluate(() => document.activeElement?.textContent?.trim());
    if (arrowed !== "Rename") problems.push({ page: "/cases (row menu)", focus: "ArrowDown did not move to next menuitem", arrowed });
    await page.keyboard.press("End");
    const ended = await page.evaluate(() => document.activeElement?.textContent?.trim());
    if (ended !== "Delete case…") problems.push({ page: "/cases (row menu)", focus: "End did not move to last menuitem", ended });
    await page.keyboard.press("Escape");
    const restored = await page.evaluate(() => document.activeElement?.getAttribute("aria-label"));
    if (!/^Actions for/.test(restored ?? "")) problems.push({ page: "/cases (row menu)", focus: "Escape did not restore focus to trigger", restored });
    // #605: choosing Delete from the keyboard must leave focus on the safe
    // action in the replacement confirmation row, not on <body>.
    await trigger.focus();
    await page.keyboard.press("Enter");
    await page.keyboard.press("End");
    await page.keyboard.press("Enter");
    const confirmFocus = await page.evaluate(() => {
      const el = document.activeElement;
      return {
        tag: el?.tagName ?? null,
        focused: el === document.body ? "BODY" : (el?.textContent ?? "").trim().slice(0, 40),
        hasCancel: [...document.querySelectorAll("button")].some((b) => b.textContent?.trim() === "Cancel"),
      };
    });
    await page.keyboard.press("Escape");
    const afterEscape = await page.evaluate(() => ({
      label: document.activeElement?.getAttribute("aria-label") ?? null,
      stillConfirming: (document.body.textContent ?? "").includes("Delete “Row menu probe”?"),
    }));
    if (
      confirmFocus.focused !== "Cancel" ||
      !confirmFocus.hasCancel ||
      afterEscape.label !== "Actions for Row menu probe" ||
      afterEscape.stillConfirming
    ) {
      problems.push({ page: "/cases (row menu)", focus: "Delete confirmation did not take focus or cancel on Escape", confirmFocus, afterEscape });
    }
  }
  await ctx.close();
}

// #557: the money drawer's cost tabs must follow the ARIA tabs pattern (one
// Tab stop, arrows move between tabs, named tabpanel). Stub an extract result
// with plants/groups so the drawer's cost section renders.
{
  const ctx = await browser.newContext({ viewport: viewports.desktop });
  const page = await ctx.newPage();
  page.on("console", (m) => m.type() === "error" && problems.push({ page: "/cases/new (cost tabs)", console: m.text() }));
  page.on("pageerror", (e) => problems.push({ page: "/cases/new (cost tabs)", console: String(e) }));
  const product = {
    name: "Probe gun",
    plants: [
      { id: "p1", name: "CN plant", country: "CN" },
      { id: "p2", name: "TH plant", country: "TH" },
    ],
    components: [
      { name: "Fan", made_in: "CN", plant: "p1", group: "motor_fan_assembly" },
      { name: "Board", made_in: "TH", plant: "p2", group: "pcba" },
    ],
  };
  await page.route("**/api/v1/products/extract", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ product, warnings: [] }) }),
  );
  await page.route("**/api/v1/duty-rates", (route) => route.fulfill({ status: 200, contentType: "application/json", body: "{}" }));
  await page.route("**/api/v1/products/hts-suggest", (route) => route.fulfill({ status: 200, contentType: "application/json", body: "{}" }));
  // #573: the per-part HTS feature check runs once per page load. Stub it too,
  // or its intentional local 404 becomes a console error and fails ui-check.
  await page.route("**/api/v1/products/parts-hts-suggest", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ parts: [], good: null, max_parts: 20 }) }),
  );
  await page.goto(base + newCasePath, { waitUntil: "networkidle" });
  // #589: a closed combobox has no listbox in the DOM, so it must not point
  // aria-controls at a missing id. Check every collapsed field on this page.
  const danglingControls = await page.evaluate(() =>
    [...document.querySelectorAll('input[role="combobox"][aria-expanded="false"][aria-controls]')]
      .map((el) => {
        const controls = el.getAttribute("aria-controls");
        return { label: el.getAttribute("aria-label") ?? el.id, controls, exists: !!document.getElementById(controls) };
      })
      .filter((x) => !x.exists),
  );
  if (danglingControls.length) {
    problems.push({ page: "/cases/new (combobox)", focus: "closed combobox points at a missing listbox", fields: danglingControls });
  }
  // The relationship must appear once the popup is really in the DOM.
  await page.locator('input[role="combobox"]').first().focus();
  await page.waitForFunction(() => {
    const el = document.activeElement;
    const id = el?.getAttribute("aria-controls");
    return !!id && document.getElementById(id)?.getAttribute("role") === "listbox";
  });
  const openControls = await page.evaluate(() => {
    const el = document.activeElement;
    const id = el?.getAttribute("aria-controls");
    return { expanded: el?.getAttribute("aria-expanded"), listbox: document.getElementById(id)?.getAttribute("role") ?? null };
  });
  if (openControls.expanded !== "true" || openControls.listbox !== "listbox") {
    problems.push({ page: "/cases/new (combobox)", focus: "open combobox does not reference its listbox", openControls });
  }
  await page.keyboard.press("Escape");
  await page.getByRole("tab", { name: "Paste a description" }).click();
  await page.locator("textarea").first().fill("Each heat gun is assembled in Thailand using components sourced from China and Thailand, with a fan and a board and a housing.");
  await page.getByRole("button", { name: "Turn into parts and plants" }).click();
  await page.getByRole("button", { name: /money inputs/i }).first().click();
  const tabs = page.locator('[role="tablist"][aria-label="How to enter costs"] [role="tab"]');
  if ((await tabs.count()) !== 2) {
    problems.push({ page: "/cases/new (cost tabs)", focus: "cost tablist did not render", count: await tabs.count() });
  } else {
    const before = await tabs.evaluateAll((es) => es.map((e) => ({ text: e.textContent?.trim(), ti: e.getAttribute("tabindex"), controls: e.getAttribute("aria-controls") })));
    const tabStops = await page.evaluate(() => [...document.querySelectorAll('[role="tab"]')].filter((e) => e.getAttribute("tabindex") !== "-1").length);
    if (before[0]?.ti !== "0" || before[1]?.ti !== "-1" || tabStops !== 2 || !before[0]?.controls)
      problems.push({ page: "/cases/new (cost tabs)", focus: "cost tabs are not a single Tab stop", before, tabStops });
    await tabs.first().focus();
    await page.keyboard.press("ArrowRight");
    const after = await page.evaluate((controls) => {
      const selected = document.querySelector('[role="tablist"][aria-label="How to enter costs"] [aria-selected="true"]');
      const panel = controls ? document.getElementById(controls) : null;
      return {
        text: document.activeElement?.textContent?.trim(),
        selected: selected?.textContent?.trim(),
        panelRole: panel?.getAttribute("role"),
        panelLabelledBy: panel?.getAttribute("aria-labelledby"),
        selectedId: selected?.id,
      };
    }, before[0]?.controls);
    if (after.text !== "Each group" || after.selected !== "Each group" || after.panelRole !== "tabpanel" || !after.panelLabelledBy || after.panelLabelledBy !== after.selectedId)
      problems.push({ page: "/cases/new (cost tabs)", focus: "ArrowRight did not move and select the next tab", after });
  }
  await ctx.close();
}

// #607: an invalid cost-basis select must point at its error paragraph, like
// the neighbouring number and text fields do.
{
  const ctx = await browser.newContext({ viewport: viewports.desktop });
  const page = await ctx.newPage();
  page.on("console", (m) => m.type() === "error" && !/400 \(Bad Request\)/.test(m.text()) && problems.push({ page: "/cases/new (cost basis)", console: m.text() }));
  page.on("pageerror", (e) => problems.push({ page: "/cases/new (cost basis)", console: String(e) }));
  const product = {
    name: "Probe gun",
    plants: [
      { id: "p1", name: "CN plant", country: "CN" },
      { id: "p2", name: "TH plant", country: "TH" },
    ],
    components: [
      { name: "Fan", made_in: "CN", plant: "p1", group: "motor_fan_assembly" },
      { name: "Board", made_in: "TH", plant: "p2", group: "pcba" },
    ],
  };
  await page.route("**/api/v1/products/extract", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ product, warnings: [] }) }),
  );
  await page.route("**/api/v1/duty-rates", (route) => route.fulfill({ status: 200, contentType: "application/json", body: "{}" }));
  await page.route("**/api/v1/products/hts-suggest", (route) => route.fulfill({ status: 200, contentType: "application/json", body: "{}" }));
  await page.route("**/api/v1/products/parts-hts-suggest", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ parts: [], good: null, max_parts: 20 }) }),
  );
  await page.route("**/api/v1/cases/predict", (route) =>
    route.fulfill({
      status: 400,
      contentType: "application/json",
      body: JSON.stringify({
        code: "invalid_input",
        message: "economics: operations.groups: operations.groups.motor_fan_assembly.cost_basis: Choose whether labour and logistics are inside or on top of the group cost.",
        retryable: false,
      }),
    }),
  );
  await page.goto(base + newCasePath, { waitUntil: "networkidle" });
  await page.getByRole("tab", { name: "Paste a description" }).click();
  await page.locator("textarea").first().fill("Each heat gun is assembled in Thailand using components sourced from China and Thailand, with a fan and a board and a housing.");
  await page.getByRole("button", { name: "Turn into parts and plants" }).click();
  await page.getByRole("button", { name: /Find the likely origin/i }).first().click();
  await page.getByRole("alert").filter({ hasText: "Choose whether" }).waitFor({ timeout: 8000 });
  await page.getByRole("button", { name: /money inputs/i }).first().click();
  await page.getByRole("dialog", { name: "Money inputs" }).waitFor();
  await page.getByRole("switch", { name: "Include operating costs when ranking moves" }).focus();
  await page.keyboard.press("Space");
  const basis = await page.evaluate(() => {
    const select = [...document.querySelectorAll('select[aria-invalid="true"]')].find((el) => el.id.endsWith("-basis"));
    const error = select ? document.getElementById(`${select.id}-e`) : null;
    return {
      describedby: select?.getAttribute("aria-describedby") ?? null,
      errorId: error?.id ?? null,
      errorText: (error?.textContent ?? "").trim(),
    };
  });
  if (!basis.errorText || !basis.describedby?.split(/\s+/).includes(basis.errorId)) {
    problems.push({ page: "/cases/new (cost basis)", focus: "invalid select does not describe its error", basis });
  }
  await ctx.close();
}

// #548: the elapsed counter in a pending live region must not be announced
// once a second. Enter both states with a delayed stub, let the counter tick,
// and fail if the changing text is exposed to assistive tech.
const delayed = async (route, body) => {
  await new Promise((r) => setTimeout(r, 8000));
  await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(body) });
};
{
  const ctx = await browser.newContext({ viewport: viewports.desktop });
  const page = await ctx.newPage();
  page.on("console", (m) => m.type() === "error" && problems.push({ page: "/triage (running)", console: m.text() }));
  page.on("pageerror", (e) => problems.push({ page: "/triage (running)", console: String(e) }));
  await page.route("**/api/v1/cases/batch", (route) =>
    delayed(route, {
      summary: { total: 1, failed: 0, auto: 0, needs_info: 0, needs_review: 1, out_of_scope: 0 },
      rows: [],
    }),
  );
  await page.goto(base + "/triage", { waitUntil: "networkidle" });
  await page.getByRole("button", { name: /Try the sample/i }).click();
  await page.getByRole("button", { name: /Triage 5 products/i }).click();
  const region = page.locator('section[aria-live="polite"]');
  await region.waitFor();
  await page.waitForFunction(
    () => [...document.querySelectorAll('section[aria-live="polite"] span')].some((s) => /^[1-9]\d* s$/.test(s.textContent.trim())),
    null,
    { timeout: 5000 },
  );
  const tick = await region.evaluate((el) => {
    const t = [...el.querySelectorAll("span")].find((s) => /^\d+ s$/.test(s.textContent.trim()));
    return { text: t?.textContent.trim() ?? null, hidden: Boolean(t?.closest('[aria-hidden="true"]')) };
  });
  if (!tick.text || !tick.hidden) problems.push({ page: "/triage (running)", focus: "ticking elapsed is exposed in a live region", ...tick });
  await ctx.close();
}
{
  const ctx = await browser.newContext({ viewport: viewports.desktop });
  const page = await ctx.newPage();
  page.on("console", (m) => m.type() === "error" && problems.push({ page: "ask (pending)", console: m.text() }));
  page.on("pageerror", (e) => problems.push({ page: "ask (pending)", console: String(e) }));
  await page.route(`**/api/v1/cases/${reflowId}`, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(reflowFixture) }),
  );
  await page.route(`**/api/v1/cases/${reflowId}/ask`, (route) => delayed(route, { reply: { answer: "ok" }, chat: [] }));
  // The case shell estimates duty on mount; the local backend has no such
  // endpoint for a pinned demo, so answer it to keep this state console-clean.
  await page.route(`**/api/v1/cases/${reflowId}/plan/estimate`, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: "{}" }),
  );
  await page.goto(base + `/cases/${reflowId}`, { waitUntil: "networkidle" });
  await page.getByRole("button", { name: /Ask about this case/i }).first().click();
  await page.getByRole("textbox", { name: "Your question" }).fill("Why China?");
  await page.getByRole("button", { name: "Ask", exact: true }).click();
  await page.waitForFunction(
    () => {
      const s = [...document.querySelectorAll('[role="status"]')].find((x) => x.textContent.includes("Reading the case"));
      return [...(s?.querySelectorAll("span") ?? [])].some((x) => /^[1-9]\d* s$/.test(x.textContent.trim()));
    },
    null,
    { timeout: 5000 },
  );
  const tick = await page.evaluate(() => {
    const s = [...document.querySelectorAll('[role="status"]')].find((x) => x.textContent.includes("Reading the case"));
    const t = [...s.querySelectorAll("span")].find((x) => /^\d+ s$/.test(x.textContent.trim()));
    return { text: t?.textContent.trim() ?? null, hidden: Boolean(t?.closest('[aria-hidden="true"]')) };
  });
  if (!tick.text || !tick.hidden) problems.push({ page: "ask (pending)", focus: "ticking elapsed is exposed in a live region", ...tick });
  // #568: the waiting message must be announced once, not once by the sr-only
  // answer region and again by the visible role=status.
  const waiting = await page.evaluate(
    () =>
      [...document.querySelectorAll('[role="status"],[aria-live]')].filter((el) =>
        (el.textContent || "").includes("Reading the case and earlier rulings"),
      ).length,
  );
  if (waiting !== 1) problems.push({ page: "ask (pending)", focus: "waiting message is announced by more than one live region", waiting });
  await ctx.close();
}

// #564: a 429 rate limit ticks a countdown inside role=status. The seconds
// must be hidden from assistive tech, with a stable phrase in their place,
// or the status is announced again on every tick (#548 pattern).
{
  const ctx = await browser.newContext({ viewport: viewports.desktop });
  const page = await ctx.newPage();
  page.on("console", (m) => {
    // Chrome logs the deliberate 429 as a console error; that is the state.
    if (m.type() === "error" && !m.text().includes("429")) problems.push({ page: "/triage (rate limited)", console: m.text() });
  });
  page.on("pageerror", (e) => problems.push({ page: "/triage (rate limited)", console: String(e) }));
  await page.route("**/api/v1/cases/batch", (route) =>
    route.fulfill({
      status: 429,
      contentType: "application/json",
      headers: { "Retry-After": "6" },
      body: JSON.stringify({ code: "rate_limited", message: "Too many requests." }),
    }),
  );
  await page.goto(base + "/triage", { waitUntil: "networkidle" });
  await page.getByRole("button", { name: /Try the sample/i }).click();
  await page.getByRole("button", { name: /Triage 5 products/i }).click();
  const note = page.locator('div[role="status"]').filter({ hasText: "Too many requests" });
  await note.waitFor();
  await page.waitForFunction(
    () => [...document.querySelectorAll('div[role="status"] span')].some((s) => /^[1-9]\d* s$/.test(s.textContent.trim())),
    null,
    { timeout: 5000 },
  );
  const tick = await note.evaluate((el) => {
    const t = [...el.querySelectorAll("span")].find((s) => /^\d+ s$/.test(s.textContent.trim()));
    return { text: t?.textContent.trim() ?? null, hidden: Boolean(t?.closest('[aria-hidden="true"]')), stable: el.textContent.includes("a moment") };
  });
  if (!tick.text || !tick.hidden || !tick.stable) problems.push({ page: "/triage (rate limited)", focus: "ticking countdown is exposed in the status live region", ...tick });
  await ctx.close();
}

// #571: the streamed move table is a visual scratchpad, not a live region.
// Re-announcing the whole tbody on each cell change buries the one new fact;
// a small status must announce just the completed move.
{
  const ctx = await browser.newContext({ viewport: viewports.narrow });
  const page = await ctx.newPage();
  page.on("console", (m) => m.type() === "error" && problems.push({ page: "/cases/[id]/plan (live table)", console: m.text() }));
  page.on("pageerror", (e) => problems.push({ page: "/cases/[id]/plan (live table)", console: String(e) }));
  await page.addInitScript((caseId) => {
    localStorage.setItem(`op:demo:${caseId}`, "heatgun-2022");
    const real = window.fetch.bind(window);
    window.fetch = (input, init) => {
      const url = typeof input === "string" ? input : input.url;
      if (!url.includes(`/api/v1/cases/${caseId}/plan`)) return real(input, init);
      const enc = new TextEncoder();
      let n = 0;
      const events = [
        { event: "start", data: { moves: [
          { key: "m1", description: "Move fan to Thailand" },
          { key: "m2", description: "Move motor to Thailand" },
          { key: "m3", description: "Move board to Thailand" },
        ], total: 3 } },
        { event: "baseline", data: { plan: { description: "No change: keep the current setup", net_per_year: 0 } } },
        { event: "move_started", data: { key: "m1", done: 1, total: 3 } },
        { event: "move_done", data: { key: "m1", done: 1, total: 3, plan: { description: "Move fan to Thailand", net_per_year: 5000, verdict: { confidence: 0.94 } } } },
        { event: "move_started", data: { key: "m2", done: 2, total: 3 } },
        { event: "move_done", data: { key: "m2", done: 2, total: 3, plan: { description: "Move motor to Thailand", net_per_year: 8000, verdict: { confidence: 0.93 } } } },
        { event: "move_started", data: { key: "m3", done: 3, total: 3 } },
        { event: "move_done", data: { key: "m3", done: 3, total: 3, plan: { description: "Move board to Thailand", net_per_year: 3000, verdict: { confidence: 0.95 } } } },
      ];
      const stream = new ReadableStream({
        start(c) {
          const t = setInterval(() => {
            if (n < events.length) {
              const e = events[n++];
              c.enqueue(enc.encode(`event: ${e.event}\ndata: ${JSON.stringify(e.data)}\n\n`));
            }
          }, 120);
          window.__stopPlanSse = () => { clearInterval(t); c.close(); };
        },
      });
      return Promise.resolve(new Response(stream, { status: 200, headers: { "content-type": "text/event-stream" } }));
    };
  }, reflowId);
  await page.route("**/api/session", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ client_id: "ui-check", email: "ui-check@example.com", name: "UI Check", picture: null, exp: null }) }),
  );
  await page.route("**/api/v1/workspaces/mine", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ email: "ui-check@example.com", workspace: null, invites: [] }) }),
  );
  await page.route("**/api/v1/demos/heatgun-2022", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(reflowFixture) }),
  );
  await page.route("**/api/v1/demos", (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ demos: [] }) }),
  );
  await page.route(`**/api/v1/cases/${reflowId}`, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ ...reflowFixture, plan: null, targets: null }) }),
  );
  await page.route(`**/api/v1/cases/${reflowId}/plan/estimate`, (route) =>
    route.fulfill({ status: 200, contentType: "application/json", body: "{}" }),
  );
  await page.goto(base + `/cases/${reflowId}/plan?target=Thailand`, { waitUntil: "networkidle" });
  await page.getByRole("button", { name: /Search moves to/i }).first().click();
  await page.locator("tbody").first().waitFor();
  // #600: at 320 px the live table is 640 px wide. Its scroller must be a
  // named, focusable region or a keyboard user cannot reach the right columns.
  const scroller = await page.evaluate(() => {
    const wrap = [...document.querySelectorAll("div.overflow-x-auto")].find((d) => d.querySelector("table"));
    return wrap
      ? { role: wrap.getAttribute("role"), label: wrap.getAttribute("aria-label"), tabIndex: wrap.tabIndex, scrollWidth: wrap.scrollWidth, clientWidth: wrap.clientWidth }
      : null;
  });
  if (!scroller || scroller.role !== "region" || !scroller.label || scroller.tabIndex !== 0 || scroller.scrollWidth <= scroller.clientWidth) {
    problems.push({ page: "/cases/[id]/plan (live table)", focus: "horizontal scroller is not keyboard reachable", scroller });
  }
  // Wait for the stream to reach the second move; before the fix this is where
  // the old tbody live region replays every row instead of one short status.
  await page.waitForFunction(() => (document.querySelector("tbody")?.textContent ?? "").includes("$8k"), null, { timeout: 8000 });
  const live = await page.evaluate(() => {
    const tbody = document.querySelector("tbody");
    const update = [...document.querySelectorAll('[role="status"]')].find((el) => /Move 2 of 3 checked/.test(el.textContent ?? ""));
    return {
      tbodyLive: tbody?.getAttribute("aria-live") ?? null,
      updateText: (update?.textContent ?? "").trim().replace(/\s+/g, " "),
      updateCount: update ? 1 : 0,
      tableRows: tbody?.querySelectorAll("tr").length ?? 0,
    };
  });
  if (live.tbodyLive || live.updateCount !== 1 || !/Move 2 of 3 checked: Move motor to Thailand/.test(live.updateText)) {
    problems.push({ page: "/cases/[id]/plan (live table)", focus: "streamed move table re-announces every row instead of one status", ...live });
  }
  // A one-cell change must not put the whole table text inside a live region.
  const replay = await page.evaluate(() => {
    const tbody = document.querySelector("tbody");
    return (tbody?.textContent ?? "").trim().replace(/\s+/g, " ");
  });
  if (/Move fan to Thailand.*Move motor to Thailand/s.test(live.updateText) || live.updateText === replay) {
    problems.push({ page: "/cases/[id]/plan (live table)", focus: "status replays the whole move table", update: live.updateText.slice(0, 200) });
  }
  await ctx.close();
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
