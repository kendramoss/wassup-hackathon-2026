"use client";

import { useParams, useRouter } from "next/navigation";
import { useEffect } from "react";
import { stepIndex } from "@/lib/format";
import { CaseShell } from "../case-shell";
import { Footer, Main, Next } from "../chrome";
import { groupName } from "../plan-bits";
import { ButtonLink, Country, Panel, PanelHead, Spinner } from "../ui";

// Step 1 for an existing case: what the engine read. A case is not edited in
// place; "Start a new case" copies nothing and keeps this one intact.
export function CaseProduct() {
  return (
    <CaseShell tab="product">
      {({ id, c }) => (
        <>
          <Main className="flex max-w-[1240px] flex-col gap-6">
            <div>
              <p className="eyebrow">Step 1 · Product</p>
              <h1 className="mt-1 text-xl tracking-[-0.04em]">{c.product?.name || "Pasted description"}</h1>
              {c.product?.description && <p className="duo mt-1 max-w-[80ch] text-sm text-dim">{c.product.description}</p>}
            </div>
            {c.product?.components?.length ? (
              <Panel>
                <PanelHead right={`${c.product.plants?.length ?? 0} plants`}>Parts and plants</PanelHead>
                <ul className="grid gap-x-8 gap-y-1 text-[13px] sm:grid-cols-2">
                  {c.product.components.map((p, i) => (
                    <li key={i} className="flex justify-between gap-3 border-b border-line py-1.5">
                      <span>{p.name}</span>
                      <span className="text-dim">
                        <Country name={p.made_in} /> {p.group ? `· ${groupName(p.group)}` : ""}
                      </span>
                    </li>
                  ))}
                </ul>
              </Panel>
            ) : null}
            <Panel>
              <PanelHead>Facts the engine read</PanelHead>
              <p className="duo whitespace-pre-wrap text-[13px] leading-relaxed">{c.facts}</p>
            </Panel>
          </Main>
          <Footer>
            <Next>See the likely origin and the earlier rulings behind it.</Next>
            <div className="ml-auto">
              <ButtonLink href={`/cases/${encodeURIComponent(id)}/origin`}>Likely origin →</ButtonLink>
            </div>
          </Footer>
        </>
      )}
    </CaseShell>
  );
}

// /cases/{id} reopens the case on its own step.
export function CaseRedirect() {
  const { id } = useParams<{ id: string }>();
  return (
    <CaseShell tab="origin">
      {({ c }) => <Go to={`/cases/${encodeURIComponent(id)}/${["product", "product", "origin", "plan", "draft"][stepIndex[c.step ?? "origin"] ?? 2]}`} />}
    </CaseShell>
  );
}

function Go({ to }: { to: string }) {
  const router = useRouter();
  useEffect(() => router.replace(to), [router, to]);
  return (
    <Main>
      <Spinner label="Opening the case…" />
    </Main>
  );
}
