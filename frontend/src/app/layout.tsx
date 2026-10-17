import type { Metadata, Viewport } from "next";
import localFont from "next/font/local";
import { SkipLink } from "@/components/op/skip-link";
import { copy } from "@/lib/copy";
import "./globals.css";

// Paper Mono (SIL OFL 1.1, fonts/PaperMono-OFL.txt), bundled.
const paperMono = localFont({
  variable: "--font-paper-mono",
  display: "swap",
  src: [
    { path: "./fonts/PaperMono-Light.woff2", weight: "300", style: "normal" },
    { path: "./fonts/PaperMono-Regular.woff2", weight: "400", style: "normal" },
    { path: "./fonts/PaperMono-Medium.woff2", weight: "500", style: "normal" },
    { path: "./fonts/PaperMono-SemiBold.woff2", weight: "600", style: "normal" },
  ],
});

export const metadata: Metadata = {
  title: { default: copy.name, template: `%s · ${copy.name}` },
  description: copy.tagline,
  // Link previews (#156); the image is app/opengraph-image.tsx. SITE_URL
  // overrides the base for previews on other hosts.
  metadataBase: new URL(process.env.SITE_URL ?? "https://frontend-guxax2teha-uc.a.run.app"),
  openGraph: { type: "website", siteName: copy.name, title: copy.name, description: copy.tagline },
  twitter: { card: "summary_large_image", title: copy.name, description: copy.tagline },
};

export const viewport: Viewport = { themeColor: "#edf2f9", colorScheme: "light" };

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${paperMono.variable} h-full antialiased`}>
      <body className="flex min-h-full flex-col">
        <SkipLink />
        {children}
      </body>
    </html>
  );
}
