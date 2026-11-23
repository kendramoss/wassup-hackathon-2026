import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar, PageLoading } from "@/components/op/chrome";
import { PlanStep } from "@/components/op/screens/plan-step";

export const metadata: Metadata = { title: "Plan" };

export default function Page() {
  return (
    <>
      <AppBar active="cases" />
      {/* #685: phone tap targets and type (globals.css) */}
      <div className="touch44 contents">
        <Suspense fallback={<PageLoading label="Loading the case…" />}>
          <PlanStep />
        </Suspense>
      </div>
    </>
  );
}
