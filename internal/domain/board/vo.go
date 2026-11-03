package board

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type ID int64

var entityType = domain.NewEntityType("board")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("board id must be positive")
	}
	return ID(value), nil
}

// カード名と同じ上限にそろえる（画面で同じ幅に収まり、決まりを 2 つ覚えずに済むため）
const MaxNameLength = card.MaxNameLength

type Name string

// 長さを正規化する前の値で数える理由は card.NewName と同じ
func NewName(value string) (Name, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ErrNameEmpty
	}
	if utf8.RuneCountInString(trimmed) > MaxNameLength {
		return "", ErrNameTooLong
	}
	return Name(trimmed), nil
}

func (n Name) String() string {
	return string(n)
}

// 配布デッキの状態（grantdeck.Status）と移り方は同じだが、型は共有しない。片方だけ移り方を変えたくなったときに
// 分けられなくなるため。準備中はレイアウトを自由に変えられ、公開の条件を満たさなくてよい。使用不可は新しい対戦に使わない。
// 公開中 → 準備中に戻せないのは、使われている間にレイアウトが変わるため
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusRetired   Status = "retired"
)

func NewStatus(value string) (Status, error) {
	status := Status(value)
	switch status {
	case StatusDraft, StatusPublished, StatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrStatusUnknown, value)
	}
}

func (s Status) String() string {
	return string(s)
}

// 公開した後の状態と、状態が変わるか。準備中・使用不可からは公開中にでき、公開中にもう一度公開しても変わらない
func (s Status) Publish() (Status, bool, error) {
	switch s {
	case StatusDraft, StatusRetired:
		return StatusPublished, true, nil
	case StatusPublished:
		return StatusPublished, false, nil
	default:
		return s, false, ErrStatusUnknown
	}
}

// 使用不可にした後の状態と、状態が変わるか。一度も公開していない準備中は使用不可にできない。使用不可にもう一度しても変わらない
func (s Status) Retire() (Status, bool, error) {
	switch s {
	case StatusPublished:
		return StatusRetired, true, nil
	case StatusRetired:
		return StatusRetired, false, nil
	case StatusDraft:
		return s, false, ErrInvalidStatusTransition
	default:
		return s, false, ErrStatusUnknown
	}
}

// 登録の直後の既定のルール（対戦を始めるとき、ここから変えられる前提の値）。登録のイベントが値を明示して持つので、
// 変えても過去のボードの値は変わらない。ラウンドの上限（30）は仮の値
const (
	DefaultTargetMagic     = 8000
	DefaultStartingMagic   = 600
	DefaultLapBonus        = 300
	DefaultCheckpointBonus = 100
	DefaultRoundLimit      = 30
	DefaultDiceMin         = 1
	DefaultDiceMax         = 6
)

// 対戦の既定のルール。効き方は対戦の決まりなので、ここでは値の形だけを決める
type DefaultRules struct {
	targetMagic     int
	startingMagic   int
	lapBonus        int
	checkpointBonus int
	roundLimit      int
	diceMin         int
	diceMax         int
}

// 上限は持たない（要るなら studio_rules に入れる）。違反は errors.Join でまとめて返し、呼ぶ側が errors.Is で項目ごとに分ける
func NewDefaultRules(targetMagic, startingMagic, lapBonus, checkpointBonus, roundLimit, diceMin, diceMax int) (DefaultRules, error) {
	var errs []error
	if targetMagic < 1 {
		errs = append(errs, ErrTargetMagicTooSmall)
	}
	if startingMagic < 0 {
		errs = append(errs, ErrStartingMagicTooSmall)
	}
	if lapBonus < 0 {
		errs = append(errs, ErrLapBonusTooSmall)
	}
	if checkpointBonus < 0 {
		errs = append(errs, ErrCheckpointBonusTooSmall)
	}
	if roundLimit < 1 {
		errs = append(errs, ErrRoundLimitTooSmall)
	}
	if diceMin < 1 {
		errs = append(errs, ErrDiceMinTooSmall)
	}
	// 下限を割った値と比べても意味がないので、下限を満たすときだけ比べる
	if startingMagic >= 0 && targetMagic <= startingMagic {
		errs = append(errs, ErrTargetMagicNotGreaterThanStartingMagic)
	}
	if diceMin >= 1 && diceMax < diceMin {
		errs = append(errs, ErrDiceMaxLessThanDiceMin)
	}
	if len(errs) > 0 {
		return DefaultRules{}, errors.Join(errs...)
	}
	return DefaultRules{
		targetMagic: targetMagic, startingMagic: startingMagic, lapBonus: lapBonus, checkpointBonus: checkpointBonus,
		roundLimit: roundLimit, diceMin: diceMin, diceMax: diceMax,
	}, nil
}

// 登録の直後の既定のルール。定数が決まりを満たすことはテストで確かめる
func InitialDefaultRules() DefaultRules {
	return DefaultRules{
		targetMagic: DefaultTargetMagic, startingMagic: DefaultStartingMagic,
		lapBonus: DefaultLapBonus, checkpointBonus: DefaultCheckpointBonus,
		roundLimit: DefaultRoundLimit, diceMin: DefaultDiceMin, diceMax: DefaultDiceMax,
	}
}

// 届いて拠点に戻ると勝ちになる魔力
func (r DefaultRules) TargetMagic() int {
	return r.targetMagic
}

func (r DefaultRules) StartingMagic() int {
	return r.startingMagic
}

// 拠点に戻ったときに増える魔力
func (r DefaultRules) LapBonus() int {
	return r.lapBonus
}

// チェックポイントを通ったときに増える魔力
func (r DefaultRules) CheckpointBonus() int {
	return r.checkpointBonus
}

// 上限に達したら、その時点で魔力の多い人が勝つ
func (r DefaultRules) RoundLimit() int {
	return r.roundLimit
}

// サイコロの目の最小
func (r DefaultRules) DiceMin() int {
	return r.diceMin
}

// サイコロの目の最大
func (r DefaultRules) DiceMax() int {
	return r.diceMax
}

type defaultRulesJSON struct {
	TargetMagic     int `json:"target_magic"`
	StartingMagic   int `json:"starting_magic"`
	LapBonus        int `json:"lap_bonus"`
	CheckpointBonus int `json:"checkpoint_bonus"`
	RoundLimit      int `json:"round_limit"`
	DiceMin         int `json:"dice_min"`
	DiceMax         int `json:"dice_max"`
}

func (r DefaultRules) MarshalJSON() ([]byte, error) {
	return json.Marshal(defaultRulesJSON{
		TargetMagic: r.targetMagic, StartingMagic: r.startingMagic, LapBonus: r.lapBonus, CheckpointBonus: r.checkpointBonus,
		RoundLimit: r.roundLimit, DiceMin: r.diceMin, DiceMax: r.diceMax,
	})
}

// 決まりは確かめない（読み出しを決まりの変更に巻き込まないため。決まりは書くときに NewDefaultRules が守る。Size と同じ）
func (r *DefaultRules) UnmarshalJSON(data []byte) error {
	var v defaultRulesJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*r = DefaultRules{
		targetMagic: v.TargetMagic, startingMagic: v.StartingMagic, lapBonus: v.LapBonus, checkpointBonus: v.CheckpointBonus,
		roundLimit: v.RoundLimit, diceMin: v.DiceMin, diceMax: v.DiceMax,
	}
	return nil
}
