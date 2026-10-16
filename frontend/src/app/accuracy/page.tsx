import type { Metadata } from "next";
import { Suspense } from "react";
import { AppBar } from "@/components/op/chrome";
import { AccuracyPage } from "@/components/op/screens/accuracy-page";

export const metadata: Metadata = { title: "How accurate is it?" };

export default function Page() {
  return (
    <>
      <AppBar active="accuracy" />
      <Suspense>
        <AccuracyPage />
      </Suspense>
    </>
  );
}
