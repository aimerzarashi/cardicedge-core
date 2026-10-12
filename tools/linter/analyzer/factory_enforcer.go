package analyzer

import (
	"flag"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

var factoryEnforcerFilters = &filterConfig{
	targetPackages: stringPatterns{"internal"},
}

var FactoryEnforcer = &analysis.Analyzer{
	Name:      "factory_enforcer",
	Doc:       "enforces the use of factory functions for target layer types, prohibiting direct instantiation (type conversion, struct literal, zero-value variable declaration)",
	Run:       run,
	Flags:     newFlags(),
	FactTypes: []analysis.Fact{new(generatedType)},
}

// 生成コード（先頭に Code generated ... DO NOT EDIT.）で定義された型の印。生成コードは生成関数を持たないため対象外にする。
// パスの名前で除外を設定すると生成コードのパッケージを足すたびに設定が要るため、型を定義したパッケージを調べるときに
// 印を付け、使う側のパッケージを調べるときに Facts で受け取る（core#8）
type generatedType struct{}

func (*generatedType) AFact() {}

func (*generatedType) String() string {
	return "generatedType"
}

func exportGeneratedTypes(pass *analysis.Pass) {
	for _, file := range pass.Files {
		if !ast.IsGenerated(file) {
			continue
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				if obj := pass.TypesInfo.Defs[spec.(*ast.TypeSpec).Name]; obj != nil {
					pass.ExportObjectFact(obj, &generatedType{})
				}
			}
		}
	}
}

func newFlags() flag.FlagSet {
	fs := flag.FlagSet{}
	factoryEnforcerFilters.registerFlags(&fs, "comma-separated list of package path patterns to enforce (e.g. internal/domain,internal/myother)")
	return fs
}

func run(pass *analysis.Pass) (any, error) {
	exportGeneratedTypes(pass)
	if factoryEnforcerFilters.shouldSkipPackage(pass) {
		return nil, nil
	}

	for _, file := range pass.Files {
		if factoryEnforcerFilters.shouldSkipFile(pass, file) {
			continue
		}

		ast.Inspect(file, func(n ast.Node) bool {
			switch expr := n.(type) {
			// ゼロ値の変数宣言は defined type（type Xxx string 等）だけを検出する。構造体（Event・State・ReadModel 等）はゼロ値から組み立てるため許す
			case *ast.GenDecl:
				if expr.Tok != token.VAR {
					return true
				}

				for _, spec := range expr.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok || len(valueSpec.Values) > 0 {
						continue
					}

					if valueSpec.Type != nil {
						declType := pass.TypesInfo.TypeOf(valueSpec.Type)
						if declType == nil {
							continue
						}

						// 構造体・インターフェースは、DI のためにゼロ値で宣言することがあるため許す
						if _, ok := declType.Underlying().(*types.Struct); ok {
							continue
						}
						if _, ok := declType.Underlying().(*types.Interface); ok {
							continue
						}

						if isTargetLayerTypeInfo(declType) && isExternalUsageType(pass, declType) {
							typeName := types.TypeString(declType, nil)
							for _, name := range valueSpec.Names {
								pass.Reportf(name.Pos(), "zero-value variable declaration of %s is prohibited. Use a factory function.", typeName)
							}
						}
					}
				}

			case *ast.CallExpr:
				if isTypeConversion(pass, expr) {
					if isTargetLayerType(pass, expr.Fun) && isExternalUsage(pass, expr.Fun) {
						typeName := types.TypeString(pass.TypesInfo.TypeOf(expr.Fun), nil)
						pass.Reportf(expr.Pos(), "direct type conversion of %s is prohibited. Use a factory function.", typeName)
					}
				}

			case *ast.CompositeLit:
				// スライス・配列・マップのリテラルの要素は構造体の項目ではないため対象外。要素が対象の構造体リテラルなら、ast.Inspect がその要素を別に訪れる
				if !isStructLitType(pass, expr) {
					return true
				}

				for i, elt := range expr.Elts {
					var key ast.Expr
					value := elt
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						key = kv.Key
						value = kv.Value
					}

					var fieldType types.Type
					if key != nil {
						fieldType = getFieldType(pass, key, expr)
					} else {
						fieldType = getFieldTypeByIndex(pass, expr, i)
					}

					if fieldType == nil {
						// 項目の型が取れない場合でも、値が対象の構造体リテラルなら検出するため
						if compLit, ok := value.(*ast.CompositeLit); ok {
							valueType := pass.TypesInfo.TypeOf(compLit)
							if valueType != nil && isTargetLayerTypeInfo(valueType) && isExternalUsageType(pass, valueType) {
								typeName := types.TypeString(valueType, nil)
								pass.Reportf(value.Pos(), "direct assignment of %s is prohibited. Pass a pre-validated variable.", typeName)
							}
						}
						continue
					}

					if isTargetLayerTypeInfo(fieldType) && isExternalUsageType(pass, fieldType) {
						if isDirectCreation(pass, value) {
							typeName := types.TypeString(fieldType, nil)
							pass.Reportf(value.Pos(), "direct assignment of %s is prohibited. Pass a pre-validated variable.", typeName)
						}
					}
				}
			}
			return true
		})
	}
	return nil, nil
}

