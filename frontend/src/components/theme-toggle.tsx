"use client";

import { useSyncExternalStore } from "react";
import { Monitor, Moon, Sun } from "./icons";

type Theme = "system" | "light" | "dark";
const order: Theme[] = ["system", "light", "dark"];
const labels: Record<Theme, string> = { system: "System", light: "Light", dark: "Dark" };
const listeners = new Set<() => void>();

function current(): Theme {
  const t = document.documentElement.dataset.theme;
  return t === "light" || t === "dark" ? t : "system";
}

function apply(theme: Theme) {
  if (theme === "system") delete document.documentElement.dataset.theme;
  else document.documentElement.dataset.theme = theme;
  try {
    if (theme === "system") localStorage.removeItem("theme");
    else localStorage.setItem("theme", theme);
  } catch {
    // Storage blocked (private mode): the choice lasts until reload.
  }
  listeners.forEach((l) => l());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

// Cycles System -> Light -> Dark. The saved choice is applied before first
// paint by the script in layout.tsx.
export function ThemeToggle() {
  const theme = useSyncExternalStore(subscribe, current, () => "system" as Theme);
  const next = order[(order.indexOf(theme) + 1) % order.length];
  const IconFor = theme === "light" ? Sun : theme === "dark" ? Moon : Monitor;

  return (
    <button
      type="button"
      onClick={() => apply(next)}
      className="inline-flex size-9 items-center justify-center rounded-md text-muted hover:bg-subtle hover:text-fg"
      aria-label={`Theme: ${labels[theme]}. Switch to ${labels[next]}`}
      title={`Theme: ${labels[theme]}`}
    >
      <IconFor />
    </button>
  );
}
