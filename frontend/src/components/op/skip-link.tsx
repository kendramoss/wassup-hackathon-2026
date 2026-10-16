"use client";

// "Skip to content" that lands on the visible page's <main>. After a client
// navigation Next keeps the previous page mounted but hidden, so a plain
// href="#main" can jump into the hidden copy.
export function SkipLink() {
  return (
    <a
      href="#main"
      onClick={(e) => {
        const main = [...document.querySelectorAll<HTMLElement>("main")].find((m) => m.getClientRects().length > 0);
        if (!main) return;
        e.preventDefault();
        main.tabIndex = -1;
        main.focus();
      }}
      className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-3 focus:z-50 focus:bg-ink focus:px-3 focus:py-2 focus:text-paper"
    >
      Skip to content
    </a>
  );
}
