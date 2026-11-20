package logsafe

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"testing"
)

// guardFindings is the source guard the wiring unit will run over the real
// packages. It is ADDITIONAL proof, never the runtime gate: it reports log
// calls whose message is not a string literal, whose literal is not in the
// known set, and "err" attributes not wrapped in logsafe.Err.
func guardFindings(t *testing.T, src string, known func(string) bool) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		msgIdx := -1
		switch sel.Sel.Name {
		case "InfoContext", "WarnContext", "ErrorContext", "DebugContext":
			msgIdx = 1
		case "Info", "Warn", "Error", "Debug":
			msgIdx = 0
		default:
			return true
		}
		if len(call.Args) <= msgIdx {
			return true
		}
		pos := fset.Position(call.Pos()).String()
		lit, ok := call.Args[msgIdx].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			out = append(out, pos+": message is not a string literal")
		} else if s, _ := strconv.Unquote(lit.Value); !known(s) {
			out = append(out, pos+": message "+lit.Value+" is not in the known set")
		}
		for i := msgIdx + 1; i+1 < len(call.Args); i += 2 {
			k, ok := call.Args[i].(*ast.BasicLit)
			if !ok || k.Kind != token.STRING {
				continue
			}
			if s, _ := strconv.Unquote(k.Value); s == "err" {
				w, ok := call.Args[i+1].(*ast.CallExpr)
				wsel, ok2 := ast.Expr(nil), false
				if ok {
					if se, isSel := w.Fun.(*ast.SelectorExpr); isSel {
						wsel, ok2 = se, true
					}
				}
				if !ok || !ok2 || wsel.(*ast.SelectorExpr).Sel.Name != "Err" {
					out = append(out, pos+": err attribute is not wrapped in logsafe.Err")
				}
			}
		}
		return true
	})
	sort.Strings(out)
	return out
}

func TestASTGuardOnFixtures(t *testing.T) {
	known := DefaultPolicy().KnownMessage
	good := `package p
func f(l L, ctx C, err error){
 l.InfoContext(ctx, "request", "status", 1)
 l.ErrorContext(ctx, "llm call failed", "err", logsafe.Err(err))
}`
	if got := guardFindings(t, good, known); len(got) != 0 {
		t.Fatalf("false positives: %v", got)
	}
	bad := `package p
func f(l L, ctx C, err error, s string){
 l.InfoContext(ctx, s)
 l.InfoContext(ctx, "panic: custom text")
 l.WarnContext(ctx, fmt.Sprintf("x %s", s))
 l.ErrorContext(ctx, "llm call failed", "err", err)
 l.Debug("llm call content", "system", s)
}`
	got := guardFindings(t, bad, known)
	// non-literal x2, unknown literal x2 ("panic: custom text", "llm call content"), raw err x1
	if len(got) != 5 {
		t.Fatalf("want 5 findings, got %d: %v", len(got), got)
	}
}
