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
	ErrAttributeUnknown = errors.New("board: unknown attribute")
	// 幅・高さが 1 未満、または studio_rules の範囲の外
	ErrSizeOutOfRange = errors.New("board: size is out of range")
	// 既定のルールの決まり。目標の魔力・ラウンドの上限・サイコロの目の最小は 1 以上、開始時の魔力・周回のボーナス・チェックポイントのボーナスは 0 以上、
	// 目標の魔力は開始時の魔力より大きい、サイコロの目の最大は最小以上
	ErrTargetMagicTooSmall                    = errors.New("board: target magic is too small")
	ErrStartingMagicTooSmall                  = errors.New("board: starting magic is too small")
	ErrLapBonusTooSmall                       = errors.New("board: lap bonus is too small")
	ErrCheckpointBonusTooSmall                = errors.New("board: checkpoint bonus is too small")
	ErrTargetMagicNotGreaterThanStartingMagic = errors.New("board: target magic is not greater than starting magic")
	ErrRoundLimitTooSmall                     = errors.New("board: round limit is too small")
	ErrDiceMinTooSmall                        = errors.New("board: dice min is too small")
	ErrDiceMaxLessThanDiceMin                 = errors.New("board: dice max is less than dice min")
	// 準備中のボードを使用不可にしようとした
	ErrInvalidStatusTransition = errors.New("board: invalid status transition")
	// 準備中でないボードの名前・レイアウト・既定のルールを変える、または削除しようとした
	ErrNotDraft = errors.New("board: not draft")
	// レイアウトの決まり（ValidateLayout）を守っていない。違反の一覧は *InvalidLayoutError
	ErrInvalidLayout = errors.New("board: invalid layout")
	// 公開の条件を満たしていない。理由は *NotPublishableError
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

// 画面が何を直せばよいか示せるよう、Readiness をそのまま持つ（理由の一覧は Readiness.Problems）
type NotPublishableError struct {
	Readiness Readiness
}

func (e *NotPublishableError) Error() string {
	return fmt.Sprintf("%s: bases=%d checkpoints=%d unreachable=%d unpaired=%d",
		ErrNotPublishable, e.Readiness.BaseCount(), e.Readiness.CheckpointCount, len(e.Readiness.UnreachableTiles), len(e.Readiness.UnpairedTiles))
}

func (e *NotPublishableError) Is(target error) bool {
	return target == ErrNotPublishable
}
