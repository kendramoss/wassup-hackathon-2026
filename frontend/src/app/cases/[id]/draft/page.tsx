import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar, PageLoading } from "@/components/op/chrome";
import { DraftStep } from "@/components/op/screens/draft-step";

export const metadata: Metadata = { title: "Draft for broker" };

export default function Page() {
  return (
    <>
      <AppBar active="cases" />
      <Suspense fallback={<PageLoading label="Loading the case…" />}>
        <DraftStep />
      </Suspense>
    </>
  );
}
