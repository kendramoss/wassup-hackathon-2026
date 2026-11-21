import type { Metadata } from "next";
import { Suspense } from "react";
import { BrokerReviewPage, BrokerReviewShell } from "@/components/op/screens/broker-review-page";

// A broker opens /review#<token> from a link the case owner sent. No account.
// The token is the URL fragment: the browser never sends it to a server, so no
// request log or Referer holds it. The page reads it client-side, clears it
// from the address bar, and sends it only as X-Review-Token. The page is never
// indexed and sends no referrer (headers in next.config.ts).
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
