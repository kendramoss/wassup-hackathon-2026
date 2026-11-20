"use client";

import { useCallback, useEffect, useState } from "react";
import { ACCESS_STALE, api, type CaseResponse } from "./api/client";

// Pinned demos are read-only for visitors (backend D57): plan, scenarios,
// compare, watch and ask answer but don't save. This tab keeps the latest
// state a screen produced for a demo, so a result doesn't vanish when the
// next screen loads the case again. Per tab, for this visit only.
const localKey = (id: string) => `op:local:${id}`;

function readLocal(id: string): CaseResponse | null {
  try {
    const s = sessionStorage.getItem(localKey(id));
    return s ? (JSON.parse(s) as CaseResponse) : null;
  } catch {
    return null;
  }
}

function writeLocal(c: CaseResponse) {
  try {
    if (c.id && c.pinned) sessionStorage.setItem(localKey(c.id), JSON.stringify(c));
  } catch {}
}

// Loads one case and lets a screen replace it after a mutation.
export function useCase(id: string) {
  const [data, setState] = useState<CaseResponse | null>(null);
  const [error, setError] = useState<unknown>(null);
  const fetchCase = useCallback(
    () =>
      api.getCase(id).then((c) => {
        setState(c.pinned ? (readLocal(id) ?? c) : c);
      }, setError),
    [id],
  );
  useEffect(() => {
    fetchCase();
  }, [fetchCase]);
  // A write was refused (403): the case or the caller's role may have changed,
  // so show what the server says now. One read at a time; a GET never raises
  // this, so it cannot loop.
  useEffect(() => {
    if (typeof window === "undefined") return;
    let reading = false;
    const stale = () => {
      if (reading) return;
      reading = true;
      fetchCase().finally(() => {
        reading = false;
      });
    };
    window.addEventListener(ACCESS_STALE, stale);
    return () => window.removeEventListener(ACCESS_STALE, stale);
  }, [fetchCase]);
  const reload = useCallback(() => {
    setError(null);
    fetchCase();
  }, [fetchCase]);
  const setData = useCallback((next: CaseResponse | null | ((d: CaseResponse | null) => CaseResponse | null)) => {
    setState((d) => {
      const c = typeof next === "function" ? next(d) : next;
      if (c) writeLocal(c);
      return c;
    });
  }, []);
  return { data, error, reload, setData };
}

export function caseTitle(c: CaseResponse | null): string {
  if (!c) return "Loading…";
  return c.display_name || c.product?.name || (c.facts ?? "").split(/\s+/).slice(0, 6).join(" ") + "…";
}
