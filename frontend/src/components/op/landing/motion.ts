import { createHeatGun, type Country, type HeatGun } from "./heat-gun";

const phases = [
  { exploded: 0, moved: 0 },
  { exploded: 1, moved: 0 },
  { exploded: 1, moved: 1 },
  { exploded: 0, moved: 1 },
] as const;

// Only the on-screen schematic renders frames. The page scrolls natively and
// the case narrative works even if WebGL is unavailable.
export function startHeatGun(host: HTMLElement, flag: (country: Country) => string) {
  const canvas = host.querySelector("canvas")!;
  const motion = matchMedia("(prefers-reduced-motion: reduce)");
  let gun: HeatGun;
  try {
    const context = canvas.getContext("webgl2", { antialias: true, alpha: true });
    if (!context) {
      host.dataset.ready = "false";
      return;
    }
    gun = createHeatGun({
      canvas,
      context,
      labels: host.querySelector("[data-labels]")!,
      leaders: host.querySelector("svg")!,
      flag,
      reduce: motion.matches,
    });
  } catch {
    host.dataset.ready = "false";
    return;
  }
  host.dataset.ready = "true";
  let id = 0;
  let visible = true;
  let last = performance.now();
  let exploded = 0;
  let moved = 0;

  function frame(now: number) {
    const dt = Math.min((now - last) / 1000, 0.05);
    const step = phases[Number(host.dataset.phase)] ?? phases[0];
    const rate = motion.matches ? 1 : 1 - Math.exp(-10 * dt);
    exploded += (step.exploded - exploded) * rate;
    moved += (step.moved - moved) * rate;
    gun.frame(exploded, moved, dt, now / 1000);
    last = now;
    id = visible ? requestAnimationFrame(frame) : 0;
  }

  const resize = new ResizeObserver(() => gun.resize());
  resize.observe(host);
  const view = new IntersectionObserver(([entry]) => {
    visible = entry.isIntersecting;
    if (visible && !id) {
      last = performance.now();
      id = requestAnimationFrame(frame);
    }
  });
  view.observe(host);
  id = requestAnimationFrame(frame);

  return () => {
    cancelAnimationFrame(id);
    resize.disconnect();
    view.disconnect();
    gun.dispose();
    delete host.dataset.ready;
  };
}