func getFieldType(pass *analysis.Pass, keyExpr ast.Expr, compLit *ast.CompositeLit) types.Type {
	litType := pass.TypesInfo.TypeOf(compLit)
	if litType == nil {
		return nil
	}

	var structType *types.Struct
	switch t := litType.Underlying().(type) {
	case *types.Struct:
		structType = t
	case *types.Named:
		if s, ok := t.Underlying().(*types.Struct); ok {
			structType = s
		}
	}
	if structType == nil {
		return nil
	}

	var fieldName string
	if ident, ok := keyExpr.(*ast.Ident); ok {
		fieldName = ident.Name
	} else if sel, ok := keyExpr.(*ast.SelectorExpr); ok {
		fieldName = sel.Sel.Name
	}

	if fieldName == "" {
		return nil
	}

	for field := range structType.Fields() {
		if field.Name() == fieldName {
			return field.Type()
		}
	}

	return nil
}

func isStructLitType(pass *analysis.Pass, compLit *ast.CompositeLit) bool {
	litType := pass.TypesInfo.TypeOf(compLit)
	if litType == nil {
		return false
	}

	switch t := litType.Underlying().(type) {
	case *types.Struct:
		return true
	case *types.Named:
		_, ok := t.Underlying().(*types.Struct)
		return ok
	default:
		return false
	}
}

func getFieldTypeByIndex(pass *analysis.Pass, compLit *ast.CompositeLit, index int) types.Type {
	litType := pass.TypesInfo.TypeOf(compLit)
	if litType == nil {
		return nil
	}

	var structType *types.Struct
	switch t := litType.Underlying().(type) {
	case *types.Struct:
		structType = t
	case *types.Named:
		if s, ok := t.Underlying().(*types.Struct); ok {
			structType = s
		}
	}
	if structType == nil || index >= structType.NumFields() {
		return nil
	}

	return structType.Field(index).Type()
}

// 生成コードのパッケージ（gqlgen・sqlboiler 等）の型は、生成関数を持たないため対象外にする
func isTargetPackage(pkgPath string) bool {
	if matchesAny(pkgPath, factoryEnforcerFilters.excludePackages, false) {
		return false
	}
	return factoryEnforcerFilters.isTargetPackage(pkgPath)
}

func isExternalUsage(pass *analysis.Pass, expr ast.Expr) bool {
	typ := pass.TypesInfo.TypeOf(expr)
	return isExternalUsageType(pass, typ)
}

func isExternalUsageType(pass *analysis.Pass, typ types.Type) bool {
	named, ok := typ.Underlying().(*types.Named)
	if !ok {
		named, ok = typ.(*types.Named)
	}
	if !ok || named.Obj().Pkg() == nil {
		return false
	}

	typePkgPath := named.Obj().Pkg().Path()

	// 同じパッケージの中は、生成関数の実装そのものなので許す
	if typePkgPath == pass.Pkg.Path() {
		return false
	}

	if !isTargetPackage(typePkgPath) {
		return false
	}

	return !pass.ImportObjectFact(named.Obj(), new(generatedType))
}

func isTargetLayerType(pass *analysis.Pass, expr ast.Expr) bool {
	typ := pass.TypesInfo.TypeOf(expr)
	return isTargetLayerTypeInfo(typ)
}

func isTargetLayerTypeInfo(typ types.Type) bool {
	for {
		if ptr, ok := typ.(*types.Pointer); ok {
			typ = ptr.Elem()
		} else if named, ok := typ.(*types.Named); ok {
			// Underlying が Pointer のこともあるため、繰り返し剥がす
			if _, ok := named.Underlying().(*types.Pointer); ok {
				typ = named.Underlying()
				continue
			}
			break
		} else {
			break
		}
	}

	named, ok := typ.(*types.Named)
	if !ok {
		return false
	}
	pkg := named.Obj().Pkg()
	return pkg != nil && isTargetPackage(pkg.Path())
}

func isDirectCreation(pass *analysis.Pass, expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return true // "STAFF001" などの生の値はNG
	case *ast.CallExpr:
		// 型変換は検証を通らないため違反。関数呼び出し（NewXxx）は検証を通るので許す
		return isTypeConversion(pass, e)
	case *ast.CompositeLit:
		// 対象の構造体を直接作るのは、項目が変数や空でも違反（生成関数で作らせるため）
		typ := pass.TypesInfo.TypeOf(e)
		if typ != nil && isTargetLayerTypeInfo(typ) && isExternalUsageType(pass, typ) {
			return true
		}

		for _, elt := range e.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if isDirectCreation(pass, kv.Value) {
					return true
				}
			} else {
				if isDirectCreation(pass, elt) {
					return true
				}
			}
		}
		// 項目がすべて変数や関数呼び出し（検証済みの値）なら、リテラルでの組み立てを許す
		return false
	default:
		return false // 変数 (ast.Ident) などは安全とみなす
	}
}

func isTypeConversion(pass *analysis.Pass, call *ast.CallExpr) bool {
	tv, ok := pass.TypesInfo.Types[call.Fun]
	if !ok {
		return false
	}

	// 型変換の Fun は、型情報の上で IsType() が true になる
	if tv.IsType() {
		return true
	}

	// 型情報が取れない場合に備えて、識別子のオブジェクトでも確かめる
	var obj types.Object
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		obj = pass.TypesInfo.Uses[fun]
	case *ast.SelectorExpr:
		obj = pass.TypesInfo.Uses[fun.Sel]
	}

	if obj != nil {
		switch obj.(type) {
		case *types.Var, *types.Func:
			return false
		case *types.TypeName:
			return true
		}
	}

	return false
}
