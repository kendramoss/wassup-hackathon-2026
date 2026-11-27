import type { Metadata } from "next";
import { Instrument_Sans } from "next/font/google";
import { Suspense } from "react";
import { BusinessAnswers } from "@/components/op/business/business-answers";

// One answer per country after a check for several countries (#945, #932 R1).
const instrument = Instrument_Sans({
  subsets: ["latin"],
  variable: "--font-instrument",
  display: "swap",
});

export const metadata: Metadata = {
  title: "Your answers",
  description:
    "The likely country for your product in each country you import it into.",
};

export default function Page() {
  return (
    <div className={instrument.variable}>
      <Suspense>
        <BusinessAnswers />
      </Suspense>
    </div>
  );
}
