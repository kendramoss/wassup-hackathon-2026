import type { NextConfig } from "next";

const isDev = process.env.NODE_ENV === "development";

// Content Security Policy without nonces (node_modules/next/dist/docs, guide
// "content-security-policy", section "Without Nonces"): nonces would force
// every page to render per request, which cacheComponents and partial
// prerendering rule out. Google sign-in loads its script, frames and styles
// from accounts.google.com/gsi. Shipped as Report-Only (D137, D220) until
// D270: enforced once the app's pages showed no violations and the Google
// button's script and frame loaded under the enforced policy in a local
// production build.
const csp = [
  "default-src 'self'",
  `script-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/client${isDev ? " 'unsafe-eval'" : ""}`,
  "style-src 'self' 'unsafe-inline' https://accounts.google.com/gsi/style",
  // The signed-in avatar is Google's profile picture (sign-in.tsx).
  "img-src 'self' blob: data: https://lh3.googleusercontent.com",
  "font-src 'self'",
  "connect-src 'self' https://accounts.google.com/gsi/",
  "frame-src https://accounts.google.com/gsi/",
  "object-src 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join("; ");

// Response headers for every page and /api response (D137). The camera is
// allowed for this site only (BOM photos); microphone and location are off.
const securityHeaders = [
  { key: "Strict-Transport-Security", value: "max-age=31536000; includeSubDomains" },
  { key: "X-Content-Type-Options", value: "nosniff" },
  { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
  { key: "X-Frame-Options", value: "DENY" },
  { key: "Permissions-Policy", value: "camera=(self), microphone=(), geolocation=()" },
  { key: "Content-Security-Policy", value: csp },
];

const nextConfig: NextConfig = {
  output: "standalone",
  // No x-powered-by header (judge panel round 2, D220): it names the
  // framework and helps no visitor.
  poweredByHeader: false,
  cacheComponents: true,
  partialPrefetching: true,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
  async headers() {
    // The broker review page holds a secret token (in the URL fragment, which
    // is never sent, and then in memory): no referrer leaves it and it is
    // never indexed. /review/:path* is only ever a 404 (tokens are never
    // accepted in a path) and gets the same headers. Listed after the
    // site-wide rule, so its Referrer-Policy wins here (the last match for a
    // header key applies).
    return [
      { source: "/:path*", headers: securityHeaders },
      {
        source: "/review",
        headers: [
          { key: "Referrer-Policy", value: "no-referrer" },
          { key: "X-Robots-Tag", value: "noindex, nofollow" },
        ],
      },
      {
        source: "/review/:path*",
        headers: [
          { key: "Referrer-Policy", value: "no-referrer" },
          { key: "X-Robots-Tag", value: "noindex, nofollow" },
        ],
      },
    ];
  },
};

export default nextConfig;
