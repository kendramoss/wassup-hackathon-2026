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
  const barH = () => bar?.offsetHeight ?? 0;
  const setBar = () => bar && root.style.setProperty("--bar", `${bar.offsetHeight}px`);
  setBar();
  const barRo = new ResizeObserver(setBar);
  if (bar) barRo.observe(bar);

  // ---- the stage -------------------------------------------------------------
  // Wide screens pin one stage and move its three panes sideways as the page
  // scrolls down (D93). Scroll is native: the page position, in screens past
  // the stage's top, is split into the figures' phases below. Phones stack
  // the panes and each figure keeps its own scroll progress.
  const hz = $("hz"), hzpin = $("hzpin"), track = $("track"), hzfill = $("hzfill");
  const panes = [...track.children] as HTMLElement[];
  // Screens of scroll: fig.1 plays over 0–4, slides to fig.2 over 4–5.5,
  // fig.2 fills over 5.3–7.5, slides to fig.3 over 7.5–9, fig.3 settles
  // over 8.9–10.5 and holds on the way into the cases until 11.
  const SCREENS = 11, ARRIVE = [0, 5.5, 10.6];
  const horizontal = () => wide();
  // The pin sticks once the stage's top reaches the AppBar, so progress
  // counts from there.
  const travel = () => Math.max(1, hz.offsetHeight - hzpin.clientHeight);
  const at = () => clamp01((barH() - hz.getBoundingClientRect().top) / travel()) * SCREENS;
  const seg = (a: number, b: number) => clamp01((at() - a) / (b - a));
  // Panes moved so far: eased while scrolling, a cut with reduced motion.
  const slid = () => {
    const P = at();
    return reduce ? (P >= 4.75 ? 1 : 0) + (P >= 8.25 ? 1 : 0) : ramp(P, 4, 5.5) + ramp(P, 7.5, 9);
  };
  const goTo = (i: number, smooth: boolean) =>
    scrollTo({ top: hz.getBoundingClientRect().top + scrollY - barH() + (ARRIVE[i] / SCREENS) * travel(), behavior: smooth && !reduce ? "smooth" : "instant" });
  const hzgo = $$<HTMLButtonElement>("hzgo");
  const onGo = (e: Event) => goTo(Number((e.currentTarget as HTMLElement).dataset.i), true);
  hzgo.forEach((b) => b.addEventListener("click", onGo));
  // Tabbing into a pane that is off to the side brings it into view; the pin
  // clips sideways, so the browser would otherwise scroll it on its own.
  const onFocus = (e: FocusEvent) => {
    if (!horizontal()) return;
    hzpin.scrollLeft = 0;
    const i = panes.findIndex((p) => p.contains(e.target as Node));
    if (i >= 0 && Math.round(slid()) !== i) goTo(i, false);
  };
  track.addEventListener("focusin", onFocus);

  // ---- fig.1 ----------------------------------------------------------------
  const scroller = $("scroller"), gunSheet = $("gunsheet");
  let gun: ReturnType<typeof createHeatGun> | null = null;
  try {
    gun = createHeatGun({ canvas: $<HTMLCanvasElement>("canvas"), labels: $("labels"), leaders: $<SVGSVGElement>("leaders"), flag, reduce });
  } catch {
    gunSheet.classList.add("nogl"); // no WebGL: the story text and stamp still work
  }
  const stamp = $("stamp"), plannote = $("plannote"), tbstate = $("tbstate");
  const caps = $$("cap");
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
    if (horizontal()) return seg(0, 4);
    const r = scroller.getBoundingClientRect();
    return clamp01(-r.top / Math.max(1, r.height - innerHeight));
  };

  // ---- fig.2 ----------------------------------------------------------------
  const f2 = $("f2"), stage = $("stage"), now = $("now");
  const sheets = $$("mini");
  const pathsvg = $<SVGSVGElement>("pathsvg"), wire = $<SVGPathElement>("wire");
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
  const fig2Progress = () => {
    if (horizontal()) return seg(5.3, 7.5);
    const r = f2.getBoundingClientRect();
    return clamp01(-r.top / Math.max(1, r.height - innerHeight));
  };
  function layoutF2(p: number) {
    if (!wide()) {
      sheets.forEach((s) => s.classList.add("on"));
      return;
    }
    const W = stage.clientWidth, H = stage.clientHeight;
    // Four 250 px sheets need 1048 px with 16 px between them; narrower, they
    // scale down together so they never overlap (18 Oct). half is half a
    // sheet's drawn width, for the wire's ends and arrows.
    const k = Math.min(1, (W - 48) / 1000);
    const half = 125 * k;
    const gap = Math.min(330, (W - 2 * half) / 3);
    // One flat row. The wire draws left to right; each sheet rises into
    // place just before the wire reaches it, then fills in.
    const d = ramp(p, 0.08, 0.86);
    const cx = (i: number) => W / 2 + (i - 1.5) * gap, cy = H / 2;
    const x0 = cx(0) - half - 70;
    const x1 = cx(3) + half + 70;
    const head = lerp(x0, x1, d);
    sheets.forEach((s, i) => {
      const rise = reduce ? 1 : ease(clamp01((head - (cx(i) - half - gap * 0.55)) / (gap * 0.45)));
      s.style.transform = `translate(${(i - 1.5) * gap}px, ${lerp(12, 0, rise)}px) scale(${k})`;
    });
    wire.setAttribute("d", `M${x0},${cy} H${x1}`);
    const len = x1 - x0;
    wire.style.strokeDasharray = `${len}`;
    wire.style.strokeDashoffset = String(len * (1 - d));
    wires.forEach((w, i) => {
      const ax = cx(i) - half - 2;
      w.arrow.setAttribute("points", `${ax},${cy} ${ax - 9},${cy - 5} ${ax - 9},${cy + 5}`);
      w.arrow.style.opacity = head > ax ? "1" : "0";
      if (w.label) {
        w.label.setAttribute("x", String((cx(i - 1) + half + ax) / 2));
        w.label.setAttribute("y", String(cy - 10));
        w.label.style.opacity = head > ax ? "1" : "0";
      }
    });
    // The wire starts at the Product sheet, so it is filled from the start.
    const reached = sheets.map((_, i) => i === 0 || head > cx(i) - half);
    const at = reached.lastIndexOf(true);
    sheets.forEach((s, i) => {
      s.classList.toggle("on", reached[i]);
      s.classList.toggle("cur", i === at);
    });
    now.textContent = L.fig2Now[Math.max(0, reached.lastIndexOf(true))];
  }
  // ---- fig.3 ----------------------------------------------------------------
  const f3 = $("f3"), endcta = $("endcta"), ruler = $<SVGSVGElement>("ruler"), mv = $("mv"), field = $("field");
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
  const fig3Progress = () => {
    if (horizontal()) return seg(8.9, 10.5);
    const r = f3.getBoundingClientRect();
    return clamp01((innerHeight - r.top) / (innerHeight * 0.85));
  };

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
    const s = horizontal() ? slid() : 0;
    if (horizontal()) {
      track.style.transform = `translateX(${-s * hzpin.clientWidth}px)`;
      hzfill.style.width = `${(at() / SCREENS) * 100}%`;
      hzgo.forEach((b, i) => b.toggleAttribute("aria-current", Math.round(s) === i));
    } else track.style.transform = "";
    if (near(gunSheet) && s < 1) {
      const p = fig1Progress();
      shown1 = reduce ? p : shown1 + (p - shown1) * (1 - Math.exp(-dt * 7));
      const ex = ramp(shown1, 0.12, 0.36) - ramp(shown1, 0.8, 0.95);
      const mvv = ramp(shown1, 0.46, 0.7);
      const cbpv = ramp(shown1, 0.74, 0.82); // anything else CBP's 2026 change moved (none drawn now), while still exploded (reassembly starts at 0.8)
      const step = shown1 < 0.2 ? 0 : shown1 < 0.44 ? 1 : shown1 < 0.76 ? 2 : 3;
      caps.forEach((c, i) => (c.hidden = i !== step));
      tbstate.textContent = L.sheetStates[step];
      setStamp(shown1 > 0.76);
      plannote.classList.toggle("on", shown1 > 0.5 && shown1 < 0.97);
      gun?.frame(ex, mvv, dt, (now_ - start) / 1000, cbpv);
    }
    if (near(f2) && (!horizontal() || (s > 0.2 && s < 1.8))) {
      const p = fig2Progress();
      shown2 = reduce ? p : shown2 + (p - shown2) * 0.15;
      layoutF2(reduce ? 1 : shown2);
    }
    if (near(f3) && (!horizontal() || s > 1.2)) {
      const t = reduce ? 1 : fig3Progress();
      if (Math.abs(t - t3) > 0.001) {
        t3 = t;
        drawRuler(t);
        drawField(t);
        endcta.classList.toggle("on", t > 0.97);
      }
    }
    raf = requestAnimationFrame(frame);
  }
  // Lay every figure out once, so each is in its start state (or its end
  // state with reduced motion) before any scrolling.
  layoutF2(reduce ? 1 : 0);
  drawRuler(reduce ? 1 : 0);
  drawField(reduce ? 1 : 0);
  endcta.classList.toggle("on", reduce);
  raf = requestAnimationFrame(frame);

  return () => {
    cancelAnimationFrame(raf);
    ro.disconnect();
    barRo.disconnect();
    track.removeEventListener("focusin", onFocus);
    hzgo.forEach((b) => b.removeEventListener("click", onGo));
    gun?.dispose();
    field.replaceChildren();
    wires.forEach((w) => {
      w.arrow.remove();
      w.label?.remove();
    });
  };
}
