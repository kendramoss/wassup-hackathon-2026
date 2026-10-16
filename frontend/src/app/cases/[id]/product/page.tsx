import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar } from "@/components/op/chrome";
import { CaseProduct } from "@/components/op/screens/case-product";

export const metadata: Metadata = { title: "Product" };

export default function Page() {
  return (
    <>
      <AppBar />
      <Suspense>
        <CaseProduct />
      </Suspense>
    </>
  );
}
