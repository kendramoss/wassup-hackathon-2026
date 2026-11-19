import { NextResponse, type NextRequest } from "next/server";
import { VISITOR_COOKIE, isHttps, newVisitorId, validVisitorId, visitorCookieOptions } from "@/lib/visitor";

// Google sign-in only works on origins listed on the OAuth client, so the
// addresses that aren't listed send visitors to the one that is (19 Oct).
// frontend-88903435351.us-central1.run.app is listed and is what the deploy
// smoke test calls, so it is left alone.
const canonical = "undelabs.dev";
const moved = new Set(["www.undelabs.dev", "frontend-guxax2teha-uc.a.run.app"]);

// A first-time browser gets its visitor ID on the page response, before the
// page's parallel API calls start. Without this, each cookie-less call mints
// its own ID and the last response overwrites the cookie (D58, #160). The
// #269 canonical-host change dropped this; #307 restores it.
const isPage = (path: string) => !/^\/(api\/|_next\/)/.test(path) && !/\.[^/]+$/.test(path);

export function proxy(req: NextRequest) {
  const host = (req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "").split(":")[0].toLowerCase();
  if (moved.has(host)) {
    const url = new URL(req.nextUrl.pathname + req.nextUrl.search, `https://${canonical}`);
    return NextResponse.redirect(url, 308);
  }
  // Direct API requests keep the fallback in lib/backend.ts; assets need no ID.
  if (!isPage(req.nextUrl.pathname) || validVisitorId(req.cookies.get(VISITOR_COOKIE)?.value)) return NextResponse.next();
  const res = NextResponse.next();
  res.cookies.set(VISITOR_COOKIE, newVisitorId(), visitorCookieOptions(isHttps(req)));
  return res;
}

export const config = {
  matcher: "/:path*",
};
