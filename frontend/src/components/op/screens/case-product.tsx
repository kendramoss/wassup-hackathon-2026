"use client";

import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { useEffect } from "react";
import { stepIndex } from "@/lib/format";
import { CaseShell } from "../case-shell";
import { Footer, Main, Next } from "../chrome";
import { PartsByGroup } from "../parts-by-group";
import { ButtonLink, Fold, Panel, Spinner } from "../ui";

// Step 1 for an existing case: what the engine read. A case is not edited in
// place; "Start a new case" copies nothing and keeps this one intact.
export function CaseProduct() {
  return (
    <CaseShell tab="product">
      {({ id, c }) => (
        <>
          <Main className="flex max-w-[1240px] flex-col gap-6">
            <div>
              <p className="eyebrow">Step 1 of 4 · Product</p>
              <h1 className="mt-1 text-xl tracking-[-0.04em]">{c.product?.name || "Pasted description"}</h1>
              {c.product?.description && <p className="duo mt-1 max-w-[80ch] text-sm text-dim">{c.product.description}</p>}
            </div>
            {c.product?.components?.length ? (
              <Panel>
                <PartsByGroup product={c.product} origin={c.prediction?.origin_country} />
              </Panel>
            ) : null}
            <div>
              <Fold title="Facts the engine read" open={!c.product?.components?.length}>
                <p className="duo whitespace-pre-wrap text-[13px] leading-relaxed">{c.facts}</p>
              </Fold>
            </div>
            <p className="text-[13px] text-dim">
              A case isn&apos;t edited in place.{" "}
              <Link href={`/cases/new?from=${encodeURIComponent(id)}`} className="text-ok underline underline-offset-2">
                {c.product?.plants?.length ? "Start a new case from this one" : "Add parts and plants on a new case"}
              </Link>
            </p>
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
