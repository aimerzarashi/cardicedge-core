package command

import (
	"errors"
	"fmt"
)

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変えて返す
var (
	// 認証ミドルウェアが JWT の値から (iss, sub) を特定できなかった（値の形が正しくない）
	ErrInvalidCredentials = errors.New("command: invalid credentials")
	// 検証済みの (iss, sub) が context に無い（認証ミドルウェアを通らずに呼ばれた）
	ErrNotAuthenticated = errors.New("command: not authenticated")
	// (iss, sub) に登録済みのプレイヤーがいない
	ErrPlayerNotRegistered = errors.New("command: player not registered")
	// 同じプレイヤーの別のアバターが同じ名前を使っている（ほかのプレイヤーとは同じ名前でよい）
	ErrAvatarNameAlreadyInUse = errors.New("command: avatar name already in use")
	// プレイヤーが持てるアバターの数の上限に達している。上限の値は LimitReachedError で受け取る
	ErrAvatarLimitReached = errors.New("command: avatar limit reached")
)

// スターターの配布を始めるときに、マスタが game に公開されていない。待てば直る失敗（公開すれば直る）なので、
// ポリシーのやり直しに任せる
var ErrMasterNotPublished = errors.New("command: master not published")

// 上限に達した。画面が「N 体まで」と文言に出せるよう、そのときの上限の値を持つ（game は上限の値を書き写さないため）。
// errors.Is は理由ごとの Err...LimitReached と一致する（値の要らない呼び出し側は、理由だけで見分けられる。studio の LimitReachedError と同じ形）
type LimitReachedError struct {
	reason error
	Max    int
}

func (e *LimitReachedError) Error() string {
	return fmt.Sprintf("%s (max %d)", e.reason, e.Max)
}

func (e *LimitReachedError) Unwrap() error {
	return e.reason
}
