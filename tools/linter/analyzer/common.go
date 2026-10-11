package analyzer

import (
	"flag"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"
)

type stringPatterns []string

func (p *stringPatterns) String() string {
	return strings.Join(*p, ",")
}

func (p *stringPatterns) Set(v string) error {
	for pattern := range strings.SplitSeq(v, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern != "" {
			*p = append(*p, pattern)
		}
	}
	return nil
}

func matchesAny(target string, patterns stringPatterns, matchAllIfEmpty bool) bool {
	if len(patterns) == 0 {
		return matchAllIfEmpty
	}
	for _, pattern := range patterns {
		if strings.Contains(target, pattern) {
			return true
		}
	}
	return false
}

type filterConfig struct {
	// 部分一致。空ならすべてのパッケージを対象にする
	targetPackages stringPatterns
	// 手書きと生成コードが混ざるパッケージで、生成されたファイルだけを除くため
	excludeFiles stringPatterns
	// 全ファイルが生成コードのパッケージ（sqlboiler の models 等）を除くため
	excludePackages stringPatterns
}

func (c *filterConfig) registerFlags(fs *flag.FlagSet, targetUsage string) {
	fs.Var(&c.targetPackages, "packages", targetUsage)
	fs.Var(&c.excludeFiles, "exclude-files", "comma-separated list of file path patterns to exclude (e.g. generated.go,models_gen.go). Use for packages that mix hand-written and generated files.")
	fs.Var(&c.excludePackages, "exclude-packages", "comma-separated list of package path patterns to exclude entirely (e.g. platform/orm/sqlboiler/models). Use for fully generated packages.")
}

func (c *filterConfig) isTargetPackage(pkgPath string) bool {
	return matchesAny(pkgPath, c.targetPackages, false)
}

func (c *filterConfig) shouldSkipPackage(pass *analysis.Pass) bool {
	return matchesAny(pass.Pkg.Path(), c.excludePackages, false)
}

// go test がリンク用に合成する仮想ファイル（.go 拡張子が無い）は、手で直せないため除く
func (c *filterConfig) shouldSkipFile(pass *analysis.Pass, file *ast.File) bool {
	filename := pass.Fset.Position(file.Pos()).Filename
	if !strings.HasSuffix(filename, ".go") {
		return true
	}
	// 生成コードは手で直せないため調べない（先頭の Code generated ... DO NOT EDIT. で判定する）
	if ast.IsGenerated(file) {
		return true
	}
	return matchesAny(filename, c.excludeFiles, false)
}
