package oblodai

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/oblodai/oblodai-go/v2/internal/fixtures"
)

// Documentation is part of the contract: a method that tells a caller to branch on an error code
// must name a code the core actually has. A typo here sends someone chasing a branch that never
// fires — and the code they should have handled goes unhandled.

// codeToken matches a family.reason token in a doc comment.
var codeToken = regexp.MustCompile(`\b[a-z][a-z0-9_]*\.[a-z][a-z0-9_]+\b`)

// clientOwnedFamilies are raised by this package, not by the core, so they are not in the
// catalogue: they are asserted by the constants in errors.go instead.
var clientOwnedFamilies = map[string]bool{"sdk": true, "transport": true, "webhook": true}

func TestErrorCodesNamedInMethodDocsExist(t *testing.T) {
	catalogue := map[string]bool{}
	for _, code := range fixtures.LoadContract(t).ErrorCodes {
		catalogue[code] = true
	}
	if len(catalogue) == 0 {
		t.Fatal("the contract carries no error codes")
	}

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing the package: %v", err)
	}
	documented := 0
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			if strings.HasSuffix(name, "_test.go") {
				continue
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Doc == nil {
					continue
				}
				for _, code := range codesIn(fn.Doc.Text()) {
					documented++
					if !catalogue[code] {
						t.Errorf("%s: the doc names %q, which the contract's error catalogue does not have",
							fn.Name.Name, code)
					}
				}
			}
		}
	}
	if documented < 40 {
		t.Fatalf("only %d documented error codes were found; the money-moving methods must list theirs", documented)
	}

	// The check has to be able to fail.
	if got := codesIn("Errors worth branching on: payout.made_up_code, payout.frozen."); len(got) != 2 {
		t.Fatalf("codesIn = %v, want both codes", got)
	}
	if catalogue["payout.made_up_code"] {
		t.Fatal("the catalogue cannot contain an invented code")
	}
}

// codesIn reads the error codes a doc comment tells callers to branch on: everything after an
// "Errors worth branching on:" line, up to the end of that paragraph.
func codesIn(doc string) []string {
	var out []string
	inList := false
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Errors worth branching on") {
			inList = true
		} else if trimmed == "" {
			inList = false
		}
		if !inList {
			continue
		}
		for _, token := range codeToken.FindAllString(trimmed, -1) {
			family, _, _ := strings.Cut(token, ".")
			if clientOwnedFamilies[family] {
				continue
			}
			out = append(out, token)
		}
	}
	return out
}
