# Technical & Operational Audit: Unde Labs (`wassup-hackathon-2026`)

**Date:** 18 Nov 2026  
**Scope:** Public repository structure, architecture, evaluation benchmarks, issues, and merged pull requests (up to PR #271; code-level items not directly inspected are marked *verify*).  
**Project:** Unde Labs (formerly Origin Proof) — Google Cloud AI Builder Cup 2026 (Manufacturing Track).

---

## 1. Executive Summary

**Unde Labs** is an AI-driven trade compliance and supply-chain optimisation platform built for the Google Cloud AI Builder Cup 2026 (Manufacturing theme). It predicts U.S. Customs and Border Protection (CBP) Country of Origin (CoO) determinations under the "substantial transformation" standard, models alternative manufacturing footprints to mitigate tariff exposure (such as Section 301 duties, e.g., ~41.7% vs. ~16.7% on a demo heat gun), and generates draft ruling requests for licensed customs brokers.

| Audit Area | Rating | Summary Notes |
|---|---|---|
| **Product & Problem Fit** | Strong | Addresses a concrete manufacturing pain point with quantified tariff savings. |
| **Evaluation** | Good (Refreshable) | Tested on 377 held-out 2026 CBP rulings; planner lacks post-cutoff live holdings. |
| **Security & Access** | Fair | Strong rate-limiting and upload bounds, but `CASE_OWNERSHIP` ships disabled. |
| **CI/CD & Deploys** | Fair | Health-checked Cloud Run deploys with rollback, though a PR merged with failing CI. |
| **Testing** | Fair | Go race tests, `vet`, `staticcheck`, and Playwright/axe checks; some AI paths tested in prod. |
| **Repo Hygiene** | Needs Work | Legacy artifacts from an earlier chat-agent pivot and mixed naming (`Origin Proof` vs. `Unde Labs`). |
| **Production Readiness** | Prototype | Requires enterprise authentication, tenant isolation, and cost/feasibility modeling. |

---

## 2. System Architecture & Backend Go Engine

The backend is implemented as a high-concurrency REST API in **Go**, paired with a **Next.js** frontend on **Google Cloud Run**. Rather than relying on single-shot LLM generation, the Go backend enforces a deterministic multi-stage agentic pipeline backed by **Vertex AI (Gemini)** and **Cloud Firestore** vector search.

### Core Pipeline Components

1. **Ingestion & BOM Parser:**
   * Normalises unstructured Bill of Materials (BOM) data (CSV/Excel), Harmonised Tariff Schedule (HTS) classifications, sub-assembly origins, and manufacturing process narratives into structured Go structs.
2. **RAG & Precedent Retrieval (`Firestore Vector Search`):**
   * Queries a vector-indexed corpus of **2,790+ historical CBP rulings** (CROSS database) matching component HTS chapters and manufacturing operations (e.g., PCBA soldering, CNC machining, injection molding, firmware flashing) to ground the legal reasoning in established precedent.
3. **Origin Inference Engine:**
   * Evaluates the manufacturing pipeline against the CBP *substantial transformation* test (name, character, and use) to output a predicted Country of Origin, a confidence calibration tier, legal justification, and direct ruling citations.
4. **Supply Chain Relocation Planner:**
   * Iteratively simulates shifting manufacturing steps or sub-assembly sourcing across plants (e.g., China to Thailand or Vietnam) and calculates duty-rate deltas to identify cost-optimal manufacturing footprints.
5. **Verifier Agent (Self-Correction Loop):**
   * Acts as an adversarial compliance auditor before recommendations reach the UI, cross-checking proposed moves against negative CBP precedents (where minor assembly failed to confer new origin) to prevent hallucinated tariff-engineering strategies.

---

## 3. Evaluation Methodology (`eval/RESULTS.md`)

To prevent data leakage and measure real-world generalisation, the system uses a strict temporal split between its retrieval corpus and evaluation holdout set:

* **Knowledge Base (Retrieval Corpus):** Historical CBP rulings issued prior to 2026 ($N > 2,790$).
* **Holdout Test Set:** **377 unseen CBP rulings** published between January and September 2026.
* **Task:** Given the product description, component origins, and manufacturing narrative extracted from a ruling request, predict the final Country of Origin determined by CBP.

### Benchmark Results

| Model / Strategy | Evaluation Scope | Accuracy | Delta vs. Baseline |
|---|---|---|---|
| **Heuristic Baseline** *(Most-mentioned country in BOM/narrative)* | All 377 cases | **60.5%** | — |
| **Unde Labs (All Cases)** | All 377 cases | **90.2%** | **+29.7%** |
| **Unde Labs (High-Confidence Tier)** | ~80% confident subset | **96.0%** | **+35.5%** |

### Key Analytical Takeaways

* **Overcoming Frequency Bias:** Keyword frequency heuristics fail nearly 40% of the time because high-value components often originate in one country while transformative manufacturing occurs in another. The inference engine weighs *process complexity* over raw component count.
* **Confidence Calibration:** The 5.8% accuracy lift on the high-confidence tier confirms that ambiguous edge cases are reliably flagged for human broker review.

---

## 4. Technical & Operational Findings

### High Priority

* **H1. Any visitor can see other visitors' cases.**
  * `GET /v1/cases` returns every pinned case plus the 200 newest other cases (PR #140). Case ownership by `X-Visitor-ID` exists but ships disabled (`CASE_OWNERSHIP=off`). Users enter product structure, supplier plants, and costs, which are commercially sensitive.
  * **Action:** Turn `CASE_OWNERSHIP` on, or restrict the list to pinned demos plus the caller's own cases. Replace client-supplied `X-Visitor-ID` headers with authenticated sign-in before real customer data goes in. *Verify* whether non-pinned cases can be read or written by ID without ownership.
* **H2. Branch protection does not block failing CI.**
  * PR #252 merged with the backend `gofmt` check failing. The deploy failed and production stayed on the previous revision (fixed in #254). Rollback worked, but `main` was broken.
  * **Action:** Require status checks (`gofmt`, `vet`, tests, frontend build) before merge on `main`.

### Medium Priority

* **M1. The planner can now be tested properly.**
  * The README notes the planner is the least tested part because a clean test required rulings published after 25 Sep 2026, and none with an origin holding existed on 18 Oct.
  * **Action:** Pull CBP rulings from 26 Sep onward with origin holdings and re-run the planner evaluation. Publish the result alongside the 377-ruling benchmark.
* **M2. Features tested first in production.**
  * PR #240 (AI product-family check) was merged without testing against Vertex locally; the author planned to test it in production after deploy.
  * **Action:** Use the staging service (`origin-proof-api-staging`) or a tagged no-traffic revision for model-path checks before shifting traffic.
* **M3. Cloud Run surface area & cost.**
  * Issue #148 found six services with `ingress all`. Two public extras were deleted; staging services remain. The backend also runs with `minScale: 1` by design (D59).
  * **Action:** *Verify* that staging services are private (IAM) or have scale-to-zero and budget alerts, and confirm the always-on cost is intended after judging ends.
* **M4. Legal / advice framing.**
  * HTS suggestions carry a "suggestion only — confirm with a licensed broker" note (#172). Origin verdicts and plans carry the same compliance risk.
  * **Action:** *Verify* a similar disclaimer appears on origin verdicts, relocation plans, and draft ruling requests, not only HTS suggestions.

### Low Priority

* **L1. Leftovers from the pivot.**
  * The repo began as an auto-deploy chat agent (issues #2, #14, #17: in-memory sessions, `/api/chat/stream`, `get_time` tool) before pivoting on 16 Oct.
  * **Action:** *Verify* and remove dead chat-agent code, routes, and workflows, or move them out of the main tree.
* **L2. Naming alignment.**
  * Repo is `wassup-hackathon-2026`, legacy product name is `Origin Proof`, current product/team is `Unde Labs`, and GCP project is `wassup-hackathon-2026`.
  * **Action:** Add a mapping note in `README.md`, update the GitHub repository "About" sidebar description, and clean up remaining `ORIGIN_PROOF_*` fallback variables once migrated.
* **L3. Local developer onboarding.**
  * Backend exits without application-default credentials (#146, fixed in #163 with a printed hint and README note).
  * **Action:** Consider a `LLM_PROVIDER=fake` default for local dev so `/accuracy` and the UI work out of the box without GCP credentials.
* **L4. Submission checklist (#54).**
  * The "Problem statement and category declared" box appeared unticked in the issue body.
  * **Action:** Confirm it is complete and close #54.

---

## 5. Strengths, Limitations & Production Roadmap

### What Is Done Well

* **Precedent-Grounded Reasoning & Held-Out Evaluation:** Tightly couples LLM reasoning with verifiable CBP ruling citations and validates performance against 377 unseen 2026 rulings.
* **Abuse & Input Controls:** Demo cases are read-only for visitors; Excel uploads are bounded (unzip limits, 1,000 rows × 64 cols); per-client rate limits group IPv6 by `/64`; separate lookup budgets isolate CROSS ruling fetches (#132, #140).
* **Resilient Caching & Constrained Output:** Failed CROSS lookups are not cached (#140), and the product-family check enforces a JSON-schema enum and rejects invented families with unit tests (#240).
* **Safe Deploys & UI Discipline:** New Cloud Run revisions receive zero traffic until `/health` passes with automatic rollback (#14, #33); state persists in Firestore (#37/#47); UI issues are audited systematically across widths 320–1440 in light/dark modes (#83, #89, #91).

### Technical Debt & Scope Limitations

* **Simplified Cost Modeling:** The relocation planner optimises strictly around tariff and duty deltas; it does not yet ingest capital expenditure (CapEx), re-tooling lead times, minimum order quantities (MOQs), or regional labour/logistics overhead.
* **Corpus Boundaries:** Scoped to U.S. CBP rulings (19 CFR Part 102 / Part 177) and does not incorporate EU Binding Origin Information (BOI) or USMCA Regional Value Content (RVC) sliding-scale rules.
* **Enterprise Hardening:** Requires multi-tenant RBAC, SOC2-compliant audit logging, and PII/trade-secret redaction pipelines prior to commercial enterprise onboarding.

### Recommended Action Order

1. Turn on `CASE_OWNERSHIP` or restrict `GET /v1/cases` (**H1**).
2. Require CI checks (`gofmt`, `vet`, tests, build) on `main` (**H2**).
3. Re-run the planner benchmark on post-25-Sep rulings (**M1**).
4. Lock down or scale down staging services and review always-on Cloud Run cost (**M3**).
5. Standardise the licensed-broker disclaimer across verdicts, plans, and ruling requests (**M4**).
6. Remove chat-agent pivot leftovers and align `Unde Labs` naming across docs and GitHub metadata (**L1, L2**).

---

## 6. Reference Sources

* Repository: https://github.com/kendramoss/wassup-hackathon-2026
* `#2` In-memory sessions: https://github.com/kendramoss/wassup-hackathon-2026/issues/2
* `#14` Health check and rollback: https://github.com/kendramoss/wassup-hackathon-2026/issues/14
* `#17` Playwright smoke tests: https://github.com/kendramoss/wassup-hackathon-2026/issues/17
* `#44` Frontend build: https://github.com/kendramoss/wassup-hackathon-2026/issues/44
* `#54` Submission checklist: https://github.com/kendramoss/wassup-hackathon-2026/issues/54
* `#83`, `#89`, `#91` UI audit issues: https://github.com/kendramoss/wassup-hackathon-2026/issues/83
* `#132` Security fixes (D57): https://github.com/kendramoss/wassup-hackathon-2026/pull/132
* `#140` Security hardening (D58): https://github.com/kendramoss/wassup-hackathon-2026/pull/140
* `#146` Local dev credentials: https://github.com/kendramoss/wassup-hackathon-2026/issues/146
* `#148` Public Cloud Run services: https://github.com/kendramoss/wassup-hackathon-2026/issues/148
* `#172` HTS suggestion: https://github.com/kendramoss/wassup-hackathon-2026/issues/172
* `#240` AI family check: https://github.com/kendramoss/wassup-hackathon-2026/pull/240
* `#254` `gofmt` fix after failed CI merge: https://github.com/kendramoss/wassup-hackathon-2026/pull/254