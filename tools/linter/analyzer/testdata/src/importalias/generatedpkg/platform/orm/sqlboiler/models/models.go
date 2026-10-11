package models

// このパッケージ自体が exclude-packages のデフォルトパターン
// (platform/orm/sqlboiler/models) に該当するため、
// パッケージ全体が検証対象外となり、エイリアスがあっても検知されない
import alias "importalias/importtarget"

func UseAliased() string {
	return alias.Name
}
