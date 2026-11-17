package pilot

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Structural guard for the exceptional unwind path. Concrete cancellation and
// joins are separately exercised by Publisher tests and the actual helper run.
func TestSeedBorrowClosesInActionScope(t *testing.T) {
	f, e := parser.ParseFile(token.NewFileSet(), "seed_lane.go", nil, 0)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		scope, ok := call.Fun.(*ast.FuncLit)
		if !ok {
			return true
		}
		deferred := false
		actions := map[string]bool{}
		ast.Inspect(scope.Body, func(n ast.Node) bool {
			if d, ok := n.(*ast.DeferStmt); ok {
				ast.Inspect(d.Call, func(n ast.Node) bool {
					c, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					s, ok := c.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					id, ok := s.X.(*ast.Ident)
					if ok && id.Name == "publisher" && s.Sel.Name == "Close" {
						deferred = true
					}
					return true
				})
			}
			if c, ok := n.(*ast.CallExpr); ok {
				if s, ok := c.Fun.(*ast.SelectorExpr); ok {
					if owner, ok := s.X.(*ast.SelectorExpr); ok && owner.Sel.Name == "seedStore" {
						actions[s.Sel.Name] = true
					}
				}
			}
			return true
		})
		if deferred && actions["Publish"] && actions["Reconcile"] && actions["Attest"] {
			found = true
		}
		return true
	})
	if !found {
		t.Fatal("concrete Publisher.Close must be deferred in the invoked core action scope")
	}
}
