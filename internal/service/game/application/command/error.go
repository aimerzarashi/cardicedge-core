package command

import "errors"

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変えて返す
var (
	// 認証ミドルウェアが JWT の値から (iss, sub) を特定できなかった（値の形が正しくない）
	ErrInvalidCredentials = errors.New("command: invalid credentials")
	// 検証済みの (iss, sub) が context に無い（認証ミドルウェアを通らずに呼ばれた）
	ErrNotAuthenticated = errors.New("command: not authenticated")
	// (iss, sub) に登録済みのプレイヤーがいない
	ErrPlayerNotRegistered = errors.New("command: player not registered")
)
