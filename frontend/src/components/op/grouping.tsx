"use client";

import { createContext, useContext, type ReactNode } from "react";
import { groupName } from "@/lib/families";

// The template version the open case's stored plan was made with (D372), so
// a group split after that plan keeps the name it had then. CaseShell sets
// it; outside a case it is unset and every group reads its current name.
const MadeWith = createContext<string | undefined>(undefined);

export function GroupingProvider({ madeWith, children }: { madeWith?: string; children: ReactNode }) {
  return <MadeWith.Provider value={madeWith}>{children}</MadeWith.Provider>;
}

export function useMadeWith(): string | undefined {
  return useContext(MadeWith);
}

// groupName with the open case's plan version filled in.
export function useGroupName(): typeof groupName {
  const madeWith = useMadeWith();
  return (id, family, product, v) => groupName(id, family, product, v ?? madeWith);
}
