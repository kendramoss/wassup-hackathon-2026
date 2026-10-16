import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar } from "@/components/op/chrome";
import { OriginStep } from "@/components/op/screens/origin-step";

export const metadata: Metadata = { title: "Likely origin" };

export default function Page() {
  return (
    <>
      <AppBar />
      <Suspense>
        <OriginStep />
      </Suspense>
    </>
  );
}
