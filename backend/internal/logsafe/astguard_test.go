package logsafe

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// guardFile is the source guard the wiring unit will run over the real
// packages (it is ADDITIONAL proof, never the runtime gate). It reports, for
// log calls (Info/Warn/Error/Debug, their Context forms, Log and LogAttrs):
//   - a message that is not a string literal, or a literal not in the known set;
//   - an "err" attribute (plain key or slog.String/slog.Any constructor) whose
//     value is not a logsafe.Err(...) call;
//   - any x.Error() call anywhere inside the call's arguments.
func guardFile(t *testing.T, fset *token.FileSet, f *ast.File, known func(string) bool) []string {
	t.Helper()
	var out []string
	isErrWrap := func(e ast.Expr) bool {
		c, ok := e.(*ast.CallExpr)
		if !ok {
			return false
		}
		se, ok := c.Fun.(*ast.SelectorExpr)
		return ok && se.Sel.Name == "Err"
	}
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
		case "Log", "LogAttrs":
			msgIdx = 2
		default:
			return true
		}
		if sel.Sel.Name == "Error" && len(call.Args) == 0 {
			return true // err.Error() itself, not a log call
		}
		if len(call.Args) <= msgIdx {
			return true
		}
		pos := fset.Position(call.Pos()).String()
		if lit, ok := call.Args[msgIdx].(*ast.BasicLit); !ok || lit.Kind != token.STRING {
			out = append(out, pos+": message is not a string literal")
		} else if s, _ := strconv.Unquote(lit.Value); !known(s) {
			out = append(out, pos+": message "+lit.Value+" is not in the known set")
		}
		for i := msgIdx + 1; i < len(call.Args); i++ {
			// plain "err", value pairs
			if k, ok := call.Args[i].(*ast.BasicLit); ok && k.Kind == token.STRING && i+1 < len(call.Args) {
				if s, _ := strconv.Unquote(k.Value); s == "err" && !isErrWrap(call.Args[i+1]) {
					out = append(out, pos+": err attribute is not wrapped in logsafe.Err")
				}
			}
			ast.Inspect(call.Args[i], func(m ast.Node) bool {
				c, ok := m.(*ast.CallExpr)
				if !ok {
					return true
				}
				cs, ok := c.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if cs.Sel.Name == "Error" && len(c.Args) == 0 {
					out = append(out, fset.Position(c.Pos()).String()+": raw Error() inside a log call")
				}
				if (cs.Sel.Name == "String" || cs.Sel.Name == "Any") && len(c.Args) == 2 {
					if k, ok := c.Args[0].(*ast.BasicLit); ok {
						if s, _ := strconv.Unquote(k.Value); s == "err" && !isErrWrap(c.Args[1]) {
							out = append(out, fset.Position(c.Pos()).String()+": err attribute constructor is not wrapped in logsafe.Err")
						}
					}
				}
				return true
			})
		}
		return true
	})
	sort.Strings(out)
	return out
}

func guardSource(t *testing.T, src string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	return guardFile(t, fset, f, DefaultPolicy().KnownMessage)
}

func TestASTGuardOnFixtures(t *testing.T) {
	good := `package p
func f(l L, ctx C, err error){
 l.InfoContext(ctx, "request", "status", 1)
 l.ErrorContext(ctx, "llm call failed", "err", logsafe.Err(err))
 l.LogAttrs(ctx, slog.LevelWarn, "request", slog.Any("err", logsafe.Err(err)))
 l.Log(ctx, slog.LevelInfo, "request", "status", 2)
}`
	if got := guardSource(t, good); len(got) != 0 {
		t.Fatalf("false positives: %v", got)
	}
	bad := `package p
func f(l L, ctx C, err error, s string){
 l.InfoContext(ctx, s)
 l.InfoContext(ctx, "panic: custom text")
 l.WarnContext(ctx, fmt.Sprintf("x %s", s))
 l.ErrorContext(ctx, "llm call failed", "err", err)
 l.Debug("llm call content", "system", s)
 l.LogAttrs(r.Context(), slog.LevelWarn, "shared rate limiter unavailable; using this instance's limits",
   slog.String("request_id", id), slog.String("err", err.Error()))
 l.Log(ctx, slog.LevelInfo, s)
 l.InfoContext(ctx, "request", "detail", err.Error())
}`
	got := guardSource(t, bad)
	want := map[string]int{
		"message is not a string literal":             3, // s, Sprintf, Log(s)
		"is not in the known set":                     2, // custom text, llm call content
		"err attribute is not wrapped in logsafe.Err": 1,
		"err attribute constructor is not wrapped":    1,
		"raw Error() inside a log call":               2, // slog.String("err", err.Error()) and "detail", err.Error()
	}
	for sub, n := range want {
		c := 0
		for _, g := range got {
			if strings.Contains(g, sub) {
				c++
			}
		}
		if c != n {
			t.Errorf("%q: got %d findings, want %d (all: %v)", sub, c, n, got)
		}
	}
}

// Informational: the guard runs over the real handler package so the wiring
// unit knows what it must fix. It asserts nothing about the findings (the
// call sites are not wired at this stage) but it must parse and run.
func TestASTGuardRunsOnRealPackagesInformational(t *testing.T) {
	total := 0
	for _, dir := range []string{"../httpapi", "../llm", "../../cmd/server"} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			fset := token.NewFileSet()
			af, err := parser.ParseFile(fset, f, src, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range guardFile(t, fset, af, DefaultPolicy().KnownMessage) {
				total++
				if strings.Contains(g, "limit_shared.go") {
					t.Logf("known wiring target: %s", g)
				}
			}
		}
	}
	t.Logf("guard findings over httpapi, llm, cmd/server at this stage: %d (to be fixed at wiring)", total)
}
