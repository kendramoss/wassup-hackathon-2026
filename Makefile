# Unde Labs. Deploys are handled by .github/workflows/deploy.yml, not here.
GO_DIR := backend
STATICCHECK ?= $(shell command -v staticcheck 2>/dev/null || echo $(HOME)/go/bin/staticcheck)

.PHONY: doctor demo-seed test lint fmt run smoke reproduce verify-data eval-dev eval-test eval-report eval-slice prompt-snapshot eval-summary bundle-corpus upload-corpus refresh rescore-build rescore-run

test: ## unit tests, no network
	cd $(GO_DIR) && go test ./...

lint: ## gofmt, go vet, staticcheck
	@cd $(GO_DIR) && out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	cd $(GO_DIR) && go vet ./...
	cd $(GO_DIR) && $(STATICCHECK) ./...

fmt:
	cd $(GO_DIR) && gofmt -w .

run: ## local server on :8080
	cd $(GO_DIR) && ADMIN_TOKEN=$${ADMIN_TOKEN:-local-dev-admin-token} go run ./cmd/server

smoke: ## one live Vertex call (Application Default Credentials and GOOGLE_CLOUD_PROJECT)
	cd $(GO_DIR) && go run ./cmd/server -smoke

doctor: ## check the tools and LF checkout that make reproduce needs
	@echo "Unde Labs reproduce prerequisites:"
	@ok=1; \
	for t in make shasum go python3; do \
	  if p=$$(command -v $$t 2>/dev/null); then printf '  ok    %-8s %s\n' "$$t" "$$p"; \
	  else printf '  MISS  %-8s install it and re-run make doctor\n' "$$t"; ok=0; fi; \
	done; \
	gv=$$(go version 2>/dev/null | sed -n 's/.*go\([0-9][0-9]*\)\.\([0-9][0-9]*\).*/\1.\2/p'); \
	if [ -n "$$gv" ]; then printf '  info  go       %s (reproduce needs >= 1.27)\n' "$$gv"; fi; \
	pv=$$(python3 -c 'import sys; print("%d.%d" % sys.version_info[:2])' 2>/dev/null); \
	if [ -n "$$pv" ]; then printf '  info  python   %s (reproduce needs >= 3.8)\n' "$$pv"; fi; \
	eol=$$(git ls-files --eol eval/SHA256SUMS 2>/dev/null | awk '{print $$1}'); \
	case "$$eol" in \
	  i/lf) printf '  ok    checkout LF (eval/SHA256SUMS)\n' ;; \
	  *) printf '  MISS  checkout is %s; see .gitattributes (core.autocrlf=input, then re-clone)\n' "$$eol"; ok=0 ;; \
	esac; \
	if [ "$$ok" -eq 1 ]; then echo "Ready: make reproduce"; else echo "Fix the MISS lines, then re-run make doctor"; exit 1; fi

