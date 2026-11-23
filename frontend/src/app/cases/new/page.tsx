import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar, PageLoading } from "@/components/op/chrome";
import { ProductStep } from "@/components/op/screens/product-step";

export const metadata: Metadata = { title: "New case" };

export default function NewCasePage() {
  return (
    <>
      <AppBar active="cases" />
      {/* #685: phone tap targets and type (globals.css) */}
      <div className="touch44 contents">
        <Suspense fallback={<PageLoading label="Loading…" />}>
          <ProductStep />
        </Suspense>
      </div>
    </>
  );
}
