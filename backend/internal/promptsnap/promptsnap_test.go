package promptsnap

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite snapshot.json (refused while a prompt changed without a version bump)")

const snapshotFile = "snapshot.json"

// TestSnapshot is the CI guard: every prompt's text must match the committed
// fingerprint for its version.
func TestSnapshot(t *testing.T) {
	cur := Take()
	old, err := Load(snapshotFile)
	if err != nil && !*update {
		t.Fatalf("%v (create it with: go test ./internal/promptsnap -update)", err)
	}
	problems := Diff(old, cur)
	var unbumped, stale []string
	for _, p := range problems {
		if p.Kind == Unbumped {
			unbumped = append(unbumped, p.Msg)
		} else {
			stale = append(stale, p.Msg)
		}
	}
	if len(unbumped) > 0 {
		t.Fatalf("prompt changed without a version bump:\n%s", strings.Join(unbumped, "\n"))
	}
	if *update {
		if err := cur.Write(snapshotFile); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d prompts)", snapshotFile, len(cur.Prompts))
		return
	}
	if len(stale) > 0 {
		t.Fatalf("snapshot.json is out of date; run go test ./internal/promptsnap -update:\n%s", strings.Join(stale, "\n"))
	}
}

func snap(entries map[string]Entry) Snapshot { return Snapshot{Prompts: entries} }

func TestDiff(t *testing.T) {
	base := map[string]Entry{"engine.system": {"engine-v1", "aaa"}, "router.system": {"router-v2", "bbb"}}
	tests := []struct {
		name string
		cur  map[string]Entry
		want []Kind
		msg  string // fragment of the first problem
	}{
		{"unchanged", base, nil, ""},
		{"text changed, version not", map[string]Entry{"engine.system": {"engine-v1", "ccc"}, "router.system": {"router-v2", "bbb"}},
			[]Kind{Unbumped}, "bump the version constant, run the live slice"},
		{"text and version changed", map[string]Entry{"engine.system": {"engine-v2", "ccc"}, "router.system": {"router-v2", "bbb"}},
			[]Kind{Stale}, "moved from engine-v1 to engine-v2"},
		{"version bumped alone", map[string]Entry{"engine.system": {"engine-v2", "aaa"}, "router.system": {"router-v2", "bbb"}},
			[]Kind{Stale}, "moved from"},
		{"prompt added", map[string]Entry{"engine.system": {"engine-v1", "aaa"}, "router.system": {"router-v2", "bbb"}, "x.system": {"x-v1", "ddd"}},
			[]Kind{Stale}, "new prompt x.system"},
		{"prompt removed", map[string]Entry{"engine.system": {"engine-v1", "aaa"}},
			[]Kind{Stale}, "router.system (router-v2) was removed"},
		{"both kinds, name order", map[string]Entry{"engine.system": {"engine-v1", "zzz"}, "router.system": {"router-v3", "yyy"}},
			[]Kind{Unbumped, Stale}, "engine.system"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Diff(snap(base), snap(tt.cur))
			if len(got) != len(tt.want) {
				t.Fatalf("problems = %+v, want kinds %v", got, tt.want)
			}
			for i, p := range got {
				if p.Kind != tt.want[i] {
					t.Errorf("problem %d kind = %v, want %v (%s)", i, p.Kind, tt.want[i], p.Msg)
				}
			}
			if tt.msg != "" && !strings.Contains(got[0].Msg, tt.msg) {
				t.Errorf("message %q, want it to contain %q", got[0].Msg, tt.msg)
			}
		})
	}
}

// A one-character edit to a prompt must change its fingerprint, and every
// prompt the snapshot covers must be non-empty and carry a version.
func TestTakeCoversEveryPrompt(t *testing.T) {
	s := Take()
	for _, name := range []string{"engine.system", "engine.user", "engine.rules", "engine.textile", "engine.textile2",
		"router.system", "clarify.system", "clarify.answers", "verifier.system", "verifier.task", "caseagent.system"} {
		e, ok := s.Prompts[name]
		if !ok || e.Version == "" || len(e.SHA256) != 64 {
			t.Errorf("%s = %+v", name, e)
		}
	}
	for _, p := range Prompts() {
		if strings.TrimSpace(p.Text) == "" {
			t.Errorf("%s renders empty", p.Name)
		}
	}
	if d := Diff(s, Take()); len(d) != 0 {
		t.Fatalf("Take is not deterministic: %+v", d)
	}
	p := Prompts()[0]
	old := snap(map[string]Entry{p.Name: s.Prompts[p.Name]})
	cur := snap(map[string]Entry{p.Name: {Version: p.Version, SHA256: fingerprint(p.Text + ".")}})
	if got := Diff(old, cur); len(got) != 1 || got[0].Kind != Unbumped {
		t.Errorf("edited %s without a bump: %+v", p.Name, got)
	}
}

func TestWriteLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snap.json")
	s := Take()
	if err := s.Write(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if d := Diff(s, got); len(d) != 0 || got.Doc == "" {
		t.Errorf("round trip: %+v", d)
	}
}
