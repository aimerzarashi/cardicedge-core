// Package migrations は DB マイグレーション（NNNN_name.sql）を埋め込んで提供する。
// 適用は `app migrate`（internal/platform/migration）で行う。
package migrations

import "embed"

// FS はこのディレクトリのファイルを埋め込む。.sql 以外（このファイル等）は適用時に無視される。
// （SQL ファイルが1つも無くてもビルドできるよう、パターンは * にしている）
//
//go:embed *
var FS embed.FS
