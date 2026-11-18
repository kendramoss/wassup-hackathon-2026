# Technical Audit: Unde Labs (`wassup-hackathon-2026`)

## 1. Executive Summary

**Unde Labs** is a specialized AI-driven trade compliance and supply chain optimization platform developed for the Google Cloud AI Builder Cup 2026 (Manufacturing Track). The system predicts U.S. Customs and Border Protection (CBP) Country of Origin (CoO) determinations under the "substantial transformation" standard, models alternative manufacturing footprints to mitigate tariff exposure (e.g., Section 301 duties), and automates draft ruling requests for customs brokers.

## 2. System Architecture & Backend Go Engine

The backend is implemented as a high-concurrency REST API in **Go**, orchestrated alongside a **Next.js** frontend on **Google Cloud Run**. Rather than relying on single-shot LLM generation, the Go backend enforces a deterministic multi-stage agentic pipeline backed by **Vertex AI (Gemini)** and **Cloud Firestore** vector search.

### Core Engine Components

1. **Ingestion & BOM Parser:**
   * Normalizes unstructured Bill of Materials (BOM) data, component Harmonized Tariff Schedule (HTS) classifications, sub-assembly origins, and manufacturing process descriptions into structured Go structs.

2. **RAG & Precedent Retrieval (`Firestore Vector Search`):**
   * Queries a vector-indexed corpus of **2,790+ historical CBP rulings** (CROSS database).
   * Retrieves rulings matching both component HTS chapters and specific manufacturing operations (e.g., PCBA soldering, CNC machining, injection molding, firmware flashing) to ground the legal reasoning in established precedent.

3. **Origin Inference Engine:**
   * Evaluates the manufacturing pipeline against the CBP *substantial transformation* test (name, character, and use).
   * Outputs a predicted Country of Origin, a confidence calibration tier, legal justification, and direct citations to binding CBP rulings.

4. **Supply Chain Relocation Planner:**
   * Iteratively simulates shifting specific manufacturing steps or sub-assembly sourcing to alternative jurisdictions (e.g., shifting final assembly from China to Vietnam or Mexico).
   * Computes duty-rate deltas based on tariff schedules to identify cost-optimal manufacturing footprints.

5. **Verifier Agent (Self-Correction Loop):**
   * Acts as an adversarial compliance auditor before any recommendation reaches the UI.
   * Cross-checks proposed supply chain modifications against negative CBP precedents (e.g., rulings where minor assembly or packaging was deemed *insufficient* to confer new origin) to prevent hallucinated tariff-engineering strategies.

## 3. Evaluation Methodology (`eval/RESULTS.md`)

To prevent data leakage and accurately measure real-world generalization, the project isolates training/retrieval data from test data using a strict temporal split.

### Dataset & Experimental Setup

* **Knowledge Base (Retrieval Corpus):** Historical CBP rulings issued prior to 2026 ($N > 2,790$).
* **Holdout Test Set:** **377 unseen CBP rulings** published in 2026.
* **Task:** Given the product description, component origins, and manufacturing narrative extracted from a ruling request, predict the final Country of Origin determined by CBP.

### Benchmark Results

| Model / Strategy | Evaluation Scope | Accuracy | Delta vs. Baseline | 
| ----- | ----- | ----- | ----- | 
| **Heuristic Baseline** *(Most-mentioned country in BOM/narrative)* | All 377 cases | **60.5%** | — | 
| **Unde Labs (All Cases)** | All 377 cases | **90.2%** | **+29.7%** | 
| **Unde Labs (High-Confidence Tier)** | High-confidence subset | **96.0%** | **+35.5%** | 

### Key Analytical Takeaways

* **Overcoming Frequency Bias:** Simple keyword or country-frequency heuristics fail nearly 40% of the time because high-value components often originate in one country while the transformative manufacturing occurs in another. The Go inference engine successfully weighs *process complexity* over raw component count.
* **Effective Confidence Calibration:** The 5.8% accuracy jump between overall predictions (90.2%) and high-confidence predictions (96.0%) demonstrates that the system reliably flags ambiguous edge cases for human customs broker review.

## 4. Strengths, Limitations & Production Readiness

### Key Strengths

* **Precedent-Grounded Reasoning:** Tightly couples LLM reasoning with verifiable CBP ruling citations, directly addressing the explainability requirements of customs compliance.
* **Closed-Loop Verification:** The Verifier Agent mitigates the risk of recommending non-compliant "screwdriver plant" operations that would fail CBP scrutiny.
* **Actionable Output:** Bridges compliance analysis with financial impact (duty savings) and operational execution (automated broker request drafting).

### Limitations & Technical Debt

* **Simplified Cost Modeling:** The relocation planner optimizes strictly around tariff and duty deltas; it does not yet ingest capital expenditure (CapEx), re-tooling lead times, minimum order quantities (MOQs), or regional labor/logistics overhead.
* **Corpus Boundaries:** Currently scoped to U.S. CBP rulings (19 CFR Part 102 / Part 177) and does not incorporate EU Binding Origin Information (BOI) or USMCA Regional Value Content (RVC) sliding-scale rules.
* **Enterprise Hardening:** Built as a hackathon prototype; requires multi-tenant RBAC, SOC2-compliant audit logging, and PII/trade-secret redaction pipelines prior to commercial enterprise onboarding.