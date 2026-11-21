import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar, PageLoading } from "@/components/op/chrome";
import { ScenariosView } from "@/components/op/screens/scenarios-view";

export const metadata: Metadata = { title: "Scenarios" };

export default function Page() {
  return (
    <>
      <AppBar />
      <Suspense fallback={<PageLoading label="Loading the case…" />}>
        <ScenariosView />
      </Suspense>
    </>
  );
}
