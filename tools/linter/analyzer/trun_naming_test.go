package analyzer_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"gitlab.com/aimerzarashi/diecardomain/core/tools/linter/analyzer"
)

func TestTRunNamingEnforcer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer.TRunNamingEnforcer, "trun")
}
