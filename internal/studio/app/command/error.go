package command

import "errors"

// uif にドメインのエラーを見せないため、app 層のエラーに変えて返す
var (
	// 認証ミドルウェアが JWT の値からオペレーターを特定できなかった（値の形が正しくない）
	ErrInvalidCredentials = errors.New("command: invalid credentials")
	// 操作した人が context に無い（uif の認可を通らずに呼ばれた）
	ErrNotOperator = errors.New("command: not an operator")
	// 登録しようとしたメールアドレスを、ほかのオペレーターが使っている
	ErrOperatorEmailAlreadyRegistered = errors.New("command: operator email already registered")
	// Creature・Item・Spell のどれかが同じ名前を使っている
	ErrCardNameAlreadyInUse = errors.New("command: card name already in use")
	// ほかのプリセットデッキが同じ名前を使っている
	ErrPresetDeckNameAlreadyInUse = errors.New("command: starter deck name already in use")
	// 公開 ID に当たるプリセットデッキが無い（削除したものを含む）
	ErrPresetDeckNotFound = errors.New("command: starter deck not found")
	// 画面で見た revision が今の版と違う（画面を開いた後に、ほかの操作で変わった）
	ErrRevisionConflict = errors.New("command: revision conflict")
)
