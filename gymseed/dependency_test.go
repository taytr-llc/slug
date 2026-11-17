// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// This protects the actual reachable audit/read call graph, not an output
// checksum: metadata must verify existing signatures without producing any.
func TestSeedMetadataHasNoSigningOrProofDependency(t *testing.T) {
	files, e := os.ReadDir(".")
	if e != nil {
		t.Fatal(e)
	}
	graph := map[string][]string{}
	bad := map[string][]string{}
	for _, entry := range files {
		n := entry.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, e := parser.ParseFile(token.NewFileSet(), n, nil, 0)
		if e != nil {
			t.Fatal(e)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch f := call.Fun.(type) {
				case *ast.Ident:
					graph[fn.Name.Name] = append(graph[fn.Name.Name], f.Name)
				case *ast.SelectorExpr:
					if obj, ok := f.X.(*ast.Ident); ok {
						if obj.Name == "ed25519" && f.Sel.Name == "Sign" || obj.Name == "fighterportrait" || f.Sel.Name == "BlockHash" {
							bad[fn.Name.Name] = append(bad[fn.Name.Name], obj.Name+"."+f.Sel.Name)
						}
					}
					graph[fn.Name.Name] = append(graph[fn.Name.Name], f.Sel.Name)
				}
				return true
			})
		}
	}
	for _, root := range []string{"validateRecord", "audit", "Info", "Status", "Record", "History"} {
		seen := map[string]bool{}
		var visit func(string)
		visit = func(n string) {
			if seen[n] {
				return
			}
			seen[n] = true
			if len(bad[n]) > 0 {
				t.Fatalf("%s reaches %s: %v", root, n, bad[n])
			}
			for _, next := range graph[n] {
				visit(next)
			}
		}
		visit(root)
	}
}
