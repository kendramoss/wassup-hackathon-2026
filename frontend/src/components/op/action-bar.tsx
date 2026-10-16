"use client";

import { createContext, useContext, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

// Where each screen's actions go. ACTIONS_ON_TOP puts them in a sticky bar
// right under the case tabs, above the page's heading and description (user
// trial, 16 Oct). Set it to false to go back to the fixed bottom footer.
export const ACTIONS_ON_TOP = true;

const Slot = createContext<{ el: HTMLElement | null; set: (el: HTMLElement | null) => void }>({ el: null, set: () => {} });

export function ActionSlotProvider({ children }: { children: ReactNode }) {
  const [el, set] = useState<HTMLElement | null>(null);
  return <Slot.Provider value={{ el, set }}>{children}</Slot.Provider>;
}

// Rendered by the case tabs: the bar the screen's actions are portaled into,
// so they come right after the tabs in reading and focus order too.
export function ActionSlot() {
  const { set } = useContext(Slot);
  if (!ACTIONS_ON_TOP) return null;
  return (
    <div
      ref={set}
      role="region"
      aria-label="Actions"
      className="no-print sticky top-0 z-20 flex min-h-[56px] flex-wrap items-center gap-3 border-b border-line bg-paper px-4 py-2 empty:hidden sm:px-10"
    />
  );
}

export function useActionSlot() {
  return useContext(Slot).el;
}

export function Portal({ to, children }: { to: HTMLElement; children: ReactNode }) {
  return createPortal(children, to);
}
