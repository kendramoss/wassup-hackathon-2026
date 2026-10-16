import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar } from "@/components/op/chrome";
import { DraftStep } from "@/components/op/screens/draft-step";

export const metadata: Metadata = { title: "Draft for broker" };

export default function Page() {
  return (
    <>
      <AppBar active="cases" />
      <Suspense>
        <DraftStep />
      </Suspense>
    </>
  );
}