reproduce: ## recompute every published number from committed files: no model calls, no credentials
	@if git ls-files '*.pyc' 2>/dev/null | grep -q .; then echo "tracked Python bytecode — remove it from the index:"; git ls-files '*.pyc'; exit 1; fi
	$(MAKE) verify-data
	cd $(GO_DIR) && go run ./cmd/claimscheck
	cd $(GO_DIR) && go run ./cmd/eval summary -check
	cd $(GO_DIR) && go run ./cmd/corpus supersession -check
	cd $(GO_DIR) && go run ./cmd/corpus holdings -check
	cd $(GO_DIR) && go run ./cmd/corpus alerts -check
	cd $(GO_DIR) && go run ./cmd/corpus alerts-example -check
	cd $(GO_DIR) && go run ./cmd/tariffshift eval -check
	cd $(GO_DIR) && go run ./cmd/tariffshift parts-eval -check
	cd $(GO_DIR) && go run ./cmd/tariffshift eval -without 15,21,40,71,95 -out ../eval/tariffshift/d560 -check
	cd $(GO_DIR) && go run ./cmd/tariffshift parts-eval -without 15,21,40,71,95 -out ../eval/tariffshift/d560/parts -check
	cd $(GO_DIR) && go run ./cmd/tariffshift parts-example -check
	cd $(GO_DIR) && go run ./cmd/tariffshift plans-eval -check
	cd $(GO_DIR) && go run ./cmd/tariffshift plans-example -check
	cd $(GO_DIR) && go run ./cmd/tariffshift plans102-eval -check
	cd $(GO_DIR) && go run ./cmd/tariffshift plans102-eval -series d540 -check
	cd $(GO_DIR) && go run ./cmd/tariffshift plans102-eval -series heldout -check
	cd $(GO_DIR) && go run ./cmd/tariffshift plans102-eval -series heldout2 -check
	python3 -I -B eval/planner/part102_heldout.py freeze . --check
	python3 -I -B eval/planner/part102_heldout.py build . --check
	python3 -I -B eval/planner/part102_heldout.py build2 . --check
	cd $(GO_DIR) && go run ./cmd/tariffshift plans102-example -check
	cd $(GO_DIR) && go run ./cmd/tariffshift ground102-eval -check
	cd $(GO_DIR) && go run ./cmd/tariffshift p102err-single -check
	cd $(GO_DIR) && go run ./cmd/tariffshift p102err-moves -check
	cd $(GO_DIR) && go run ./cmd/tariffshift p102ck-eval -check
	cd $(GO_DIR) && go run ./cmd/tariffshift p102bar -check
	cd $(GO_DIR) && go test -count=1 ./internal/tariffshift -run 'RulesCAJSON|Parity'
	cd $(GO_DIR) && go test -count=1 ./internal/euorigin ./internal/ukorigin -run 'JSON|Coverage|Parity'
	python3 -I -B eval/planner/part102_dev.py select . --check
	python3 -I -B eval/planner/part102_dev.py build . --check
	python3 -I -B eval/tariffshift/chapter_counts.py . --check
	python3 -I -B eval/families/pool102.py . --check
	python3 -I -B eval/families/select102.py . --check
	python3 -I -B eval/families/pool_r3.py . --check
	python3 -I -B eval/families/select_r3.py . --check
	python3 -I -B eval/families/coverage_r3.py . --check
	python3 -I -B eval/families/live_r3.py . --check
	python3 -I -B eval/families/depth_b2.py . check
	python3 -I -B eval/families/depth_b3.py . check
	python3 -I -B eval/planner/build_b2_fixtures.py . --check
	python3 -I -B eval/planner/build_b3_fixtures.py . --check
	python3 -I -B eval/families/familycov_b2.py . --check
	python3 -I -B eval/families/measure_781.py . --check
	python3 -I -B eval/families/measure_850.py . --check
	python3 -I -B eval/families/measure_850.py . --tag f28-852 --check
	python3 -I -B eval/families/measure_850.py . --tag f28-854 --check
	python3 -I -B eval/families/measure_850.py . --tag f28-854 --against f28-852 --check
	python3 -I -B eval/families/depth_b4.py . check
	python3 -I -B eval/families/measure_863.py . --check
	python3 -I -B eval/families/measure_863.py . --tag f29-864 --check
	python3 -I -B eval/accuracy/diagnose_815.py . --check
	python3 -I -B eval/accuracy/measure_815.py . --check
	python3 -I -B eval/accuracy/measure_858.py . --check
	python3 -I -B eval/accuracy/measure_860.py . --check
	python3 -I -B eval/accuracy/measure_868.py . --check
	python3 -I -B eval/accuracy/measure_900.py . --check
	python3 -I -B eval/accuracy/measure_902.py . --check
	python3 -I -B eval/accuracy/measure_904.py . --check
	python3 -I -B eval/accuracy/measure_907.py . --check
	python3 -I -B eval/accuracy/measure_food_independent.py . --self-test
	cd $(GO_DIR) && go run ./cmd/marketeval canada -check
	cd $(GO_DIR) && go run ./cmd/marketeval euuk -check
	cd $(GO_DIR) && go run ./cmd/marketeval mexico -check
	python3 -I -B eval/supersession/impact.py . --check
	python3 -I -B eval/charts/build_evaluation.py . --check
	python3 -I -B eval/charts/build_trust_data.py . --check
	python3 -I -B eval/charts/build_accuracy.py . --check
	python3 -I -B eval/charts/build_repeatability.py . --check
	python3 -I -B eval/charts/build_repeatability_heldout.py . --check
	python3 -I -B eval/planner/summarize.py . --check
	python3 -I -B eval/charts/build_adjudicator.py . --check
	python3 -I -B eval/charts/build_st_checklist.py . --check
	python3 -I -B eval/planner/summarize_addendum.py . --check
	python3 -I -B eval/planner/summarize_repeats.py . --check
	python3 -I -B eval/planner/plan_bar.py . --check
	python3 -I -B eval/planner/plan_stability.py . --check
	python3 -I -B eval/planner/plan_rank.py . --check
	python3 -I -B eval/agent/score.py . --check
	python3 -I -B eval/jp/split.py . --check
	python3 -I -B eval/ca/parse.py --check
	python3 -I -B eval/jp/labels.py . --check
	python3 -I -B eval/jp/labels.py . --fresh --check
	python3 -I -B eval/jp/fresh_twins.py . --check
	python3 -I -B eval/jp/answer_oracle.py . --check
	python3 -I -B eval/jp/test_answer_oracle.py
	python3 -I -B eval/jp/test_refresh.py
	python3 -I -B eval/rescore/test_build_split.py
	python3 -I -B eval/rescore/test_summarize.py
	python3 -I -B eval/rescore/summarize.py . --month 2026-11 --check
	python3 -I -B eval/jp/check_translations.py . --check
	cd $(GO_DIR) && go run ./cmd/rcepeval -check
	cd $(GO_DIR) && go run ./cmd/jpeval bundle -check
	cd $(GO_DIR) && go run ./cmd/jpeval index -check
	cd $(GO_DIR) && go run ./cmd/jpeval summary -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -prompt-version jp-engine-v1 -split dev_eval -tag d712-dev-complete -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -prompt-version jp-engine-v1 -split dev_eval -tag d909-dev -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -prompt-version jp-engine-v1 -split dev_neg -tag d913-neg-v1 -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -prompt-version jp-engine-v2 -split dev_neg -tag d913-neg-v2 -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -prompt-version jp-engine-v2 -split dev_eval -tag d913-dev-v2 -check
	cd $(GO_DIR) && go run ./cmd/jpeval dev-negatives -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -prompt-version jp-engine-v1 -split dev_eval -tag d916-dev -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -prompt-version jp-engine-v1 -split dev_neg -tag d916-neg -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -split fresh_1126 -tag d919-fresh -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -split fresh_1126_notwin -tag d919-notwin -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -split dev_eval_answered -tag d921-dev -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -split fresh_1126_answered -tag d921-fresh -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -split dev_neg_answered -tag d921-neg -check
	python3 -I -B eval/jp/answered_summary.py . --check
	python3 -I -B eval/jp/labels.py . --held-out --confirm-phase2 --check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -prompt-version jp-engine-v1 -split held_out -check
	cd $(GO_DIR) && go run ./cmd/jpeval candidate-summary -prompt-version jp-engine-v1 -split synthetic -check

