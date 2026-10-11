package analyzer

import (
	"flag"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

var TestifyMsgAndArgsForbidder = &analysis.Analyzer{
	Name:  "testify_msgandargs_forbidder",
	Doc:   "forbids msgAndArgs in testify assertion functions",
	Run:   runTestifyMsgAndArgs,
	Flags: newTestifyMsgAndArgsFlags(),
}

func newTestifyMsgAndArgsFlags() flag.FlagSet {
	fs := flag.FlagSet{}
	return fs
}

func runTestifyMsgAndArgs(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			var pkgPath string

			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				if ident, ok := fun.X.(*ast.Ident); ok {
					obj := pass.TypesInfo.Uses[ident]
					if pkgName, ok := obj.(*types.PkgName); ok {
						pkgPath = pkgName.Imported().Path()
					}
				}
			default:
				return true
			}

			if !strings.Contains(pkgPath, "testify/assert") && !strings.Contains(pkgPath, "testify/require") {
				return true
			}

			tv, ok := pass.TypesInfo.Types[call.Fun]
			if !ok {
				return true
			}

			var sig *types.Signature
			if ft, ok := tv.Type.(*types.Signature); ok {
				sig = ft
			} else {
				return true
			}

			if !sig.Variadic() {
				return true
			}

			requiredArgCount := sig.Params().Len() - 1

			if len(call.Args) > requiredArgCount {
				pass.Reportf(call.Pos(), "testify assertion function should not include msgAndArgs")
			}

			return true
		})
	}
	return nil, nil
}
