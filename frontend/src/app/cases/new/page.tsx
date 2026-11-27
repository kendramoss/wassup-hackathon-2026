import type { Metadata } from "next";
import { Suspense } from "react";
import { Instrument_Sans } from "next/font/google";
import { AppBar, PageLoading } from "@/components/op/chrome";
import { ProductStep } from "@/components/op/screens/product-step";
import { NewCaseBar } from "@/components/op/business/new-case-bar";

export const metadata: Metadata = { title: "New case" };

// Instrument Sans for the business bar and intro when the form is opened
// from the business view (?via=business).
const instrument = Instrument_Sans({ subsets: ["latin"], variable: "--font-instrument", display: "swap" });

export default function NewCasePage() {
  return (
    <div className={`${instrument.variable} contents`}>
      <Suspense fallback={<AppBar active="cases" />}>
        <NewCaseBar />
      </Suspense>
      {/* #685: phone tap targets and type (globals.css) */}
      <div className="touch44 contents">
        <Suspense fallback={<PageLoading label="Loading…" />}>
          <ProductStep />
        </Suspense>
      </div>
    </div>
  );
}
