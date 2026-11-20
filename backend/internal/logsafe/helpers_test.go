package logsafe

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

// Synthetic sentinels. None is real data. Each must be absent from the FULL
// output (message, keys, group names and values), not only from values.
const (
	sentEmail    = "zz.user285+leak@example.invalid"
	sentToken    = "tok_285_SYNTHETIC_DO_NOT_USE_0123456789"
	sentTokenRID = "tok285SYNTHETIC0123456789abcdef" // valid request-id SHAPE, token-like
	sentKeyName  = "keyname_ZZ285_SYNTH_supplier"
	sentGroup    = "grpname_ZZ285_SYNTH_bom"
	sentSupplier = "Zzyzx-Synthetic-Supplier-285"
	sentBOM      = "PN-285-SYNTH-0001"
	sentMsg      = "panic: customer Zzyzx-Synthetic-Supplier-285 shipped PN-285-SYNTH-0001"
)

var allSentinels = []string{sentEmail, sentToken, sentTokenRID, sentKeyName, sentGroup, sentSupplier, sentBOM, "Zzyzx", "ZZ285", "285_SYNTH", "zz.user285"}

func assertClean(t *testing.T, out string) {
	t.Helper()
	for _, s := range allSentinels {
		if strings.Contains(out, s) {
			t.Fatalf("sentinel %q leaked in output:\n%s", s, out)
		}
	}
}

// newTest returns a logger over a debug-level JSON handler wrapped by the
// sanitiser, the buffer, and the handler.
func newTest(t *testing.T, p *Policy) (*slog.Logger, *bytes.Buffer, *Handler) {
	t.Helper()
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	h := NewHandler(inner, p)
	return slog.New(h), &buf, h
}

var bg = context.Background()
