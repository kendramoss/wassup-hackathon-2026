import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar } from "@/components/op/chrome";
import { PlanStep } from "@/components/op/screens/plan-step";

export const metadata: Metadata = { title: "Plan" };

export default function Page() {
  return (
    <>
      <AppBar active="cases" />
      <Suspense>
        <PlanStep />
      </Suspense>
    </>
  );
}