refresh: ## find new CBP and Japan Customs rulings and check them (D931; network, no model calls): make refresh DATE=YYYY-MM-DD JP_RAW=dir [JP_SEED=dir]
	cd $(GO_DIR) && go run ./cmd/corpus refresh -date $(DATE)
	python3 -I -B eval/jp/refresh.py . $(JP_RAW) $(if $(JP_SEED),--seed $(JP_SEED)) --date $(DATE)

rescore-build: ## label and freeze the month's CBP re-scoring set from a refresh (D931; no model calls): make rescore-build MONTH=YYYY-MM REFRESH=YYYY-MM-DD FROZEN_AT=ISO
	mkdir -p eval/rescore/$(MONTH)/facts
	cd $(GO_DIR) && go run ./cmd/corpus final2-label -index ../eval/refresh/$(REFRESH)/cbp_probe.jsonl -since 2000-01-01 -existing ../eval/data/labels.jsonl,../eval/data/labels_usmca.jsonl,../eval/data/labels_final2.jsonl,../eval/data/labels_usmca_final2.jsonl -manual ../eval/rescore/$(MONTH)/manual.json -facts ../eval/rescore/$(MONTH)/facts -out ../eval/rescore/$(MONTH)/labels.jsonl
	python3 -I -B eval/rescore/build_split.py . --month $(MONTH) --frozen-at $(FROZEN_AT)

