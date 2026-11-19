// Scroll state for the landing's timed figure replay (D103). Pure so that
// distinguishing the app's own smooth-scroll arrival from a visitor's scroll
// can be tested without a browser. A "seek" is a programmatic scroll the app
// started; an "arrival" is the replay it started at the same time.

export type Seek = { target: number; until: number };
export type Arrival = { pane: 2 | 3; start: number; duration: number };
export type ScrollState = {
  seek: Seek | null;
  arrival: Arrival | null;
  lastY: number;
  lastDistance: number;
};

export const newScrollState = (y: number): ScrollState => ({ seek: null, arrival: null, lastY: y, lastDistance: 0 });

// Start a programmatic scroll and, at the same time, an optional replay.
// Seeding lastDistance with the current gap keeps the first frame's motion
// toward the target from looking like a visitor scrolling away from it.
export const startScroll = (s: ScrollState, y: number, target: number, until: number, pane: 2 | 3 | null, now: number): ScrollState => ({
  ...s,
  seek: { target, until },
  arrival: pane === null ? null : { pane, start: now, duration: pane === 2 ? 2800 : 3400 },
  lastY: y,
  lastDistance: Math.abs(y - target),
});

export function advanceScroll(s: ScrollState, y: number, now: number): { state: ScrollState; seeking: boolean; interrupted: boolean } {
  const distance = s.seek ? Math.abs(y - s.seek.target) : 0;
  const seeking = s.seek !== null && now < s.seek.until && distance > 2;
  // While the app is still seeking, the gap normally shrinks; a growing gap
  // means the visitor moved the page. Once the seek has settled, any position
  // change is theirs. Either way the visitor takes control of the rendering:
  // the replay stops now, so it cannot jump back when its clock would expire.
  const interrupted =
    s.arrival !== null &&
    ((seeking && distance > s.lastDistance + 8) || (!seeking && s.seek === null && Math.abs(y - s.lastY) > 1));
  const settle = s.seek !== null && (!seeking || now >= s.seek.until);
  return {
    seeking,
    interrupted,
    state: {
      seek: interrupted || settle ? null : s.seek,
      arrival: interrupted ? null : s.arrival,
      lastY: y,
      lastDistance: distance,
    },
  };
}
