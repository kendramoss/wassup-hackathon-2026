"use client";

import { useCallback, useEffect, useState } from "react";
import { api, type CaseResponse } from "./api/client";

// Loads one case and lets a screen replace it after a mutation.
export function useCase(id: string) {
  const [data, setData] = useState<CaseResponse | null>(null);
  const [error, setError] = useState<unknown>(null);
  const fetchCase = useCallback(() => api.getCase(id).then(setData, setError), [id]);
  useEffect(() => {
    fetchCase();
  }, [fetchCase]);
  const reload = useCallback(() => {
    setError(null);
    fetchCase();
  }, [fetchCase]);
  return { data, error, reload, setData };
}

export function caseTitle(c: CaseResponse | null): string {
  if (!c) return "Loading…";
  return c.display_name || c.product?.name || (c.facts ?? "").split(/\s+/).slice(0, 6).join(" ") + "…";
}
