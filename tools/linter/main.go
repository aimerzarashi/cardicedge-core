package main

import (
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"

	"gitlab.com/aimerzarashi/diecardomain/core/tools/linter/analyzer"
)

func main() {
	// core の設定は、make lint・CI に引数で書かず、ここで既定値として決める（両方で同じ設定にするため）。
	// 検査ごとの既定値（analyzer パッケージ）を書き換えないのは、検査のテストがその既定値を前提にしているため
	// internal 全体を対象にするのは、値の型を足したパッケージを対象に入れ忘れないため（core#8）。
	// 生成コード（sqlboiler・gqlgen 等）は、先頭の Code generated ... DO NOT EDIT. で自動で除く
	set(analyzer.FactoryEnforcer, "packages", "internal")
	// 外部のライブラリは、名前が衝突したときに別名が要るため対象にしない
	set(analyzer.ImportAliasForbidder, "packages", "gitlab.com/aimerzarashi/diecardomain/core")

	multichecker.Main(
		analyzer.FactoryEnforcer,
		analyzer.ImportAliasForbidder,
		analyzer.TestifyMsgAndArgsForbidder,
		analyzer.TRunNamingEnforcer,
		analyzer.TestContextEnforcer,
	)
}

func set(a *analysis.Analyzer, name, value string) {
	if err := a.Flags.Set(name, value); err != nil {
		panic(err)
	}
}
