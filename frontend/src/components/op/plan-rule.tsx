import type { Plan, PlanResult } from "@/lib/api/client";
import { copy } from "@/lib/copy";
import { Term } from "./term";
import { Chip, cx } from "./ui";

// The 19 CFR 102.20 rule check on each plan move (D441). The backend sends
// it only on a Part 102 case, or on a substantial-transformation case whose
// good and parts all carry codes; otherwise nothing shows here. Calm when
// the rule and the precedent check agree or the rule cannot decide; amber
// when they disagree in a way worth a broker's look, like the case's rule
// check panel (D363). Display only.

type PlanRule = NonNullable<Plan["rule_check"]>;

export function planRuleTone(rc: PlanRule): "ask" | undefined {
  return rc.review ? "ask" : undefined;
}

export function PlanRuleChip({ plan, explain }: { plan: Plan; explain?: boolean }) {
  const rc = plan.rule_check;
  if (!rc?.label) return null;
  const P = copy.planRule;
  // The label names 102.20 (or the 102.11 paragraph) itself.
  return (
    <span data-plan-rule={rc.status} className="inline-flex">
      <Chip tone={planRuleTone(rc)} className="whitespace-normal">
        {explain ? <Term explain={P.explain}>{rc.label}</Term> : rc.label}
      </Chip>
    </span>
  );
}

// The review note under a move that the rule and precedent disagree on.
export function PlanRuleNote({ plan, className }: { plan: Plan; className?: string }) {
  const rc = plan.rule_check;
  if (!rc?.review || !rc.review_note) return null;
  return <span className={cx("block text-xs text-ask", className)}>{rc.review_note}</span>;
}

// The caption above the moves table, when the search carries the check.
export function planRuleCaption(result: PlanResult): string | null {
  const all = [result.baseline, ...(result.plans ?? []), ...(result.rejected ?? [])];
  const rc = all.find((p) => p?.rule_check)?.rule_check;
  if (!rc) return null;
  return copy.planRule.caption(rc.applies === "part102");
}
