import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar } from "@/components/op/chrome";
import { ProductStep } from "@/components/op/screens/product-step";

export const metadata: Metadata = { title: "New case" };

export default function NewCasePage() {
  return (
    <>
      <AppBar />
      <Suspense>
        <ProductStep />
      </Suspense>
    </>
  );
}
