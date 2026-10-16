"use client";

import { useParams } from "next/navigation";
import { useState, type ReactNode } from "react";
import type { CaseResponse } from "@/lib/api/client";
import { formatDate, stepIndex } from "@/lib/format";
import { caseTitle, useCase } from "@/lib/use-case";
import { AskDrawer } from "./ask-drawer";
import { CaseTabs, ErrorBox, Main, tabStep, type CaseTab } from "./chrome";
import { Spinner } from "./ui";

export type CaseCtx = {
  id: string;
  c: CaseResponse;
  setCase: (c: CaseResponse) => void;
  reload: () => void;
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
  const reached = data ? (stepIndex[data.step ?? "origin"] ?? 2) : step;
  const title = data ? (
    <>
      {caseTitle(data)} <span className="text-dim">· {formatDate(data.date)}</span>
    </>
  ) : (
    "Loading…"
  );
  return (
    <>
      <CaseTabs caseId={id} current={tab} reached={Math.max(reached, step)} title={title} onAsk={data?.prediction?.origin_country ? () => setAsking(true) : undefined} />
      {error ? (
        <Main>
          <ErrorBox error={error} onRetry={reload} />
        </Main>
      ) : !data ? (
        <Main>
          <Spinner label="Loading the case…" />
        </Main>
      ) : (
        children({ id, c: data, setCase: setData, reload })
      )}
      {asking && data && <AskDrawer c={data} onClose={() => setAsking(false)} onCase={setData} />}
    </>
  );
}
