import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar, PageLoading } from "@/components/op/chrome";
import { CaseProduct } from "@/components/op/screens/case-product";

export const metadata: Metadata = { title: "Product" };

export default function Page() {
  return (
    <>
      <AppBar />
      {/* #685: phone tap targets and type (globals.css) */}
      <div className="touch44 contents">
        <Suspense fallback={<PageLoading label="Loading the case…" />}>
          <CaseProduct />
        </Suspense>
      </div>
    </>
  );
}
