import type { Metadata, Viewport } from "next";
import localFont from "next/font/local";
import { ActionSlotProvider } from "@/components/op/action-bar";
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
};

export const viewport: Viewport = { themeColor: "#edf2f9", colorScheme: "light" };

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${paperMono.variable} h-full antialiased`}>
      <body className="flex min-h-full flex-col">
        <a
          href="#main"
          className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-3 focus:z-50 focus:bg-ink focus:px-3 focus:py-2 focus:text-paper"
        >
          Skip to content
        </a>
        <ActionSlotProvider>{children}</ActionSlotProvider>
      </body>
    </html>
  );
}
