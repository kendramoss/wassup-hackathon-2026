// Pure input classification for the landing's wheel/key stepping. Kept out of
// motion.ts so modifier and focused-control behaviour can be tested in Node.

export type WheelInput = { deltaY: number; ctrlKey: boolean; metaKey: boolean };
export type KeyInput = { key: string; shiftKey: boolean };

const SPACE_KEYS = [" ", "Spacebar"];
<<<<<<< HEAD
// Native buttons, links, ARIA controls, and <summary> handle Space/Enter themselves.
const CONTROL = "button, a[href], summary, [role=button], [role=link], [role=switch], [role=checkbox], [role=tab], [role=menuitem]";
=======
// Native buttons, disclosure summaries and ARIA controls handle Space themselves; the landing must
// not cancel that activation (the AppBar account controls are native buttons).
const CONTROL = "button, summary, a[href], [role=button], [role=link], [role=switch], [role=checkbox], [role=tab], [role=menuitem]";
>>>>>>> origin/main

export const isSpaceKey = (e: KeyInput) => SPACE_KEYS.includes(e.key);

export const inControl = (target: { closest(s: string): Element | null } | null) =>
  !!target?.closest(CONTROL);

export function wheelDirection(e: WheelInput): -1 | 0 | 1 {
  // Ctrl-wheel (and Meta-wheel) is browser zoom or a trackpad pinch: it must
  // keep its default action and must not enter the wheel-burst bookkeeping.
  if (e.ctrlKey || e.metaKey) return 0;
  return Math.sign(e.deltaY) as -1 | 0 | 1;
}

export function keyDirection(e: KeyInput): -1 | 0 | 1 {
  if (isSpaceKey(e)) return e.shiftKey ? -1 : 1; // Shift+Space pages backward
  if (e.key === "ArrowDown" || e.key === "PageDown") return 1;
  if (e.key === "ArrowUp" || e.key === "PageUp") return -1;
  return 0;
}
