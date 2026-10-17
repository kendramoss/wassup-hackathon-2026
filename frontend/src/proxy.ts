import { NextResponse, type NextRequest } from "next/server";
import { VISITOR_COOKIE, isHttps, newVisitorId, validVisitorId, visitorCookieOptions } from "@/lib/visitor";

// Gives a first-time browser its visitor ID on the page load, so the page's
// API calls all carry the same one. Without this, the calls a page fires at
// once each got a different ID and the browser kept the last (D58).
export function proxy(request: NextRequest) {
  if (validVisitorId(request.cookies.get(VISITOR_COOKIE)?.value)) return NextResponse.next();
  const res = NextResponse.next();
  res.cookies.set(VISITOR_COOKIE, newVisitorId(), visitorCookieOptions(isHttps(request)));
  return res;
}

// Pages only: not the API (lib/backend.ts handles it), Next's assets or files.
export const config = {
  matcher: ["/((?!api/|_next/|.*\\..*).*)"],
};
