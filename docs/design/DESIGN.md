# Origin Proof — frontend design decisions

Built step by step (process from Figma "Design systems 102", Disher's front-end design process and Shakuro's frontend architecture guide, scaled to a 2-day build):

1. Goals and principles — **decided**
2. User flow and states — in progress
3. Foundations (colour, type, spacing, elevation, icons, accessibility)
4. Components and layouts (prototype the key screen first, 10-second test)
5. Architecture and build (feature-organised Next.js app, typed API client)

## 1. Goals and principles (decided 16 Oct)

**Audience:** both — judges first (2:45 video, live finals Q&A), the real user underneath (trade-compliance lead at a China-plus-one contract manufacturer).

**Goal:** a judge understands each screen in 10 seconds; a compliance lead finds enough evidence to trust it and send the plan to a broker.

**Principles**
1. **Headline first, evidence one click away.** Each screen leads with one big answer (the origin, the best plan, 89.1%). Rulings, reasoning and full tables sit directly under it or in a side panel.
2. **Confident where measured, gated where not.** Lead with what we measured (89.1%, 98% on auto-accepted cases). Present uncertainty as the safety system — "auto" vs "sent to an expert", "precedent-supported" with a counter-ruling — the way winning entries framed their human approval gates. Caveats sit one click away, never on the headline, and we never claim more than we measured.
3. **Document, not dashboard.** Calm, readable, printable, like a CBP ruling letter. No decorative charts.
4. **One decision per screen.** Input: describe the product. Result: is this the origin? Plan: which change? Results: can I trust it?

**Success checks**
- 10-second test: someone new can say the origin and the recommended change after 10 seconds on each screen.
- Every on-screen number traces to `docs/NUMBERS.md`.

**Background for principle 2:** past Google hackathon winners (Gemini 3 Hackathon: Globot, Aegis; Cloud Run Hackathon) presented confident self-reported numbers and did not foreground limitations, but repeatedly won with human-approval gates and audit trails (`hackathon-research/evidence/idea-finder-mining.json`).
