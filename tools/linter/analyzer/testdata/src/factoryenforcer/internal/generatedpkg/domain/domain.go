// Package domain は gqlgen 等の完全自動生成パッケージを模したテスト用パッケージ
package domain

// Status は自動生成される enum 型を模したテスト用の defined type
type Status string

const (
	StatusActive Status = "ACTIVE"
)
