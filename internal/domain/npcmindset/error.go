package npcmindset

import "errors"

var (
	ErrInvalidState     = errors.New("npcmindset: invalid state")
	ErrNameEmpty        = errors.New("npcmindset: name is empty")
	ErrNameTooLong      = errors.New("npcmindset: name is too long")
	ErrNameAlreadyInUse = errors.New("npcmindset: name already in use")
	ErrStatusUnknown    = errors.New("npcmindset: unknown status")
	// 準備中の思考パターンを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("npcmindset: invalid status transition")
	// 準備中でない思考パターンの名前・重みを変える、または削除しようとした
	ErrNotDraft = errors.New("npcmindset: not draft")
	// 重みが 0〜100 の範囲にない
	ErrWeightOutOfRange = errors.New("npcmindset: weight is out of range")
	ErrCriterionUnknown = errors.New("npcmindset: unknown criterion")
	// 行動の種類ごとの重みが、matchaction.Actions() のすべてと過不足なく一致しない（足りない・余分・知らない種類）
	ErrActionWeightsMismatch = errors.New("npcmindset: action weights do not match the actions")
	// 採点基準ごとの重みが、Criteria() のすべてと過不足なく一致しない（足りない・余分・知らない基準）
	ErrCriterionWeightsMismatch = errors.New("npcmindset: criterion weights do not match the criteria")
)
