#!/bin/sh
# Labels one split of the realistic AD/CVD test (D945): pool, two name screens, two full readers, an adjudicator, the truth. Gateway env in the environment.
# Usage: sh eval/adcvd/real/label_split.sh SPLIT_DIR ORDERS_FILE   (from the repository root; pairs.jsonl and queries.jsonl are rewritten, answers already recorded are kept)
set -e
D="$1"; O="$2"
(cd backend && go run ./cmd/adcvdreal -orders "../$O" -queries "../$D/queries.jsonl" -k 12 -out "../$D/pairs.jsonl")
for m in claude-haiku-5-5 gpt-6.2-sol; do
  python3 -I -B eval/adcvd/real/label.py screen "$D/pairs.jsonl" "$D/facts.jsonl" "$D/screen.jsonl" --model $m --workers 10 &
done
wait
python3 -I -B eval/adcvd/real/stage2.py "$D"
for m in claude-haiku-5-5 gpt-6.2-sol; do
  python3 -I -B eval/adcvd/real/label.py run "$D/pairs.jsonl" "$D/facts.jsonl" "$D/labels.jsonl" --model $m --only "$D/stage2.jsonl" --workers 10 &
done
wait
python3 -I -B eval/adcvd/real/disagree.py "$D" claude-haiku-5-5 gpt-6.2-sol
python3 -I -B eval/adcvd/real/label.py run "$D/pairs.jsonl" "$D/facts.jsonl" "$D/labels.jsonl" --model claude-sonnet-5-5 --only "$D/disagree.jsonl" --adjudicate claude-haiku-5-5,gpt-6.2-sol --workers 6
python3 -I -B eval/adcvd/real/truth.py "$D" --pairs "$D/pairs.jsonl" --labels "$D/labels.jsonl" --a claude-haiku-5-5 --b gpt-6.2-sol --adj claude-sonnet-5-5
