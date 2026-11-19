package main

import (
	"errors"
	"strings"
	"testing"
)

func TestCredentialHint(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"adc missing", errors.New("google: could not find default credentials"), true},
		{"other error", errors.New("port already in use"), false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := credentialHint(tc.err)
			if (got != "") != tc.want {
				t.Fatalf("credentialHint(%v) = %q, want hint=%v", tc.err, got, tc.want)
			}
			if tc.want && (!strings.Contains(got, "gcloud auth application-default login") || !strings.Contains(got, "LLM_PROVIDER=fake")) {
				t.Errorf("hint is missing a recovery step: %q", got)
			}
		})
	}
}
