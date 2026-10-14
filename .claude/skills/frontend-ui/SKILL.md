---
name: frontend-ui
description: House rules and the check loop for any UI work in frontend/ (pages, components, styles, copy). Use before building or changing anything the user sees, together with the frontend-design skill if it's installed.
---

# Frontend UI for this project

The frontend is Next.js 16 with Tailwind 4 in `frontend/`. Read `frontend/AGENTS.md` first: this Next.js version differs from what you know, and its docs are in `node_modules/next/dist/docs/`.

## The brief (these win over any other design guidance)

- **Minimal.** No gradients and no emojis. Icons only when they're functional (they label an action or a status); no decorative icons. Icons live in `frontend/src/components/icons.tsx`.
- **We're building a product, not a brand.** Show the product working instead of describing it, the way Linear and Railway put their real UI in the hero. For us that means the agent's real output: tool calls, deploy steps, rollbacks, and the numbers it reports.
- **Tokens only.** Colours come from the CSS variables in `frontend/src/app/globals.css` (`--bg`, `--fg`, `--muted`, `--subtle`, `--border`, `--accent`, `--danger`, `--success`), used as Tailwind colours (`bg-subtle`, `text-muted`). Never write raw colours in components. If you need a new colour, add a token for light and for dark, both dark blocks included.
- **One accent colour**, used for the primary action and focus, not for decoration.
- **Copy lives in `frontend/src/lib/site.ts`.** Sentence case, active voice, plain words, and say what happens ("Deploy to staging", not "Submit"). Errors say what went wrong and what to do next.
- If the frontend-design skill suggests a bold visual direction, put that boldness into typography, layout and the product demo, never into gradients, glows or ornaments.

## Quality floor

Every change must keep all of these:
- Works at 390px wide with no horizontal scroll.
- Visible keyboard focus, labelled controls, `prefers-reduced-motion` respected.
- Light and dark both readable (WCAG AA).
- Loading, empty and error states for anything that waits on the agent.

## Check loop (do this before saying a UI change is done)

1. Start the dev server: `cd frontend && npm run dev` (needs `BACKEND_URL` set; any value works for pages that don't call the agent).
2. Run `npm run ui-check` (add `-- /some/path` for other pages). It saves full-page screenshots for light/dark × desktop/mobile to `frontend/.ui-check/` and fails on axe violations, console errors or horizontal scroll.
3. **Look at the screenshots** with the Read tool, at least the desktop-light and mobile-dark ones for each changed page. Fix what looks off: alignment, spacing rhythm, contrast, wrapping, things that look generic.
4. Run `npm run lint` and `npx tsc --noEmit`.
5. For a review against detailed interface rules, use the `ui-review` skill.
