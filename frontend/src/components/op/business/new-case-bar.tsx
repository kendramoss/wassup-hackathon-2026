"use client";

// The top bar on /cases/new: the business bar for visitors who came from
// the business view's "full form" links (?via=business), the technical bar
// for everyone else.
import { useSearchParams } from "next/navigation";
import { AppBar } from "../chrome";
import { BusinessBar } from "./business-bar";

export function NewCaseBar() {
  const viaBusiness = useSearchParams().get("via") === "business";
  if (!viaBusiness) return <AppBar active="cases" />;
  return (
    <div className="biz-barwrap">
      <BusinessBar />
    </div>
  );
}
