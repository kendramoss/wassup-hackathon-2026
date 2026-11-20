package budget

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// Regression tests for review findings C2, C4, C6, C8 (first written as red proofs against 0452f43).
func TestExternalSpendRaisedMidRunIsKept(t *testing.T) {
	r := newRig(t, 20, true)
	_, err := r.g.Generate(context.Background(), req(10))
	must(t, err)
	b, _ := os.ReadFile(r.shr)
	s := strings.Replace(string(b), `"spent_usd": 0.000`, `"spent_usd": 5.000`, 1)
	must(t, os.WriteFile(r.shr, []byte(s), 0o600))
	_, err = r.g.Generate(context.Background(), req(10))
	must(t, err)
	sh, _ := readShared(r.shr)
	if sh.spentNanos < 5_000_000_000 {
		t.Fatalf("externally raised spend was overwritten with %d", sh.spentNanos)
	}
}

func TestExternalReservedLoweredIsRefused(t *testing.T) {
	r := newRig(t, 20, true)
	set := func(v string) {
		b, _ := os.ReadFile(r.shr)
		s := string(b)
		if strings.Contains(s, "external_reserved_usd") {
			s = regexpReplace(s, v)
		} else {
			s = strings.Replace(s, `"events"`, `"external_reserved_usd": `+v+`,"events"`, 1)
		}
		must(t, os.WriteFile(r.shr, []byte(s), 0o600))
	}
	set("5.000000000")
	_, _ = r.g.Generate(context.Background(), req(10))
	set("0.000000000")
	if _, err := r.g.Generate(context.Background(), req(10)); !errors.Is(err, ErrTamper) {
		t.Fatalf("lowered external reservation was accepted: %v", err)
	}
}

func regexpReplace(s, v string) string {
	i := strings.Index(s, `"external_reserved_usd":`)
	j := i + len(`"external_reserved_usd":`)
	k := j
	for k < len(s) && s[k] != ',' && s[k] != '\n' && s[k] != '}' {
		k++
	}
	return s[:j] + " " + v + s[k:]
}

func TestRunWindowRequired(t *testing.T) {
	r := newRig(t, 20, true)
	r.g.o.RunEndsBy = time.Time{}
	if _, err := r.g.Generate(context.Background(), req(10)); err == nil {
		t.Fatal("a real caller without a run window was accepted")
	}
}

func TestOversizeIsNotARunHalt(t *testing.T) {
	r := newRig(t, 20, true)
	big := req(10)
	big.System = strings.Repeat("a", 100000)
	r.g.Generate(context.Background(), big)
	r.inner.respond = okResp(10, 1)
	if _, err := r.g.Generate(context.Background(), req(10)); err != nil {
		t.Fatalf("a known-unsent oversize refusal halted the run: %v", err)
	}
}
