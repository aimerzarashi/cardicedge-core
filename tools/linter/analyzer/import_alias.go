package analyzer

import (
	"flag"
	"strconv"

	"golang.org/x/tools/go/analysis"
)

var importAliasFilters = &filterConfig{
	excludeFiles:    stringPatterns{"generated.go", "models_gen.go"},
	excludePackages: stringPatterns{"platform/orm/sqlboiler/models"},
}

var ImportAliasForbidder = &analysis.Analyzer{
	Name:  "import_alias_forbidder",
	Doc:   "forbids import aliasing for the configured packages",
	Run:   runImportAliasForbidder,
	Flags: newImportAliasFlags(),
}

func newImportAliasFlags() flag.FlagSet {
	fs := flag.FlagSet{}
	importAliasFilters.registerFlags(&fs, "comma-separated list of package path patterns to forbid aliasing for (e.g. internal/domain,internal/studio). Empty means all packages.")
	return fs
}

func runImportAliasForbidder(pass *analysis.Pass) (any, error) {
	if importAliasFilters.shouldSkipPackage(pass) {
		return nil, nil
	}

	for _, file := range pass.Files {
		if importAliasFilters.shouldSkipFile(pass, file) {
			continue
		}

		for _, imp := range file.Imports {
			if imp.Name == nil {
				continue
			}

			// `_`（blank import）と `.`（dot import）は別目的の構文なのでエイリアスとして扱わない
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				continue
			}

			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}

			if !matchesAny(path, importAliasFilters.targetPackages, true) {
				continue
			}

			pass.Reportf(imp.Pos(), "import alias for %q is prohibited. Use the package's canonical name.", path)
		}
	}
	return nil, nil
}
