import type { Metadata } from "next";
import { Instrument_Sans } from "next/font/google";
import { BusinessStory } from "@/components/op/business/business-story";

// The business view: the same product told as a seven-chapter story for the
// people who pay for and plan the move. The site as built (/) is the
// technical view; the toggle in both top bars switches between them.
const instrument = Instrument_Sans({ subsets: ["latin"], variable: "--font-instrument", display: "swap" });

export const metadata: Metadata = {
  title: "For business",
  description: "See the “Made in” country U.S. Customs would likely give your product, which parts to move to change it, and what the move is worth.",
};

export default function Page() {
  return (
    <div className={instrument.variable}>
      <BusinessStory />
    </div>
  );
}
