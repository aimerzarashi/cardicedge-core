package analyzer

import (
	"flag"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"strconv"

	"golang.org/x/tools/go/analysis"
)

// サブテストの名前は、識別子のような英小文字のスネークケースにし、兄弟（同じテーブル・同じ親）で重複させない（core#8）。
// Google の Go スタイルガイドに沿う: go test -run で絞り込みやすく、テストの実行器が空白を _ に置き換えて読みにくくならず、
// / が絞り込みの区切りと衝突しないため。参照の api の「正常系/」「異常系/」は / を含むうえ、分類は付いても何を確かめるかが
// 名前に出ないため採らなかった。名前に中身があるかはレビューで見る（中身の無い名前の一覧を持つと育て続けることになるため）。
// 文字列リテラルでない名前（入力から作る t.Run(tt.in, ...) 等。Go の公式の例にもある）は調べない
var TRunNamingEnforcer = &analysis.Analyzer{
	Name:  "trun_naming_enforcer",
	Doc:   "enforces t.Run names to be unique lower_snake_case identifiers",
	Run:   runTRunNaming,
	Flags: flag.FlagSet{},
}

const (
	msgTRunNameFormat     = "t.Run name must be lower_snake_case ([a-z0-9] words joined by _), without spaces, slashes or non-ASCII characters"
	msgTRunNameDuplicated = "t.Run name must be unique among sibling subtests"
)

var tRunNameFormat = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)*$`)

func runTRunNaming(pass *analysis.Pass) (any, error) {
	// 同じ親のテスト（同じ t）の直下での名前の重複を調べる
	seenBy := map[types.Object]map[string]bool{}

	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Run" || !isTTestPointer(pass, sel.X) {
				return true
			}

			switch arg := call.Args[0].(type) {
			case *ast.BasicLit:
				if arg.Kind != token.STRING {
					return true
				}
				name := unquote(arg.Value)
				checkFormat(pass, arg, name)
				if receiver := objectOf(pass, sel.X); receiver != nil {
					if seenBy[receiver] == nil {
						seenBy[receiver] = map[string]bool{}
					}
					if seenBy[receiver][name] {
						pass.Reportf(arg.Pos(), msgTRunNameDuplicated)
					}
					seenBy[receiver][name] = true
				}
			case *ast.SelectorExpr:
				if ident, ok := arg.X.(*ast.Ident); ok {
					checkTableNames(pass, ident, arg.Sel.Name, file)
				}
			}
			return true
		})
	}
	return nil, nil
}

func checkFormat(pass *analysis.Pass, node ast.Node, name string) {
	if !tRunNameFormat.MatchString(name) {
		pass.Reportf(node.Pos(), msgTRunNameFormat)
	}
}

// checkTableNames は、ループ変数 ident（for _, tt := range tests の tt）の field を、同じ関数で定義したテーブルの
// 要素の文字列リテラルまでさかのぼって調べる。さかのぼれない行・リテラルでない値は調べない
func checkTableNames(pass *analysis.Pass, ident *ast.Ident, field string, file *ast.File) {
	varObj, ok := pass.TypesInfo.Uses[ident].(*types.Var)
	if !ok {
		return
	}
	rangeStmt := findRangeStmtForLoopVar(pass, varObj, file)
	if rangeStmt == nil {
		return
	}
	var table *ast.CompositeLit
	switch x := rangeStmt.X.(type) {
	case *ast.CompositeLit:
		table = x
	case *ast.Ident:
		table = findCompositeLitForIdent(pass, x, file)
	}
	if table == nil {
		return
	}

	seen := map[string]bool{}
	for _, elt := range table.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			elt = kv.Value
		}
		row, ok := elt.(*ast.CompositeLit)
		if !ok {
			continue
		}
		lit, ok := fieldValue(pass, row, field).(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		name := unquote(lit.Value)
		checkFormat(pass, lit, name)
		if seen[name] {
			pass.Reportf(lit.Pos(), msgTRunNameDuplicated)
		}
		seen[name] = true
	}
}

// fieldValue は構造体リテラル row の field の値を返す（キーあり・キーなしの両方）
func fieldValue(pass *analysis.Pass, row *ast.CompositeLit, field string) ast.Expr {
	for i, elt := range row.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == field {
				return kv.Value
			}
			continue
		}
		st, ok := pass.TypesInfo.TypeOf(row).Underlying().(*types.Struct)
		if ok && i < st.NumFields() && st.Field(i).Name() == field {
			return elt
		}
	}
	return nil
}

func objectOf(pass *analysis.Pass, expr ast.Expr) types.Object {
	if ident, ok := expr.(*ast.Ident); ok {
		return pass.TypesInfo.Uses[ident]
	}
	return nil
}

func unquote(quoted string) string {
	if s, err := strconv.Unquote(quoted); err == nil {
		return s
	}
	return quoted
}

func isTTestPointer(pass *analysis.Pass, expr ast.Expr) bool {
	ptr, ok := pass.TypesInfo.TypeOf(expr).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := ptr.Elem().(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == "T"
}

func findRangeStmtForLoopVar(pass *analysis.Pass, varObj *types.Var, file *ast.File) *ast.RangeStmt {
	var found *ast.RangeStmt
	ast.Inspect(file, func(n ast.Node) bool {
		rangeStmt, ok := n.(*ast.RangeStmt)
		if !ok {
			return found == nil
		}
		for _, e := range []ast.Expr{rangeStmt.Key, rangeStmt.Value} {
			if ident, ok := e.(*ast.Ident); ok && pass.TypesInfo.Defs[ident] == varObj {
				found = rangeStmt
				return false
			}
		}
		return true
	})
	return found
}

func findCompositeLitForIdent(pass *analysis.Pass, ident *ast.Ident, file *ast.File) *ast.CompositeLit {
	obj := pass.TypesInfo.Uses[ident]
	if obj == nil {
		return nil
	}
	var found *ast.CompositeLit
	ast.Inspect(file, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range s.Lhs {
				if l, ok := lhs.(*ast.Ident); ok && pass.TypesInfo.Defs[l] == obj && i < len(s.Rhs) {
					found, _ = s.Rhs[i].(*ast.CompositeLit)
					return false
				}
			}
		case *ast.ValueSpec:
			for i, name := range s.Names {
				if pass.TypesInfo.Defs[name] == obj && i < len(s.Values) {
					found, _ = s.Values[i].(*ast.CompositeLit)
					return false
				}
			}
		}
		return found == nil
	})
	return found
}
