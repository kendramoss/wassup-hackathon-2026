package httpapi

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Spec-vs-code checks for the published API and MCP docs (judge panel
// round 16, item 2): the limit classes the docs give must be the ones
// server.go wraps each route in.

// routeClasses parses server.go and returns, for every registered pattern,
// its rate class: the first argument of a.limit or a.rateOnly ("light",
// "heavy", "bulk", "lookup", "basic", "review"), "basic" for a.cheap, and
// "admin" for a.adminOnly.
func routeClasses(t *testing.T) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "HandleFunc" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		p, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		inner, ok := call.Args[1].(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := inner.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch sel.Sel.Name {
		case "cheap":
			out[p] = "basic"
		case "adminOnly":
			out[p] = "admin"
		case "limit", "rateOnly":
			if id, ok := inner.Args[0].(*ast.Ident); ok {
				out[p] = id.Name
			}
		}
		return true
	})
	if len(out) < 35 {
		t.Fatalf("found only %d classed routes in server.go; the parse is broken", len(out))
	}
	return out
}

func readDoc(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{"..", "..", ".."}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// shortName is the name openapi.yaml's limit lists use for a route: its
// last path segment that is not a parameter ("POST /v1/cases/{id}/plan" is
// "plan").
func shortName(pattern string) string {
	segs := strings.Split(strings.Fields(pattern)[1], "/")
	for i := len(segs) - 1; i >= 0; i-- {
		if !strings.HasPrefix(segs[i], "{") {
			return segs[i]
		}
	}
	return ""
}

// listAfter returns the comma-separated names in the first parenthesis
// after marker in text, with the YAML line breaks folded.
func listAfter(t *testing.T, text, marker string) []string {
	t.Helper()
	i := strings.Index(text, marker)
	if i < 0 {
		t.Fatalf("openapi.yaml no longer says %q", marker)
	}
	rest := text[i+len(marker):]
	open, end := strings.Index(rest, "("), strings.Index(rest, ")")
	if open < 0 || end < open {
		t.Fatalf("no list after %q", marker)
	}
	var out []string
	for _, n := range strings.Split(strings.Join(strings.Fields(rest[open+1:end]), " "), ",") {
		out = append(out, strings.TrimSpace(n))
	}
	sort.Strings(out)
	return out
}

// The light and heavy route lists at the top of openapi.yaml name exactly
// the model routes server.go wraps in a.limit(light, ...) and
// a.limit(heavy, ...). Five light routes were missing (judge panel round 16).
func TestOpenAPILimitListsMatchServer(t *testing.T) {
	spec := readDoc(t, "docs", "api", "openapi.yaml")
	want := map[string][]string{}
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	for p, c := range routeClasses(t) {
		// Only routes behind a.limit call the model; rateOnly(light) routes
		// (plan estimate, audit reads) are named separately in the spec.
		if (c == "light" || c == "heavy") && strings.Contains(string(src), strconv.Quote(p)+", a.limit(") {
			want[c] = append(want[c], shortName(p))
		}
	}
	for c, marker := range map[string]string{"light": "light model\n    routes", "heavy": "heavy\n    ones"} {
		sort.Strings(want[c])
		got := listAfter(t, spec, marker)
		if strings.Join(got, ", ") != strings.Join(want[c], ", ") {
			t.Errorf("openapi.yaml %s list:\n got  %v\n want %v", c, got, want[c])
		}
	}
}

// The first server in openapi.yaml is the hosted proxy, so a spec loaded
// into a client targets the site, not localhost.
func TestOpenAPIFirstServerIsHosted(t *testing.T) {
	spec := readDoc(t, "docs", "api", "openapi.yaml")
	i := strings.Index(spec, "\nservers:\n")
	if i < 0 {
		t.Fatal("openapi.yaml has no servers block")
	}
	first := strings.SplitN(spec[i+len("\nservers:\n"):], "\n", 2)[0]
	if strings.TrimSpace(first) != "- url: https://undelabs.dev/api" {
		t.Errorf("first server is %q, want https://undelabs.dev/api", first)
	}
	if !strings.Contains(spec, "- url: http://localhost:8080") {
		t.Error("the local server is gone from openapi.yaml")
	}
}

// Each tool's limit class in docs/MCP.md is its REST route's class in
// server.go. Three read tools were marked "none" though the routes are
// basic (120 a minute); none is unlimited.
func TestMCPToolLimitClassesMatchServer(t *testing.T) {
	doc := readDoc(t, "docs", "MCP.md")
	classes := routeClasses(t)
	row := regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\| `([A-Z]+) ([^`?]+)[^`]*`[^|]*\\| ([^|]+) \\|")
	words := map[string]string{"light": "light", "heavy": "heavy", "basic": "basic", "lookup": "ruling lookup"}
	n := 0
	for _, m := range row.FindAllStringSubmatch(doc, -1) {
		tool, route, class := m[1], m[2]+" "+m[3], strings.TrimSpace(m[4])
		n++
		c, ok := classes[route]
		if !ok {
			t.Errorf("%s: MCP.md names %q, which server.go does not register", tool, route)
			continue
		}
		if !strings.HasPrefix(class, words[c]) {
			t.Errorf("%s: MCP.md gives limit class %q, but %s is %s in server.go", tool, class, route, c)
		}
	}
	if n < 14 {
		t.Fatalf("read %d tool rows from MCP.md, want at least 14; the parse is broken", n)
	}
}

// docs/MCP.md lists every rate and budget error code a tool can pass on
// from the routes it calls.
func TestMCPDocListsLimitErrorCodes(t *testing.T) {
	doc := readDoc(t, "docs", "MCP.md")
	i := strings.Index(doc, "## Limits")
	if i < 0 {
		t.Fatal("MCP.md has no Limits section")
	}
	limits := doc[i:]
	for _, code := range []string{"rate_limited", "too_many_in_flight", "quota_exceeded", "client_budget_reached", "budget_reached"} {
		if !strings.Contains(limits, "`"+code+"`") {
			t.Errorf("MCP.md Limits does not name `%s`", code)
		}
	}
	if strings.Contains(doc, "tens of seconds") {
		t.Error(`MCP.md still says plan search takes "tens of seconds"; it often takes minutes`)
	}
}
