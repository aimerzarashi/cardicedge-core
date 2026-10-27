package board

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidState     = errors.New("board: invalid state")
	ErrNameEmpty        = errors.New("board: name is empty")
	ErrNameTooLong      = errors.New("board: name is too long")
	ErrNameAlreadyInUse = errors.New("board: name already in use")
	ErrStatusUnknown    = errors.New("board: unknown status")
	ErrTileKindUnknown  = errors.New("board: unknown tile kind")
	// 幅・高さが 1 未満、または studio_rules の範囲の外
	ErrSizeOutOfRange = errors.New("board: size is out of range")
	// 準備中のボードを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("board: invalid status transition")
	// 準備中でないボードの名前・レイアウトを変える、または削除しようとした
	ErrNotDraft = errors.New("board: not draft")
	// レイアウトの決まり（ValidateLayout）を守っていない。違反の一覧は *InvalidLayoutError
	ErrInvalidLayout = errors.New("board: invalid layout")
	// 公開の条件を満たしていない。数は *NotPublishableError
	ErrNotPublishable = errors.New("board: not publishable")
)

type InvalidLayoutError struct {
	Violations []LayoutViolation
}

func (e *InvalidLayoutError) Error() string {
	return fmt.Sprintf("%s: %d violations", ErrInvalidLayout, len(e.Violations))
}

func (e *InvalidLayoutError) Is(target error) bool {
	return target == ErrInvalidLayout
}

// 画面が何を直せばよいか示せるよう、Readiness の数と行けないマスをそのまま持つ
type NotPublishableError struct {
	Readiness Readiness
}

func (e *NotPublishableError) Error() string {
	return fmt.Sprintf("%s: bases=%d checkpoints=%d unreachable=%d",
		ErrNotPublishable, e.Readiness.BaseCount, e.Readiness.CheckpointCount, len(e.Readiness.UnreachableTiles))
}

func (e *NotPublishableError) Is(target error) bool {
	return target == ErrNotPublishable
}
