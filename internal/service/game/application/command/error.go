package command

import (
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
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

// 部屋の操作の結果の理由。presentation 層にドメインのエラーを見せないため、application 層のエラーに変えて返す
var (
	// 部屋が無い・入っていない部屋（存在を明かさないため区別しない）
	ErrRoomNotFound = errors.New("command: room not found")
	// 画面で見た revision が今の版と違う（画面を開いた後に、ほかの操作で変わった）
	ErrRevisionConflict = errors.New("command: revision conflict")
	// 今の状態ではできない操作
	ErrRoomStatusNotAllowed = errors.New("command: room operation not allowed in this status")
	// 席の並びがまだ決まっていない
	ErrRoomSeatsNotDecided = errors.New("command: room seats not decided")
	// 公開前に決めたものを、準備中に変えようとした
	ErrRoomConditionLocked = errors.New("command: room condition locked")
	// その操作をする担当ではない（作った人だけの操作、ほかの人の席、部屋に入っていない人の準備完了）
	ErrRoomNotResponsible = errors.New("command: room not responsible")
	// 準備完了を申告した人が、自分の担当の分を選び直そうとした、または申告し直そうとした
	ErrRoomAlreadyReady = errors.New("command: room already ready")
	// 自分の担当の分に、まだ決まっていないものがある
	ErrRoomConditionsNotDecided = errors.New("command: room conditions not decided")
)

// 対戦の操作の結果の理由（画面で見た revision と違う場合は、部屋と同じ ErrRevisionConflict）
var (
	// 対戦が無い・参加していない対戦（存在を明かさないため区別しない）
	ErrMatchNotFound = errors.New("command: match not found")
	// 今の番の参加者ではない
	ErrMatchNotYourTurn = errors.New("command: not your turn")
	// 今の段階ではできない操作
	ErrMatchPhaseNotAllowed = errors.New("command: match phase not allowed")
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

// 読み込みから保存までに先を越された（eventstore の版の衝突）のも、利用者から見れば画面が古いのと同じなので、同じエラーにする
func revisionConflict(err error) error {
	if errors.Is(err, domain.ErrVersionConflict) {
		return ErrRevisionConflict
	}
	return err
}
