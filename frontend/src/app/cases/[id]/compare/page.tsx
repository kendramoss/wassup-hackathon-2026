import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar, PageLoading } from "@/components/op/chrome";
import { CompareView } from "@/components/op/screens/compare-view";

export const metadata: Metadata = { title: "Compare countries" };

export default function Page() {
  return (
    <>
      <AppBar />
      {/* #685: phone tap targets and type (globals.css) */}
      <div className="touch44 contents">
        <Suspense fallback={<PageLoading label="Loading the case…" />}>
          <CompareView />
        </Suspense>
      </div>
    </>
  );
}
