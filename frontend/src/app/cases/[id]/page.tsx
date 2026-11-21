import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar, PageLoading } from "@/components/op/chrome";
import { CaseRedirect } from "@/components/op/screens/case-product";

export const metadata: Metadata = { title: "Case" };

export default function Page() {
  return (
    <>
      <AppBar active="cases" />
      <Suspense fallback={<PageLoading label="Loading the case…" />}>
        <CaseRedirect />
      </Suspense>
    </>
  );
}
