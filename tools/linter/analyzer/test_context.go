package analyzer

import (
	"flag"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

var TestContextEnforcer = &analysis.Analyzer{
	Name:  "test_context_enforcer",
	Doc:   "enforces t.Context() instead of context.Background()/context.TODO() in test functions",
	Run:   runTestContext,
	Flags: newTestContextFlags(),
}

func newTestContextFlags() flag.FlagSet {
	fs := flag.FlagSet{}
	return fs
}

func runTestContext(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		inspectTestContext(pass, file, false)
	}
	return nil, nil
}

func inspectTestContext(pass *analysis.Pass, root ast.Node, inTest bool) {
	ast.Inspect(root, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			// 関数宣言は外側の t を使えないため、自分の引数だけで判定する
			if node.Body != nil {
				inspectTestContext(pass, node.Body, hasTestingParam(pass, node.Type))
			}
			return false
		case *ast.FuncLit:
			// クロージャは外側の t を使えるため、外側のスコープを引き継ぐ（t.Run の中の func(t *testing.T) 等）
			inspectTestContext(pass, node.Body, inTest || hasTestingParam(pass, node.Type))
			return false
		case *ast.CallExpr:
			if inTest {
				if name, ok := contextRootFuncName(pass, node); ok {
					pass.Reportf(node.Pos(), "use t.Context() instead of context.%s() in test functions", name)
				}
			}
		}
		return true
	})
}

func hasTestingParam(pass *analysis.Pass, funcType *ast.FuncType) bool {
	if funcType.Params == nil {
		return false
	}
	for _, field := range funcType.Params.List {
		if isTestingContextProvider(pass.TypesInfo.TypeOf(field.Type)) {
			return true
		}
	}
	return false
}

func isTestingContextProvider(t types.Type) bool {
	if t == nil {
		return false
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != "testing" {
		return false
	}
	switch named.Obj().Name() {
	case "T", "B", "F", "TB":
		return true
	}
	return false
}

func contextRootFuncName(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "context" {
		return "", false
	}
	switch fn.Name() {
	case "Background", "TODO":
		return fn.Name(), true
	}
	return "", false
}
