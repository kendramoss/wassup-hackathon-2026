import { NextResponse, type NextRequest } from "next/server";

// Google sign-in only works on origins listed on the OAuth client, so the
// addresses that aren't listed send visitors to the one that is (19 Oct).
// frontend-88903435351.us-central1.run.app is listed and is what the deploy
// smoke test calls, so it is left alone.
const canonical = "undelabs.dev";
const moved = new Set(["www.undelabs.dev", "frontend-guxax2teha-uc.a.run.app"]);

export function proxy(req: NextRequest) {
  const host = (req.headers.get("x-forwarded-host") ?? req.headers.get("host") ?? "").split(":")[0].toLowerCase();
  if (!moved.has(host)) return NextResponse.next();
  const url = new URL(req.nextUrl.pathname + req.nextUrl.search, `https://${canonical}`);
  return NextResponse.redirect(url, 308);
}

export const config = {
  matcher: "/:path*",
};
