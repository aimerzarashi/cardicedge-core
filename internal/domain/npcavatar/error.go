package npcavatar

import "errors"

var (
	ErrInvalidState     = errors.New("npcavatar: invalid state")
	ErrNameEmpty        = errors.New("npcavatar: name is empty")
	ErrNameTooLong      = errors.New("npcavatar: name is too long")
	ErrNameAlreadyInUse = errors.New("npcavatar: name already in use")
	ErrStatusUnknown    = errors.New("npcavatar: unknown status")
	// 準備中の NPC のアバターを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("npcavatar: invalid status transition")
	// 準備中でない NPC のアバターの名前・使うデッキと思考パターンを変える、または削除しようとした
	ErrNotDraft = errors.New("npcavatar: not draft")
)
