package room

import "errors"

var (
	// 名前が空（前後の空白を除いた後）
	ErrNameEmpty      = errors.New("room: name is empty")
	ErrNameTooLong    = errors.New("room: name is too long")
	ErrInvalidState   = errors.New("room: invalid state")
	ErrAlreadyCreated = errors.New("room: already created")
	// 作った人だけができる操作（席の並び・公開・閉じる）を、ほかの人がした
	ErrNotOwner = errors.New("room: not owner")
	// 準備中に、その条件を決める担当ではない人が選ぼうとした（プレイヤーの席は座っている人、NPC の席と対戦ルールは作った人）
	ErrNotResponsible = errors.New("room: not responsible")
	// 部屋に入っていない人が準備完了を申告しようとした
	ErrNotParticipant = errors.New("room: not participant")
	// 今の状態ではできない操作（席の並びと公開は公開前だけ、条件を選ぶのと準備完了は公開前か準備中だけ、準備完了は準備中だけ）
	ErrStatusNotAllowed = errors.New("room: operation not allowed in this status")
	// 席の並びがまだ決まっていない（公開、席の選択はできない）
	ErrSeatsNotDecided = errors.New("room: seats not decided")
	// 席の並びに無い席
	ErrSeatNotFound = errors.New("room: seat not found")
	// 席の種類と違う側の選択
	ErrSeatKindMismatch = errors.New("room: seat kind mismatch")
	// 公開前に決めたものを、準備中に変えようとした
	ErrConditionLocked = errors.New("room: condition locked")
	// 準備完了を申告した人が、自分の担当の分を選び直そうとした、または申告し直そうとした
	ErrAlreadyReady = errors.New("room: already ready")
	// 自分の担当の分に、まだ決まっていないものがある
	ErrConditionsNotDecided = errors.New("room: conditions not decided")
	// 対戦の後の意思表示を、すでにした人がもう一度しようとした（変え直しは無い）
	ErrAlreadyDeclaredNext = errors.New("room: already declared next")
	// 最後の準備完了なのに、対戦の ID が渡されていない（呼ぶ側の誤り）
	ErrMatchIDMissing = errors.New("room: match id missing")
	// 部屋を作るのに要るもの（アバター・デッキ）が足りない。足りないものは RequirementsNotMetError が持つ
	ErrRequirementsNotMet = errors.New("room: requirements not met")
)

// 部屋を作るのに足りないもの（Requirement の並び。先に作る順）。errors.Is で ErrRequirementsNotMet と比べられる
type RequirementsNotMetError struct {
	Missing []Requirement
}

func (e *RequirementsNotMetError) Error() string {
	return ErrRequirementsNotMet.Error()
}

func (e *RequirementsNotMetError) Is(target error) bool {
	return target == ErrRequirementsNotMet
}
