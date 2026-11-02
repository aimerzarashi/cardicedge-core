package command

import (
	"errors"
	"fmt"
)

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変えて返す
var (
	// 認証ミドルウェアが JWT の値からオペレーターを特定できなかった（値の形が正しくない）
	ErrInvalidCredentials = errors.New("command: invalid credentials")
	// 操作した人が context に無い（presentation 層の認可を通らずに呼ばれた）
	ErrNotOperator = errors.New("command: not an operator")
	// 登録しようとしたメールアドレスを、ほかのオペレーターが使っている
	ErrOperatorEmailAlreadyRegistered = errors.New("command: operator email already registered")
	// Creature・Item・Spell のどれかが同じ名前を使っている
	ErrCardNameAlreadyInUse = errors.New("command: card name already in use")
	// ほかのプリセットデッキが同じ名前を使っている
	ErrPresetDeckNameAlreadyInUse = errors.New("command: starter deck name already in use")
	// 公開 ID に当たるプリセットデッキが無い（削除したものを含む）
	ErrPresetDeckNotFound = errors.New("command: starter deck not found")
	// ほかのスターターデッキが同じ名前を使っている（状態によらない）
	ErrStarterDeckNameAlreadyInUse = errors.New("command: starter deck name already in use")
	// 公開 ID に当たるスターターデッキが無い（削除したものを含む）
	ErrStarterDeckNotFound = errors.New("command: starter deck not found")
	// 準備中のスターターデッキを使用不可にしようとした
	ErrInvalidStarterDeckStatusTransition = errors.New("command: invalid starter deck status transition")
	// 準備中でないスターターデッキの名前・枚数を変える、または削除しようとした
	ErrStarterDeckNotDraft = errors.New("command: starter deck is not draft")
	// 公開中のスターターデッキの数の上限に達している。上限の値は LimitReachedError で受け取る
	ErrStarterDeckLimitReached = errors.New("command: starter deck limit reached")
	// ほかのボードが同じ名前を使っている（状態によらない）
	ErrBoardNameAlreadyInUse = errors.New("command: board name already in use")
	// 公開 ID に当たるボードが無い（削除したものを含む）
	ErrBoardNotFound = errors.New("command: board not found")
	// 準備中のボードを使用不可にしようとした
	ErrInvalidBoardStatusTransition = errors.New("command: invalid board status transition")
	// 準備中でないボードの名前・レイアウトを変える、または削除しようとした
	ErrBoardNotDraft = errors.New("command: board is not draft")
	// 公開の条件（拠点・チェックポイント・拠点から行けること）を満たしていない。理由は BoardNotPublishableError で受け取る
	ErrBoardNotPublishable = errors.New("command: board is not publishable")
	// ほかの NPC のアバターが同じ名前を使っている（状態によらない）
	ErrNPCAvatarNameAlreadyInUse = errors.New("command: npc avatar name already in use")
	// 公開 ID に当たる NPC のアバターが無い（削除したものを含む）
	ErrNPCAvatarNotFound = errors.New("command: npc avatar not found")
	// 準備中の NPC のアバターを使用不可にしようとした
	ErrInvalidNPCAvatarStatusTransition = errors.New("command: invalid npc avatar status transition")
	// 準備中でない NPC のアバターの名前を変える、または削除しようとした
	ErrNPCAvatarNotDraft = errors.New("command: npc avatar is not draft")
	// ほかの NPC のデッキが同じ名前を使っている（状態によらない）
	ErrNPCDeckNameAlreadyInUse = errors.New("command: npc deck name already in use")
	// 公開 ID に当たる NPC のデッキが無い（削除したものを含む）
	ErrNPCDeckNotFound = errors.New("command: npc deck not found")
	// 準備中の NPC のデッキを使用不可にしようとした
	ErrInvalidNPCDeckStatusTransition = errors.New("command: invalid npc deck status transition")
	// 準備中でない NPC のデッキの名前・枚数を変える、または削除しようとした
	ErrNPCDeckNotDraft = errors.New("command: npc deck is not draft")
	// 公開 ID に当たるカード（Creature・Item・Spell）が無い（カードでないものを含む）
	ErrCardNotFound = errors.New("command: card not found")
	// 準備中のカードを使用不可にしようとした
	ErrInvalidCardStatusTransition = errors.New("command: invalid card status transition")
	// ゲーム全体のルールがまだ作られていない（studio は起動時に作るので、起きないはずの状態）
	ErrGameRulesNotCreated = errors.New("command: game rules not created")
	// studio_rules がまだ作られていない（同じく、起きないはずの状態）
	ErrStudioRulesNotCreated = errors.New("command: studio rules not created")
	// 画面で見た revision が今の版と違う（画面を開いた後に、ほかの操作で変わった）
	ErrRevisionConflict = errors.New("command: revision conflict")
)

// 上限に達した。画面が「N 個まで」と文言に出せるよう、そのときの上限の値を持つ（studio は上限の値を書き写さないため）。
// errors.Is は理由ごとの Err...LimitReached と一致する（値の要らない呼び出し側は、今までどおり理由だけで見分けられる）
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

// 公開できない理由の種類。read model の publish_problems と同じ種類（domain の board.PublishProblemKind の値）
type BoardPublishProblemKind string

const (
	BoardPublishProblemBaseCount        BoardPublishProblemKind = "base_count"
	BoardPublishProblemNoCheckpoint     BoardPublishProblemKind = "no_checkpoint"
	BoardPublishProblemUnreachableTiles BoardPublishProblemKind = "unreachable_tiles"
	BoardPublishProblemUnpairedTiles    BoardPublishProblemKind = "unpaired_tiles"
)

// 公開できない理由 1 つ。Positions は関わるマスの座標（無ければ空）
type BoardPublishProblem struct {
	Kind      BoardPublishProblemKind
	Positions []BoardPosition
}

// 公開の条件を満たしていない。画面が直す先を示せるよう、理由ごとの種類と座標を持つ（LimitReachedError と同じ考え）。
// 理由の順は 拠点の数 → チェックポイント → 行けないマス。errors.Is は ErrBoardNotPublishable と一致する
type BoardNotPublishableError struct {
	Problems []BoardPublishProblem
}

func (e *BoardNotPublishableError) Error() string {
	return fmt.Sprintf("%s (%d problems)", ErrBoardNotPublishable, len(e.Problems))
}

func (e *BoardNotPublishableError) Unwrap() error {
	return ErrBoardNotPublishable
}
