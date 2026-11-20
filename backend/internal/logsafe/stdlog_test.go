package logsafe

import (
	"strings"
	"testing"
)

func TestErrorLogBridgeIsConstantClass(t *testing.T) {
	l, buf, h := newTest(t, DefaultPolicy())
	_ = l
	lg := ErrorLog(h)
	lg.Printf("http: panic serving 10.0.0.1:1234: %s\n%s", sentMsg, "goroutine 1 [running]: "+sentBOM)
	lg.Println("panic: " + sentToken)
	lg.Print("http: TLS handshake error from " + sentEmail)
	out := buf.String()
	assertClean(t, out)
	if strings.Count(out, `"msg":"http server log line"`) != 3 || strings.Count(out, `"class":"stdlog"`) != 3 {
		t.Fatalf("want 3 constant records: %s", out)
	}
	if strings.Contains(out, "panic") || strings.Contains(out, "TLS") || strings.Contains(out, "goroutine") {
		t.Fatalf("original text must never be printed: %s", out)
	}
	if !strings.Contains(out, `"level":"ERROR"`) {
		t.Fatalf("level: %s", out)
	}
}
