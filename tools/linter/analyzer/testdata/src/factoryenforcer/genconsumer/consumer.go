package genconsumer

import (
	"factoryenforcer/internal/domain"
	"factoryenforcer/internal/gentypes"
)

// 生成コードで定義された型は生成関数を持たないため、直接作ってよい
func generatedTypes() gentypes.Row {
	return gentypes.Row{Code: gentypes.Code("x")}
}

// 手で書いた型は、生成コードの型の隣でも対象のまま
func handWritten() domain.Email {
	return domain.Email("a@example.com") // want `direct type conversion of factoryenforcer/internal/domain\.Email is prohibited\. Use a factory function\.`
}
