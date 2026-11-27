import type { Metadata } from "next";
import { Instrument_Sans } from "next/font/google";
import { BusinessStory } from "@/components/op/business/business-story";

// The business view: the same product told as a seven-chapter story for the
// people who pay for and plan the move. The site as built (/) is the
// technical view; the toggle in both top bars switches between them.
const instrument = Instrument_Sans({ subsets: ["latin"], variable: "--font-instrument", display: "swap" });

export const metadata: Metadata = {
  title: "For business",
  description: "Find ways to pay less import tax. Where your parts are made decides the tax; UndeLabs shows which parts to move and what it saves.",
};

export default function Page() {
  return (
    <div className={instrument.variable}>
      <BusinessStory />
    </div>
  );
}
