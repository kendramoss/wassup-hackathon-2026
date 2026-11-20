package logsafe

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The core must stay a stdlib-only leaf: it may not import any app, model,
// domain or store package (acyclic once llm wires it), nor any third party.
func TestCoreImportsStdlibOnly(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	fset := token.NewFileSet()
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		n++
		for _, im := range af.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			first := strings.SplitN(p, "/", 2)[0]
			if strings.Contains(first, ".") || strings.Contains(p, "wassup-hackathon") {
				t.Errorf("%s imports non-stdlib %q", f, p)
			}
		}
	}
	if n == 0 {
		t.Fatal("no source files scanned")
	}
	if _, err := os.Stat("go.mod"); err == nil {
		t.Fatal("package must not carry its own module")
	}
}
