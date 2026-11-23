import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar, PageLoading } from "@/components/op/chrome";
import { OriginStep } from "@/components/op/screens/origin-step";

export const metadata: Metadata = { title: "Likely origin" };

export default function Page() {
  return (
    <>
      <AppBar />
      {/* #685: phone tap targets and type (globals.css) */}
      <div className="touch44 contents">
        <Suspense fallback={<PageLoading label="Loading the case…" />}>
          <OriginStep />
        </Suspense>
      </div>
    </>
  );
}
