"use client";

import { createContext, useContext, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

// Where each screen's actions go. ACTIONS_ON_TOP puts them at the right of
// the sticky case tabs row (nav, then the step's main button; 17 Oct). Set it
// to false to go back to the fixed bottom footer.
export const ACTIONS_ON_TOP = true;

type SlotCtx = { el: HTMLElement | null; set: (el: HTMLElement | null) => void; next: HTMLElement | null; setNext: (el: HTMLElement | null) => void };
const Slot = createContext<SlotCtx>({ el: null, set: () => {}, next: null, setNext: () => {} });

// One provider per page (CaseShell, Step 1): Next keeps the previous page
// mounted but hidden after a client navigation, so a shared slot would let
// the hidden page's actions land in the visible bar.
export function ActionSlotProvider({ children }: { children: ReactNode }) {
  const [el, set] = useState<HTMLElement | null>(null);
  const [next, setNext] = useState<HTMLElement | null>(null);
  return <Slot.Provider value={{ el, set, next, setNext }}>{children}</Slot.Provider>;
}

// Rendered at the end of the case tabs row: the screen's buttons are
// portaled here, so they come right after the tabs in reading and focus order.
export function ActionSlot() {
  const { set } = useContext(Slot);
  if (!ACTIONS_ON_TOP) return null;
  return (
    <div
      ref={set}
      role="region"
      aria-label="Actions"
      className="flex flex-wrap items-center gap-2.5 empty:hidden"
    />
  );
}

export function useActionSlot() {
  return useContext(Slot).el;
}

// The page's "Next steps" column registers here; the screen's <Next>
// sentence is portaled into it instead of crowding the tabs row.
export function useNextSlot() {
  return useContext(Slot);
}

export function Portal({ to, children }: { to: HTMLElement; children: ReactNode }) {
  return createPortal(children, to);
}
