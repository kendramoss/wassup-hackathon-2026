import type { Metadata } from "next";
import { Suspense } from "react";
import { BrokerReviewPage, BrokerReviewShell } from "@/components/op/screens/broker-review-page";

// A broker opens this from a link the case owner sent. No account. The token
// is read in the browser from the route and sent only as X-Review-Token; the
// page is never indexed and sends no referrer (headers in next.config.ts).
export const metadata: Metadata = {
  title: "Broker review",
  robots: { index: false, follow: false, nocache: true, googleBot: { index: false, follow: false } },
  referrer: "no-referrer",
};

export default function Page() {
  return (
    <BrokerReviewShell>
      <Suspense fallback={<p className="text-sm text-dim">Loading the review…</p>}>
        <BrokerReviewPage />
      </Suspense>
    </BrokerReviewShell>
  );
}
