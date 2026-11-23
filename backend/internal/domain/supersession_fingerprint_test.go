package domain

import "testing"

func fingerprintOf(t *testing.T, js string) string {
	t.Helper()
	at, _, err := ParseSupersession([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	return SupersessionFingerprint(at)
}

func TestSupersessionFingerprintTracksOnlyEnforcedEdges(t *testing.T) {
	const e1 = `{"ruling":"N1","superseded_by":"H1","date":"2025-01-02","kind":"revoked","quote":"q"}`
	const e2 = `{"ruling":"N2","superseded_by":"H2","date":"2025-03-04","kind":"modified","quote":"q"}`
	base := fingerprintOf(t, `{"version":"v1","edges":[`+e1+`,`+e2+`]}`)
	if base == "" {
		t.Fatal("empty fingerprint")
	}
	same := map[string]string{
		"edge order":        `{"version":"v1","edges":[` + e2 + `,` + e1 + `]}`,
		"version and rule":  `{"version":"v9","rule":"new prose","edges":[` + e1 + `,` + e2 + `]}`,
		"proposed entries":  `{"version":"v1","edges":[` + e1 + `,` + e2 + `],"proposed":[{"ruling":"N3","proposed_by":"H3","kind":"revoked"}]}`,
		"quote and by only": `{"version":"v1","edges":[{"ruling":"N1","superseded_by":"H7","date":"2025-01-02","kind":"modified","quote":"other"},` + e2 + `]}`,
	}
	for name, js := range same {
		if got := fingerprintOf(t, js); got != base {
			t.Errorf("%s changed the fingerprint", name)
		}
	}
	changed := map[string]string{
		"new edge":      `{"version":"v1","edges":[` + e1 + `,` + e2 + `,{"ruling":"N4","superseded_by":"H4","date":"2025-05-06","kind":"revoked"}]}`,
		"dropped edge":  `{"version":"v1","edges":[` + e1 + `]}`,
		"re-dated edge": `{"version":"v1","edges":[{"ruling":"N1","superseded_by":"H1","date":"2025-01-03","kind":"revoked"},` + e2 + `]}`,
	}
	for name, js := range changed {
		if got := fingerprintOf(t, js); got == base {
			t.Errorf("%s did not change the fingerprint", name)
		}
	}
}

func TestEmbeddedSupersessionFingerprintIsReady(t *testing.T) {
	if fp := EmbeddedSupersessionFingerprint(); len(fp) != 64 || fp == SupersessionFingerprint(nil) {
		t.Errorf("embedded fingerprint %q", fp)
	}
}
