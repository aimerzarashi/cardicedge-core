package analyzer_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"

	"gitlab.com/aimerzarashi/diecardomain/core/tools/linter/analyzer"
)

func TestFactoryEnforcer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer.FactoryEnforcer, "factoryenforcer/consumer", "factoryenforcer/internal/domain")
}

// 自動生成パッケージ（exclude-packages指定）の型は、定義元パッケージの時点で検知対象外になることを確認する
func TestFactoryEnforcer_ExcludePackages(t *testing.T) {
	require.NoError(t, analyzer.FactoryEnforcer.Flags.Set("exclude-packages", "internal/generatedpkg"))
	t.Cleanup(func() {
		require.NoError(t, analyzer.FactoryEnforcer.Flags.Set("exclude-packages", ""))
	})

	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer.FactoryEnforcer, "factoryenforcer/excludepkgconsumer", "factoryenforcer/internal/generatedpkg/domain")
}

// 生成コード（先頭に Code generated ... DO NOT EDIT.）は、除外の設定が無くても対象外になる
func TestFactoryEnforcer_SkipsGeneratedCode(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer.FactoryEnforcer, "factoryenforcer/genconsumer", "factoryenforcer/internal/gentypes", "factoryenforcer/internal/domain")
}
