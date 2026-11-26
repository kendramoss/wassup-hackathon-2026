package main

import "testing"

func TestFreshDirOf(t *testing.T) {
	for split, want := range map[string]string{"fresh_1126": "fresh-1126", "fresh_1126_notwin": "fresh-1126", "fresh_1231": "fresh-1231"} {
		if got, ok := freshDirOf(split); !ok || got != want {
			t.Errorf("%s: got %q %v", split, got, ok)
		}
	}
	for _, split := range []string{"fresh_1126_answered", "fresh_", "fresh_12", "dev_eval", "fresh_1126x"} {
		if isFresh(split) {
			t.Errorf("%s should not be a fresh split", split)
		}
	}
}
