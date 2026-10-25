package command

import "errors"

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
	// 公開中のスターターデッキの数の上限に達している
	ErrStarterDeckLimitReached = errors.New("command: starter deck limit reached")
	// 公開 ID に当たるカード（Creature・Item・Spell）が無い（カードでないものを含む）
	ErrCardNotFound = errors.New("command: card not found")
	// 準備中のカードを使用不可にしようとした
	ErrInvalidCardStatusTransition = errors.New("command: invalid card status transition")
	// ゲーム全体のルールがまだ作られていない（studio は起動時に作るので、起きないはずの状態）
	ErrGameRulesNotCreated = errors.New("command: game rules not created")
	// 画面で見た revision が今の版と違う（画面を開いた後に、ほかの操作で変わった）
	ErrRevisionConflict = errors.New("command: revision conflict")
)
