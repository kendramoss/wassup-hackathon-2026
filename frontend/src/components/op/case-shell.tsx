"use client";

import { useParams } from "next/navigation";
import { useState, type ReactNode } from "react";
import Link from "next/link";
import { ApiError, type CaseResponse } from "@/lib/api/client";
import { formatDate, stepIndex } from "@/lib/format";
import { caseTitle, useCase } from "@/lib/use-case";
import { ActionSlotProvider } from "./action-bar";
import { AskDrawer } from "./ask-drawer";
import { CaseTabs, ErrorBox, Main, tabStep, type CaseTab } from "./chrome";
import { Spinner } from "./ui";

export type CaseCtx = {
  id: string;
  c: CaseResponse;
  setCase: (c: CaseResponse) => void;
  reload: () => void;
  // Opens the Ask drawer (also offered as a suggestion on screens).
  ask: () => void;
};

// The case as the API stores it also carries current_origin (CBP's holding on
// the current setup), which the OpenAPI CaseResponse doesn't list yet.
export function currentOrigin(c: CaseResponse): { country?: string; ruling?: string } | undefined {
  return (c as CaseResponse & { current_origin?: { country?: string; ruling?: string } }).current_origin ?? c.plan?.current_origin;
}

// Loads the case from the URL and renders the step bar around a screen.
export function CaseShell({ tab, children }: { tab: CaseTab; children: (ctx: CaseCtx) => ReactNode }) {
  const { id } = useParams<{ id: string }>();
  const { data, error, reload, setData } = useCase(id);
  const [asking, setAsking] = useState(false);
  const step = tabStep[tab];
  const gone = error instanceof ApiError && error.status === 404;
  const reached = data ? (stepIndex[data.step ?? "origin"] ?? 2) : step;
  const title = data ? (
    <>
      {caseTitle(data)} <span className="text-dim">· {formatDate(data.date)}</span>
    </>
  ) : gone ? (
    "Case not found"
  ) : (
    "Loading…"
  );
  return (
    <ActionSlotProvider>
      <CaseTabs caseId={gone ? undefined : id} current={tab} reached={Math.max(reached, step)} title={title} onAsk={data?.prediction?.origin_country ? () => setAsking(true) : undefined} />
      {gone ? (
        // A deleted case or a mistyped link: say so and offer the way back (#88).
        <Main className="flex flex-col gap-3">
          <h1 className="text-xl tracking-[-0.04em]">Case not found</h1>
          <p className="duo max-w-[72ch] text-sm text-dim">
            There is no case <span className="font-mono">{id}</span>. It may have been deleted, or the link is incomplete.
          </p>
          <p>
            <Link href="/" className="text-sm text-ok underline underline-offset-2">
              ← Back to your cases
            </Link>
          </p>
        </Main>
      ) : error ? (
        <Main>
          <ErrorBox error={error} onRetry={reload} />
        </Main>
      ) : !data ? (
        <Main>
          <Spinner label="Loading the case…" />
        </Main>
      ) : (
        children({ id, c: data, setCase: setData, reload, ask: () => setAsking(true) })
      )}
      {asking && data && <AskDrawer c={data} onClose={() => setAsking(false)} onChat={(chat) => setData((d) => (d ? ({ ...d, chat } as CaseResponse) : d))} />}
    </ActionSlotProvider>
  );
}
