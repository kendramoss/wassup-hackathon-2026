// The landing's single scroll timeline (D93). One unit is one screen of page
// scroll past the pinned stage. Kept free of DOM APIs so the navigation stops
// can be tested without a browser.
export const SCREENS = 11;

export const clamp01 = (v: number) => Math.min(1, Math.max(0, v));
export const ease = (t: number) => t * t * (3 - 2 * t);
export const ramp = (p: number, a: number, b: number) => ease(clamp01((p - a) / (b - a)));

// fig.1's four states are bounded by 0.2 / 0.44 / 0.76. A stop sits in the
// middle of each band so arriving at one shows its settled state, not the
// boundary where the caption is about to flip.
export const PHASE_STOPS = [0.1, 0.32, 0.6, 0.88] as const;
export const phaseStop = (i: number) => PHASE_STOPS[i] * 4;

// Figure stops: fig.1 on phase 1, fig.2 and fig.3 at the *end* of their own
// animation. The old fig.2 stop (5.5) landed on a blank sheet because its
// wire only starts drawing at 5.3.
export const ARRIVE = [phaseStop(0), 7.5, 10.5] as const;

// One wheel gesture advances one stop: the four settled heat-gun phases,
// then fig.2 and fig.3 at the end of their own animations.
export const STOPS: readonly number[] = [phaseStop(0), phaseStop(1), phaseStop(2), phaseStop(3), ARRIVE[1], ARRIVE[2]];

// Which figure replays its own on-arrival animation at a stop. Heat-gun
// phases settle instantly; the two finished figures draw themselves in time.
export const arrivalFor = (p: number): 2 | 3 | null => (p === ARRIVE[1] ? 2 : p === ARRIVE[2] ? 3 : null);

// Past the last figure, the next down goes all the way to the page bottom;
// back past the first, the next up goes all the way to the page top.
export type Destination = { kind: "stop"; progress: number } | { kind: "top" } | { kind: "bottom" };
export const nextDestination = (p: number, dir: 1 | -1): Destination => {
  const next = dir > 0 ? STOPS.find((s) => s > p + 0.05) : [...STOPS].reverse().find((s) => s < p - 0.05);
  return next !== undefined ? { kind: "stop", progress: next } : dir > 0 ? { kind: "bottom" } : { kind: "top" };
};

export const stepAt = (p: number) => (p < 0.2 ? 0 : p < 0.44 ? 1 : p < 0.76 ? 2 : 3);

// Continuous position of the sideways slide: 0 = fig.1, 1 = fig.2, 2 = fig.3.
export const paneAt = (p: number) => ramp(p, 4, 5.5) + ramp(p, 7.5, 9);

export const fig1At = (p: number) => clamp01(p / 4);
export const fig2At = (p: number) => clamp01((p - 5.3) / (7.5 - 5.3));
export const fig3At = (p: number) => clamp01((p - 8.9) / (10.5 - 8.9));
