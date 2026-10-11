package excludepkgconsumer

import "factoryenforcer/internal/generatedpkg/domain"

// exclude-packages指定によりgeneratedpkg配下の型は検知対象外: OK（ゼロ値宣言）
func zeroValueGeneratedPackageTypeAllowed() domain.Status {
	var status domain.Status
	return status
}

// exclude-packages指定によりgeneratedpkg配下の型は検知対象外: OK（型変換）
func directConversionGeneratedPackageTypeAllowed() domain.Status {
	return domain.Status("ACTIVE")
}
