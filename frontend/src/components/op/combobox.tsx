"use client";

import { useEffect, useId, useLayoutEffect, useMemo, useRef, useState } from "react";
import { allCountries } from "@/lib/countries";
import { Flag, cx } from "./ui";

// A searchable dropdown (18 Oct): type to filter, arrow keys and Enter to
// pick, at most 8 options in view with the rest a scroll away. With
// allowCustom a value that isn't listed (a less-known country, a supplier)
// can still be typed and kept. The list is position: fixed, so a table's
// horizontal scroll never clips it.

export type ComboOption = { value: string; label: string; flag?: boolean; hint?: string };

const ROW = 36; // px per option; 8 rows show before the list scrolls
const VISIBLE = 8;

export const countryOptions: ComboOption[] = allCountries.map(([name]) => ({ value: name, label: name, flag: true }));

export function Combobox({
  value,
  onChange,
  options,
  placeholder,
  ariaLabel,
  id,
  className,
  allowCustom = false,
  invalid = false,
  flags = false,
  customLabel = (t: string) => `Use “${t}”`,
  customHint = "not in the list",
}: {
  value: string;
  onChange: (v: string) => void;
  options: ComboOption[];
  placeholder?: string;
  ariaLabel?: string;
  id?: string;
  className?: string;
  allowCustom?: boolean;
  invalid?: boolean;
  // Show a flag before the typed value too (country fields).
  flags?: boolean;
  // How the "keep what I typed" option reads.
  customLabel?: (typed: string) => string;
  customHint?: string;
}) {
  const auto = useId();
  const inputId = id ?? auto;
  const listId = `${inputId}-list`;
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState<string | null>(null); // null: not typing
  const [active, setActive] = useState(0);
  const [box, setBox] = useState<{ left: number; top: number; width: number; up: boolean } | null>(null);

  const selected = options.find((o) => o.value === value);
  const shown = query ?? selected?.label ?? value;
  const matches = useMemo(() => {
    const q = (query ?? "").trim().toLowerCase();
    if (!q) return options;
    const starts = options.filter((o) => o.label.toLowerCase().startsWith(q));
    const has = options.filter((o) => !o.label.toLowerCase().startsWith(q) && (o.label.toLowerCase().includes(q) || o.value.toLowerCase().includes(q)));
    return [...starts, ...has];
  }, [options, query]);
  const typed = (query ?? "").trim();
  const custom = allowCustom && typed !== "" && !options.some((o) => o.label.toLowerCase() === typed.toLowerCase());
  const items: ComboOption[] = custom ? [...matches, { value: typed, label: customLabel(typed), hint: customHint }] : matches;

  const place = () => {
    const r = input.current?.getBoundingClientRect();
    if (!r) return;
    const height = Math.min(items.length || 1, VISIBLE) * ROW + 2;
    const up = r.bottom + height > window.innerHeight && r.top > height;
    setBox({ left: r.left, top: up ? r.top - height - 4 : r.bottom + 4, width: Math.max(r.width, 200), up });
  };
  useLayoutEffect(() => {
    if (open) place();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, items.length]);
  useEffect(() => {
    if (!open) return;
    const close = () => setOpen(false);
    window.addEventListener("resize", close);
    window.addEventListener("scroll", place, true);
    return () => {
      window.removeEventListener("resize", close);
      window.removeEventListener("scroll", place, true);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);
  useEffect(() => {
    list.current?.querySelector<HTMLElement>(`[data-i="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [active]);

  const pick = (o: ComboOption) => {
    onChange(o.value);
    setQuery(null);
    setOpen(false);
  };
  const leave = () => {
    // Typed text that matches nothing is kept only where custom values are allowed.
    if (query !== null) {
      const exact = options.find((o) => o.label.toLowerCase() === typed.toLowerCase());
      if (exact) onChange(exact.value);
      else if (allowCustom) onChange(typed);
    }
    setQuery(null);
    setOpen(false);
  };

  return (
    <span className="relative block">
      {flags && query === null && value && (
        <span className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2">
          <Flag country={value} />
        </span>
      )}
      <input
        ref={input}
        id={inputId}
        role="combobox"
        aria-label={ariaLabel}
        aria-expanded={open}
        aria-controls={listId}
        aria-autocomplete="list"
        aria-invalid={invalid || undefined}
        aria-activedescendant={open && items[active] ? `${listId}-${active}` : undefined}
        autoComplete="off"
        placeholder={placeholder}
        className={cx(className, "pr-8", flags && query === null && value && "pl-8", invalid && "!border-ask bg-ask-wash")}
        value={shown}
        onFocus={() => {
          setActive(Math.max(0, options.findIndex((o) => o.value === value)));
          setOpen(true);
        }}
        onClick={() => setOpen(true)}
        onChange={(e) => {
          setQuery(e.target.value);
          setActive(0);
          setOpen(true);
        }}
        onBlur={leave}
        onKeyDown={(e) => {
          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
            e.preventDefault();
            if (!open) return setOpen(true);
            const n = items.length;
            if (n) setActive((a) => (e.key === "ArrowDown" ? (a + 1) % n : (a - 1 + n) % n));
          } else if (e.key === "Enter" && open && items[active]) {
            e.preventDefault();
            pick(items[active]);
          } else if (e.key === "Escape" && open) {
            e.preventDefault();
            setQuery(null);
            setOpen(false);
          }
        }}
      />
      <span aria-hidden="true" className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 text-[10px] text-dim">
        ▾
      </span>
      {open && box && (
        <ul
          ref={list}
          id={listId}
          role="listbox"
          aria-label={ariaLabel}
          style={{ left: box.left, top: box.top, width: box.width, maxHeight: VISIBLE * ROW + 2 }}
          className="fixed z-50 overflow-y-auto rounded-[2px] border border-edge bg-white py-0 shadow-[3px_3px_0_var(--shadow)]"
        >
          {items.length === 0 && <li className="flex h-9 items-center px-3 text-[13px] text-dim">No match</li>}
          {items.map((o, i) => (
            <li
              key={`${o.value}-${i}`}
              id={`${listId}-${i}`}
              data-i={i}
              role="option"
              aria-selected={o.value === value}
              // mousedown, not click: picking must happen before the input blurs.
              onMouseDown={(e) => {
                e.preventDefault();
                pick(o);
              }}
              onMouseEnter={() => setActive(i)}
              style={{ height: ROW }}
              className={cx(
                "flex cursor-pointer items-center gap-2 px-3 text-[13px]",
                i === active ? "bg-ok-wash text-ink" : "text-body",
                o.value === value && "font-medium",
              )}
            >
              {o.flag && <Flag country={o.value} />}
              <span className="truncate">{o.label}</span>
              {o.hint && <span className="ml-auto shrink-0 text-[11px] text-dim">{o.hint}</span>}
            </li>
          ))}
        </ul>
      )}
    </span>
  );
}
