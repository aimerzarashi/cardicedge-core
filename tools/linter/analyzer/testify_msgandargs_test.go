package analyzer_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"gitlab.com/aimerzarashi/diecardomain/core/tools/linter/analyzer"
)

func TestTestifyMsgAndArgsForbidder(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer.TestifyMsgAndArgsForbidder, "testifymsgandargs")
}
