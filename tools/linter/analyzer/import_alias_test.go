package analyzer_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"gitlab.com/aimerzarashi/diecardomain/core/tools/linter/analyzer"
)

func TestImportAliasForbidder(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer.ImportAliasForbidder, "importalias/importconsumer", "importalias/generatedpkg/platform/orm/sqlboiler/models")
}
