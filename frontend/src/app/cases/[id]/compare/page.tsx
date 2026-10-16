import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar } from "@/components/op/chrome";
import { CompareView } from "@/components/op/screens/compare-view";

export const metadata: Metadata = { title: "Compare countries" };

export default function Page() {
  return (
    <>
      <AppBar />
      <Suspense>
        <CompareView />
      </Suspense>
    </>
  );
}
