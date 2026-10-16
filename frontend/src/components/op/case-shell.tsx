"use client";

import { useParams } from "next/navigation";
import type { ReactNode } from "react";
import type { CaseResponse } from "@/lib/api/client";
import { formatDate, stepIndex } from "@/lib/format";
import { caseTitle, useCase } from "@/lib/use-case";
import { ErrorBox, Main, StepBar } from "./chrome";
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
export function CaseShell({ step, children }: { step: number; children: (ctx: CaseCtx) => ReactNode }) {
  const { id } = useParams<{ id: string }>();
  const { data, error, reload, setData } = useCase(id);
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
      <StepBar caseId={id} current={step} reached={Math.max(reached, step)} title={title} />
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
    </>
  );
}
