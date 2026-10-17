// Scroll-driven motion for the landing page. One animation loop drives the
// three figures; each only does work while it is near the viewport, and
// with prefers-reduced-motion every figure shows its state without tweening.
import { copy } from "@/lib/copy";
import { createHeatGun, type Country } from "./heat-gun";

const L = copy.landing;
const clamp01 = (v: number) => Math.min(1, Math.max(0, v));
const ease = (t: number) => t * t * (3 - 2 * t);
const ramp = (p: number, a: number, b: number) => ease(clamp01((p - a) / (b - a)));
const lerp = (a: number, b: number, t: number) => a + (b - a) * t;
const SVGNS = "http://www.w3.org/2000/svg";
const ruling = (id: string) => {
  const a = document.createElement("a");
  a.href = `https://rulings.cbp.gov/ruling/${encodeURIComponent(id)}`;
  a.target = "_blank";
  a.rel = "noreferrer";
  a.textContent = id;
  return a;
};

export function startLanding(root: HTMLElement, { flag }: { flag: (c: Country) => string }) {
  const $ = <T extends Element = HTMLElement>(k: string) => root.querySelector<T>(`[data-k="${k}"]`)!;
  const $$ = <T extends Element = HTMLElement>(k: string) => [...root.querySelectorAll<T>(`[data-k="${k}"]`)];
  const reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const wide = () => innerWidth > 900;
  // Pinned areas sit under the sticky AppBar, whose height wraps on phones.
  const bar = document.querySelector("header");
  const setBar = () => bar && root.style.setProperty("--bar", `${bar.offsetHeight}px`);
  setBar();
  const barRo = new ResizeObserver(setBar);
  if (bar) barRo.observe(bar);

  // ---- fig.1 ----------------------------------------------------------------
  const scrolly = $("scrolly"), scroller = $("scroller"), gunSheet = $("gunsheet");
  let gun: ReturnType<typeof createHeatGun> | null = null;
  try {
    gun = createHeatGun({ canvas: $<HTMLCanvasElement>("canvas"), labels: $("labels"), leaders: $<SVGSVGElement>("leaders"), flag, reduce });
  } catch {
    gunSheet.classList.add("nogl"); // no WebGL: the story text and stamp still work
  }
  const stamp = $("stamp"), plannote = $("plannote"), caption = $("caption"), tbstate = $("tbstate");
  const steps = $$("step");
  let after: boolean | null = null;
  function setStamp(a: boolean) {
    if (after === a) return;
    after = a;
    const c: Country = a ? "Thailand" : "China";
    const eyebrow = document.createElement("div");
    eyebrow.className = "eyebrow";
    eyebrow.textContent = `${a ? "2026" : "2022"} · CBP held`;
    const held = document.createElement("div");
    held.className = "held";
    const img = document.createElement("img");
    img.className = "flag";
    img.alt = "";
    img.src = flag(c);
    held.append(img, c);
    const rul = document.createElement("div");
    rul.className = "rul";
    rul.append("Ruling ", ruling(a ? "N357771" : "N328954"), ` · ${a ? "29 Jan 2026" : "15 Nov 2022"}`);
    stamp.replaceChildren(eyebrow, held, rul);
    stamp.classList.toggle("after", a);
    if (!reduce) {
      stamp.classList.remove("flip");
      void stamp.offsetWidth;
      stamp.classList.add("flip");
    }
  }
  setStamp(false);
  const fig1Progress = () => {
    const el = wide() ? scrolly : scroller;
    const r = el.getBoundingClientRect();
    return clamp01(-r.top / Math.max(1, r.height - innerHeight));
  };

  // ---- bridge + fig.2 ---------------------------------------------------------
  const bridge = $("bridge"), bridgepath = $<SVGPathElement>("bridgepath");
  const f2 = $("f2"), stage = $("stage"), now = $("now");
  const sheets = $$("mini");
  const pathsvg = $<SVGSVGElement>("pathsvg"), wire = $<SVGPathElement>("wire"), lead = $<SVGPathElement>("lead");
  const wires = [null, ...L.wires].map((t) => {
    const arrow = document.createElementNS(SVGNS, "polygon");
    arrow.setAttribute("fill", "var(--ok)");
    pathsvg.appendChild(arrow);
    let label: SVGTextElement | null = null;
    if (t) {
      label = document.createElementNS(SVGNS, "text");
      label.textContent = t;
      label.setAttribute("text-anchor", "middle");
      pathsvg.appendChild(label);
    }
    return { arrow, label };
  });
  let x0 = 60;
  const fig2Progress = () => {
    const r = f2.getBoundingClientRect();
    return clamp01(-r.top / Math.max(1, r.height - innerHeight));
  };
  function layoutF2(p: number) {
    if (!wide()) {
      sheets.forEach((s) => s.classList.add("on"));
      return;
    }
    const W = stage.clientWidth, H = stage.clientHeight;
    const gap = Math.min(330, (W - 250) / 3);
    // One flat row. The wire draws left to right; each sheet rises into
    // place just before the wire reaches it, then fills in.
    const d = ramp(p, 0.08, 0.86);
    const cx = (i: number) => W / 2 + (i - 1.5) * gap, cy = H / 2;
    x0 = cx(0) - 125 - 70;
    const x1 = cx(3) + 125 + 70;
    const head = lerp(x0, x1, d);
    sheets.forEach((s, i) => {
      const k = reduce ? 1 : ease(clamp01((head - (cx(i) - 125 - gap * 0.55)) / (gap * 0.45)));
      s.style.transform = `translate(${(i - 1.5) * gap}px, ${lerp(36, 0, k)}px)`;
      s.style.opacity = String(lerp(0.25, 1, k));
    });
    wire.setAttribute("d", `M${x0},${cy} H${x1}`);
    lead.setAttribute("d", `M${x0},${-stage.offsetTop - 2} V${cy}`);
    const len = x1 - x0;
    wire.style.strokeDasharray = `${len}`;
    wire.style.strokeDashoffset = String(len * (1 - d));
    wires.forEach((w, i) => {
      const ax = cx(i) - 125 - 2;
      w.arrow.setAttribute("points", `${ax},${cy} ${ax - 9},${cy - 5} ${ax - 9},${cy + 5}`);
      w.arrow.style.opacity = head > ax ? "1" : "0";
      if (w.label) {
        w.label.setAttribute("x", String((cx(i - 1) + 125 + ax) / 2));
        w.label.setAttribute("y", String(cy - 10));
        w.label.style.opacity = head > ax ? "1" : "0";
      }
    });
    const reached = sheets.map((_, i) => head > cx(i) - 125);
    sheets.forEach((s, i) => s.classList.toggle("on", reached[i]));
    now.textContent = L.fig2Now[Math.max(0, reached.lastIndexOf(true))];
  }
  function drawBridge() {
    const b = bridge.getBoundingClientRect(), g = gunSheet.getBoundingClientRect(), st = stage.getBoundingClientRect();
    const h = b.height, sx = g.left + g.width / 2 - b.left, sy = Math.max(0, g.bottom - b.top);
    const tx = wide() ? st.left + x0 - b.left : 16;
    bridgepath.setAttribute("d", `M${sx},${sy} V${h * 0.45} H${tx} V${h + 2}`);
  }

  // ---- fig.3 ----------------------------------------------------------------
  const f3 = $("f3"), f3wrap = f3.parentElement!, ruler = $<SVGSVGElement>("ruler"), mv = $("mv"), field = $("field");
  const X = (v: number) => 20 + v * 6;
  function drawRuler(t: number) {
    const axis = ramp(t, 0, 0.35);
    let s = `<line x1="20" y1="120" x2="${20 + 600 * axis}" y2="120" class="axis"/>`;
    for (let v = 0; v <= 100; v += 10) if (X(v) <= 20 + 600 * axis) s += `<line x1="${X(v)}" y1="${v % 50 ? 116 : 112}" x2="${X(v)}" y2="${v % 50 ? 124 : 128}" class="tick"/>`;
    s += `<text x="20" y="144" text-anchor="middle">0</text><text x="${X(50)}" y="144" text-anchor="middle">50</text><text x="${X(100)}" y="144" text-anchor="middle">100%</text>`;
    const b = ramp(t, 0.25, 0.5);
    s += `<g opacity="${b}"><line x1="${X(40.6)}" y1="120" x2="${X(40.6)}" y2="160" class="base"/><text x="${X(40.6) - 6}" y="172" text-anchor="end">final assembly 40.6%</text>`;
    s += `<line x1="${X(60.5)}" y1="120" x2="${X(60.5)}" y2="160" class="base"/><text x="${X(60.5) + 6}" y="172">best simple rule 60.5%</text><text x="${X(60.5) + 6}" y="185" class="sm">most-mentioned country</text></g>`;
    const v1 = lerp(0, 90.2, ramp(t, 0.35, 0.85)), v2 = lerp(0, 96.0, ramp(t, 0.5, 0.95));
    s += `<line x1="${X(v1)}" y1="40" x2="${X(v1)}" y2="120" class="mark"/><polygon points="${X(v1)},114 ${X(v1) - 5},106 ${X(v1) + 5},106" class="arrow"/>`;
    s += `<g opacity="${ramp(t, 0.8, 1)}"><path d="M${X(86.8)} 84 H${X(92.8)} M${X(86.8)} 78 V90 M${X(92.8)} 78 V90" class="bracket"/><text x="${X(90.2) - 8}" y="36" text-anchor="end" class="ink">origin accuracy 90.2%</text><text x="${X(90.2) - 8}" y="50" text-anchor="end" class="dim">86.8–92.8</text>`;
    s += `<line x1="${X(v2)}" y1="58" x2="${X(v2)}" y2="120" class="mark dash" opacity="${ramp(t, 0.5, 0.6)}"/>`;
    s += `<g opacity="${ramp(t, 0.9, 1)}"><path d="M${X(93.1)} 104 H${X(97.7)} M${X(93.1)} 98 V110 M${X(97.7)} 98 V110" class="bracket"/><text x="${X(96) + 6}" y="70" class="ink">96.0%</text><text x="${X(96) + 6}" y="83" class="dim">auto-accepted</text><text x="${X(96) + 6}" y="95" class="dim">93.1–97.7</text></g></g>`;
    ruler.innerHTML = s;
    mv.textContent = `${v1.toFixed(1)}%`;
  }
  // 377 squares: start scattered, settle into a grid sorted by outcome.
  // Counts from DESIGN.md (worked out from the official held-out rates).
  const kinds = ([["a", 288], ["b", 12], ["c", 52], ["d", 25]] as const).flatMap(([k, n]) => Array<string>(n).fill(k));
  let seed = 7;
  const rnd = () => (seed = (seed * 16807) % 2147483647) / 2147483647;
  const squares = kinds.map((k, i) => {
    const e = document.createElement("i");
    e.className = k;
    field.appendChild(e);
    return { e, i, sx: rnd(), sy: rnd(), sr: (rnd() - 0.5) * 140, delay: rnd() * 0.35 };
  });
  function drawField(t: number) {
    const W = field.clientWidth, H = field.clientHeight;
    const cols = 29, cell = Math.min((W - 2) / cols, 12.5), size = cell - 3;
    for (const s of squares) {
      const gx = (s.i % cols) * cell, gy = Math.floor(s.i / cols) * cell;
      const k = ease(clamp01((t - s.delay) / 0.6));
      s.e.style.width = s.e.style.height = `${size}px`;
      s.e.style.transform = `translate(${lerp(s.sx * (W - size), gx, k)}px, ${lerp(s.sy * (H - size), gy, k)}px) rotate(${lerp(s.sr, 0, k)}deg)`;
    }
  }
  // fig.3 stays pinned while it plays, so the squares settle over a long scroll.
  const fig3Progress = () => {
    if (!wide()) {
      const r = f3.getBoundingClientRect();
      return clamp01((innerHeight - r.top) / (innerHeight * 0.85));
    }
    const r = f3wrap.getBoundingClientRect();
    return clamp01((innerHeight * 0.55 - r.top) / (r.height - innerHeight * 0.45));
  };

  // ---- scroll stops -----------------------------------------------------------
  // On wide screens a wheel notch or a paging key moves to the next frame
  // where a figure has settled (or the bridge is centred) instead of stopping
  // halfway through a tween, and a drag or swipe that ends near a stop eases
  // onto it. Native scrolling is left alone with reduced motion, on phones,
  // inside other scrollable areas, and above or below the stops.
  let stops: number[] = [];
  function placeStops() {
    const vh = innerHeight;
    const top = (el: Element) => el.getBoundingClientRect().top + scrollY;
    const along = (el: HTMLElement, ps: number[]) => ps.map((p) => top(el) + p * Math.max(0, el.offsetHeight - vh));
    // fig.1: exploded, parts moved, CBP's 2026 ruling (see frame()).
    const s1 = along(scrolly, [0.38, 0.72, 1]);
    // fig.2: just after the wire reaches each sheet (see layoutF2()), then done.
    const gap = Math.min(330, (stage.clientWidth - 250) / 3), len = 3 * gap + 390;
    const unease = (d: number) => 0.5 - Math.sin(Math.asin(1 - 2 * d) / 3); // inverse of ease()
    const s2 = along(f2, [0, 1, 2, 3].map((i) => 0.08 + 0.78 * unease(clamp01((i * gap + 110) / len))).concat(0.92));
    // fig.3: squares lifting off, half settled, settled (inverse of fig3Progress()).
    const s3 = [0.15, 0.5, 1].map((t) => top(f3wrap) - (vh * 0.55 - t * (f3wrap.offsetHeight - vh * 0.45)));
    const end = document.documentElement.scrollHeight - vh;
    stops = [0, ...s1, top(bridge) + bridge.offsetHeight / 2 - vh / 2, ...s2, ...s3, end]
      .map((y) => Math.round(Math.min(end, Math.max(0, y))))
      .sort((a, b) => a - b);
  }
  const stepping = () => !reduce && wide() && stops.length > 1;
  let target: number | null = null, quietTimer = 0, settleTimer = 0;
  function go(y: number) {
    target = y;
    scrollTo({ top: y, behavior: "smooth" });
  }
  // A step ends once the page has arrived and the wheel has gone quiet, so a
  // trackpad's momentum after one flick doesn't run on through several stops.
  function release() {
    clearTimeout(quietTimer);
    quietTimer = window.setTimeout(() => {
      if (target === null || Math.abs(scrollY - target) < 2) target = null;
      else release();
    }, 180);
  }
  function step(dir: 1 | -1) {
    const y = scrollY;
    const next = dir > 0 ? stops.find((s) => s > y + 2) : [...stops].reverse().find((s) => s < y - 2);
    if (next === undefined) return false;
    go(next);
    return true;
  }
  const scrollsItself = (el: EventTarget | null) => {
    for (let n = el instanceof Element ? el : null; n && n !== root; n = n.parentElement) {
      const o = getComputedStyle(n).overflowY;
      if ((o === "auto" || o === "scroll") && n.scrollHeight > n.clientHeight) return true;
    }
    return !(el instanceof Node && root.contains(el));
  };
  function onWheel(e: WheelEvent) {
    if (!stepping() || e.ctrlKey || Math.abs(e.deltaY) < Math.abs(e.deltaX) || scrollsItself(e.target)) return;
    const dir = e.deltaY > 0 ? 1 : -1;
    if (target !== null) {
      e.preventDefault();
      release();
      return;
    }
    if (Math.abs(e.deltaY) < 4) return; // the tail of a trackpad gesture
    if (step(dir)) {
      e.preventDefault();
      release();
    }
  }
  const KEYS: Record<string, 1 | -1> = { PageDown: 1, PageUp: -1, ArrowDown: 1, ArrowUp: -1, " ": 1 };
  function onKey(e: KeyboardEvent) {
    const t = e.target as HTMLElement | null;
    if (!stepping() || e.altKey || e.ctrlKey || e.metaKey || !(e.key in KEYS)) return;
    if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT|BUTTON)$/.test(t.tagName))) return;
    const dir = e.key === " " && e.shiftKey ? -1 : KEYS[e.key];
    if (target !== null || step(dir)) {
      e.preventDefault();
      release();
    }
  }
  // Scrollbar drags, swipes and anchor jumps: once scrolling stops, ease onto
  // a stop that is close by (within a quarter of the screen).
  function onScroll() {
    if (target !== null || !stepping()) return;
    clearTimeout(settleTimer);
    settleTimer = window.setTimeout(() => {
      if (target !== null) return;
      const y = scrollY;
      const near = stops.reduce((a, s) => (Math.abs(s - y) < Math.abs(a - y) ? s : a), stops[0]);
      if (Math.abs(near - y) > 2 && Math.abs(near - y) < innerHeight * 0.25) {
        go(near);
        release();
      }
    }, 220);
  }
  placeStops();
  const stopsRo = new ResizeObserver(placeStops);
  stopsRo.observe(root);
  addEventListener("resize", placeStops);
  addEventListener("wheel", onWheel, { passive: false });
  addEventListener("keydown", onKey);
  addEventListener("scroll", onScroll, { passive: true });

  // ---- one loop ---------------------------------------------------------------
  const near = (el: Element) => {
    const r = el.getBoundingClientRect();
    return r.bottom > -innerHeight * 0.5 && r.top < innerHeight * 1.5;
  };
  let raf = 0, last = performance.now(), shown1 = 0, shown2 = 0, t3 = -1;
  const start = last;
  const ro = new ResizeObserver(() => gun?.resize());
  ro.observe(gunSheet);
  function frame(now_: number) {
    const dt = Math.min(0.05, (now_ - last) / 1000);
    last = now_;
    if (near(scrolly)) {
      const p = fig1Progress();
      shown1 = reduce ? p : shown1 + (p - shown1) * (1 - Math.exp(-dt * 7));
      const ex = ramp(shown1, 0.12, 0.36) - ramp(shown1, 0.8, 0.95);
      const mvv = ramp(shown1, 0.46, 0.7);
      const cbpv = ramp(shown1, 0.78, 0.9); // the rest of CBP's 2026 change, last stage only
      const step = shown1 < 0.2 ? 0 : shown1 < 0.44 ? 1 : shown1 < 0.76 ? 2 : 3;
      steps.forEach((li, i) => li.classList.toggle("on", i === step));
      caption.textContent = L.captions[step];
      tbstate.textContent = L.sheetStates[step];
      setStamp(shown1 > 0.76);
      plannote.classList.toggle("on", shown1 > 0.5 && shown1 < 0.97);
      gun?.frame(ex, mvv, dt, (now_ - start) / 1000, cbpv);
    }
    if (near(f2) || near(bridge)) {
      const p = fig2Progress();
      shown2 = reduce ? p : shown2 + (p - shown2) * 0.15;
      layoutF2(reduce ? 1 : shown2);
      drawBridge();
    }
    if (near(f3wrap)) {
      const t = reduce ? 1 : fig3Progress();
      if (Math.abs(t - t3) > 0.001) {
        t3 = t;
        drawRuler(t);
        drawField(t);
      }
    }
    raf = requestAnimationFrame(frame);
  }
  // Lay every figure out once, so each is in its start state (or its end
  // state with reduced motion) before any scrolling.
  layoutF2(reduce ? 1 : 0);
  drawBridge();
  drawRuler(reduce ? 1 : 0);
  drawField(reduce ? 1 : 0);
  raf = requestAnimationFrame(frame);

  return () => {
    cancelAnimationFrame(raf);
    ro.disconnect();
    barRo.disconnect();
    stopsRo.disconnect();
    clearTimeout(quietTimer);
    clearTimeout(settleTimer);
    removeEventListener("resize", placeStops);
    removeEventListener("wheel", onWheel);
    removeEventListener("keydown", onKey);
    removeEventListener("scroll", onScroll);
    gun?.dispose();
    field.replaceChildren();
    wires.forEach((w) => {
      w.arrow.remove();
      w.label?.remove();
    });
  };
}
