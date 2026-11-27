import type { Metadata } from "next";
import { Instrument_Sans } from "next/font/google";
import { BusinessCheck } from "@/components/op/business/business-check";

// The business view's plain first step (#893): a product name, the country it is
// imported into, and the main parts. It runs the same check as /cases/new, which stays
// the full form for engineers.
const instrument = Instrument_Sans({ subsets: ["latin"], variable: "--font-instrument", display: "swap" });

export const metadata: Metadata = {
  title: "Check your product",
  description: "Name your product, pick the country you import it into, and list the main parts. See the likely country and the import tax.",
};

export default function Page() {
  return (
    <div className={instrument.variable}>
      <BusinessCheck />
    </div>
  );
}