rescore-run: ## score production once on the month's frozen set, after its pre-registration is pushed (D931; gateway): make rescore-run MONTH=YYYY-MM
	cd $(GO_DIR) && go run ./cmd/eval run -split monthly -confirm-final -router -tag rescore-$(MONTH) -splits ../eval/rescore/$(MONTH)/splits.json -labels data/labels.jsonl -case-labels ../eval/rescore/$(MONTH)/labels.jsonl -facts data/facts,../eval/rescore/$(MONTH)/facts
	python3 -I -B eval/rescore/summarize.py . --month $(MONTH)

verify-data: ## check committed eval data against eval/SHA256SUMS and the bundled corpus against it
	cd eval && shasum -a 256 -c SHA256SUMS
	cmp eval/data/labels.jsonl backend/data/labels.jsonl
	cmp eval/data/labels_usmca.jsonl backend/data/labels_usmca.jsonl
	@if [ -d data/corpus/facts ]; then diff -rq data/corpus/facts backend/data/facts --exclude=split_meta.jsonl --exclude=.generated_manifest.json --exclude=.bundle_manifest.json && echo "bundled facts match data/corpus/facts"; fi

bundle-corpus: ## copy labels and the built facts corpus into backend/data (baked into the image); removes only bundled files whose source is gone (manifest-owned)
	mkdir -p backend/data/facts
	cp eval/data/labels.jsonl backend/data/labels.jsonl
	cp eval/data/labels_usmca.jsonl backend/data/labels_usmca.jsonl
	cd backend && go run ./cmd/corpus bundle -from ../data/corpus/facts -to data/facts

# legacy name kept: the GCS bucket exists under this name (renaming needs a migration, D-rename).
CORPUS_BUCKET ?= gs://wassup-hackathon-2026-origin-proof-corpus
upload-corpus: ## copy the bundled corpus (labels + facts) to the corpus bucket (docs/SETUP-GCP.md)
	gcloud storage rsync --recursive --delete-unmatched-destination-objects backend/data $(CORPUS_BUCKET)/bundled

eval-dev: ## engine on the dev split (469; resumes, so a finished run just re-prints its summary)
	cd $(GO_DIR) && go run ./cmd/eval run -split dev

eval-test: ## held-out split (377): run once per frozen config; never tune on it
	cd $(GO_DIR) && go run ./cmd/eval run -split final -confirm-final

eval-report: ## reprint the recorded proxy runs (the *-litellm.jsonl inputs stay as history; no model calls)
	cd $(GO_DIR) && go run ./cmd/eval report -in ../eval/results/dev-gemini-4-argon-prec-engine-v1-litellm.jsonl -scope ../eval/results/scope-dev-router-v2.jsonl
	cd $(GO_DIR) && go run ./cmd/eval report -in ../eval/results/final-gemini-4-argon-prec-engine-v1-litellm.jsonl -scope ../eval/results/scope-final-router-v2.jsonl

eval-slice: ## regression slice (D72): 30 dev rulings vs the committed baseline (~60 model calls; also .github/workflows/eval-slice.yml)
	cd $(GO_DIR) && EVAL_SLICE=on go run ./cmd/eval slice

prompt-snapshot: ## rewrite backend/internal/promptsnap/snapshot.json after bumping a prompt version (D72)
	cd $(GO_DIR) && go test ./internal/promptsnap -update

eval-summary: ## regenerate the numbers served by GET /v1/eval/summary (embedded in the server)
	cd $(GO_DIR) && go run ./cmd/eval summary

demo-seed: ## prepare a running server (make run) for recording: every demo case, plans, scenarios, watch, ask, country comparison; checks verdicts repeat (docs/DEMO.md)
	cd $(GO_DIR) && ADMIN_TOKEN=$${ADMIN_TOKEN:-local-dev-admin-token} go run ./cmd/demo seed -out ../eval/results/demo-seed.json

