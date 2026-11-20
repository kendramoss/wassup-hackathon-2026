package audit

import (
	"strings"
	"testing"
	"time"
)

func validDraft() Event {
	return Event{Tenant: "ws:ws_0123456789ab", ActorKind: ActorUser, Actor: "a1_" + strings.Repeat("a", 32), Action: CaseUpdate, Phase: PhaseCompletion,
		Outcome: Allowed, Detail: "rename", Object: "case:case_0123456789ab", Route: "PATCH /v1/cases/{id}", RequestID: "0123456789abcdef"}
}

func TestValidateAcceptsADraftAndRejectsFreeText(t *testing.T) {
	if err := validDraft().validateFields(); err != nil {
		t.Fatalf("valid draft rejected: %v", err)
	}
	cases := map[string]func(*Event){
		"unknown action":         func(e *Event) { e.Action = "case.explode" },
		"email as actor":         func(e *Event) { e.Actor = "alice@example.com" },
		"email in object":        func(e *Event) { e.Object = "case:alice@example.com" },
		"name in object":         func(e *Event) { e.Object = "Acme Widget Co" },
		"free text detail":       func(e *Event) { e.Detail = "renamed to Secret Supplier Ltd" },
		"detail on wrong action": func(e *Event) { e.Action = CaseDelete },
		"bad tenant":             func(e *Event) { e.Tenant = "alice@example.com" },
		"raw client request id":  func(e *Event) { e.RequestID = "alice@example.com" },
		"newline in route":       func(e *Event) { e.Route = "GET /v1/x\nadmin" },
		"admin with actor":       func(e *Event) { e.ActorKind = ActorAdmin },
		"user without pseudonym": func(e *Event) { e.Actor = "" },
		"intent denied":          func(e *Event) { e.Phase, e.Outcome, e.Reason = PhaseIntent, Denied, ReasonForbidden },
		"failed without reason":  func(e *Event) { e.Outcome = Failed },
		"allowed with reason":    func(e *Event) { e.Reason = ReasonInternal },
		"unknown reason":         func(e *Event) { e.Outcome, e.Reason = Failed, "database exploded: password=x" },
		"ref on single":          func(e *Event) { e.Phase, e.Ref = PhaseSingle, 4 },
		"target not a pseudonym": func(e *Event) { e.Target = "bob@example.com" },
		"negative suppressed":    func(e *Event) { e.Suppressed = -1 },
		"unknown actor kind":     func(e *Event) { e.ActorKind = "root" },
	}
	for name, mut := range cases {
		e := validDraft()
		mut(&e)
		if err := e.validateFields(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestNormalizeRequestIDNeverStoresClientText(t *testing.T) {
	if got := NormalizeRequestID("0123456789abcdef"); got != "0123456789abcdef" {
		t.Errorf("server id changed: %q", got)
	}
	got := NormalizeRequestID("alice@example.com")
	if !strings.HasPrefix(got, "h_") || len(got) != 18 || strings.Contains(got, "alice") {
		t.Errorf("client id = %q", got)
	}
	if NormalizeRequestID("alice@example.com") != got {
		t.Error("not deterministic")
	}
	if NormalizeRequestID("") != "" {
		t.Error("empty id must stay empty")
	}
	e := validDraft()
	e.RequestID = got
	if err := e.validateFields(); err != nil {
		t.Errorf("normalized id rejected: %v", err)
	}
}

func TestSealChainsAndClampsTime(t *testing.T) {
	first, err := Seal(validDraft(), Head{}, t0)
	if err != nil {
		t.Fatal(err)
	}
	if first.Seq != 1 || first.PrevHash != Genesis || first.Hash != first.ComputeHash() || first.Validate() != nil {
		t.Fatalf("first = %+v", first)
	}
	// A clock that went backwards cannot reorder time inside a tenant.
	second, err := Seal(validDraft(), HeadOf(first), t0.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if second.Seq != 2 || second.PrevHash != first.Hash || !second.Time.Equal(first.Time) {
		t.Fatalf("second = %+v", second)
	}
	// Microsecond precision, so a round trip through Firestore keeps the hash.
	third, _ := Seal(validDraft(), HeadOf(second), t0.Add(time.Hour+123*time.Nanosecond))
	if third.Time.Nanosecond()%1000 != 0 {
		t.Errorf("time not truncated to microseconds: %v", third.Time)
	}
}

func TestHashCoversEveryField(t *testing.T) {
	base, _ := Seal(validDraft(), Head{}, t0)
	muts := map[string]func(*Event){
		"tenant": func(e *Event) { e.Tenant = "ws:ws_ba9876543210" }, "seq": func(e *Event) { e.Seq++ },
		"time": func(e *Event) { e.Time = e.Time.Add(time.Microsecond) }, "actor kind": func(e *Event) { e.ActorKind = ActorAPIKey },
		"actor": func(e *Event) { e.Actor = "a1_" + strings.Repeat("b", 32) }, "target": func(e *Event) { e.Target = "a1_" + strings.Repeat("c", 32) },
		"action": func(e *Event) { e.Action = CaseExport }, "phase": func(e *Event) { e.Phase = PhaseIntent },
		"outcome": func(e *Event) { e.Outcome = Failed }, "reason": func(e *Event) { e.Reason = ReasonInternal },
		"detail": func(e *Event) { e.Detail = "plan" }, "object": func(e *Event) { e.Object = "case:case_ba9876543210" },
		"route": func(e *Event) { e.Route = "DELETE /v1/cases/{id}" }, "request id": func(e *Event) { e.RequestID = "fedcba9876543210" },
		"ref": func(e *Event) { e.Ref = 9 }, "suppressed": func(e *Event) { e.Suppressed = 3 }, "prev": func(e *Event) { e.PrevHash = strings.Repeat("1", 64) },
	}
	for name, m := range muts {
		e := base
		m(&e)
		if e.ComputeHash() == base.Hash {
			t.Errorf("hash ignores %s", name)
		}
	}
}

func TestSensitiveSetIsThePolicy(t *testing.T) {
	want := map[Action]bool{CaseDelete: true, WorkspaceInvite: true, WorkspaceJoin: true, WorkspaceRoleChange: true,
		WorkspaceMemberRemove: true, WorkspaceLeave: true, APIKeyIssue: true, APIKeyDisable: true}
	for _, a := range Actions() {
		if a.Sensitive() != want[a] {
			t.Errorf("%s sensitive=%v, policy says %v", a, a.Sensitive(), want[a])
		}
	}
	if len(Actions()) != 19 {
		t.Errorf("%d actions, the contract lists 19", len(Actions()))
	}
}

func TestObjectRefAcceptsOnlyServerMintedIDs(t *testing.T) {
	for kind, id := range map[string]string{"case": "case_0123456789ab", "workspace": "ws_0123456789ab", "apikey": "0123456789abcdef"} {
		if ref, ok := ObjectRef(kind, id); !ok || ref != kind+":"+id {
			t.Errorf("%s %s = %q %v", kind, id, ref, ok)
		}
	}
	for _, bad := range [][2]string{{"case", "alice-secret"}, {"case", "case_0123456789AB"}, {"case", "case_0123456789abc"}, {"workspace", "case_0123456789ab"},
		{"apikey", "op_secret"}, {"user", "x"}, {"case", "healthcheck_probe"}} {
		if ref, ok := ObjectRef(bad[0], bad[1]); ok {
			t.Errorf("%v accepted as %q", bad, ref)
		}
	}
}
