import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar, PageLoading } from "@/components/op/chrome";
import { ChecksView } from "@/components/op/screens/checks-view";

export const metadata: Metadata = { title: "All checks" };

export default function Page() {
  return (
    <>
      <AppBar />
      {/* #685: phone tap targets and type (globals.css) */}
      <div className="touch44 contents">
        <Suspense fallback={<PageLoading label="Loading the case…" />}>
          <ChecksView />
        </Suspense>
      </div>
    </>
  );
}
