package npcmindset

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
)

type ID int64

var entityType = domain.NewEntityType("npc_mindset")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("npc mindset id must be positive")
	}
	return ID(value), nil
}

// NPC のアバター（npcavatar.MaxNameLength）と今は同じ長さだが、参照せずに値を持つ。
// どちらかだけ変えたくなったときに分けられなくなるため
const MaxNameLength = 20

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

// NPC のデッキの状態（npcdeck.Status）と移り方は同じだが、型は共有しない。片方だけ移り方を変えたくなったときに
// 分けられなくなるため。準備中は名前・重みを自由に変えられる。使用不可は新しい対戦に使わない。
// 公開中 → 準備中に戻せないのは、使われている間に重みが変わるため
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusRetired   Status = "retired"
)

// DB の npc_mindsets.status など、外から来た値を状態にする
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

const (
	MinWeight = 0
	// 上限は画面で入れやすくするため（2026-11-18 オーナー。core#188）
	MaxWeight = 100
)

// 重み 1 つ。0〜100 の整数で、行動の種類の重みでは 0 はその種類を選ばない
type Weight int

func NewWeight(value int) (Weight, error) {
	if value < MinWeight || value > MaxWeight {
		return 0, ErrWeightOutOfRange
	}
	return Weight(value), nil
}

func (w Weight) Int() int {
	return int(w)
}

// 行動の種類ごとの重み（2026-11-17 オーナー。core#187）。対象（手札・領地等）には重みを付けず均等に選ぶ（対象に付けるのは、
// 点を付けて選ぶ強さの工夫に近いため）。思考パターンは種類のすべてを持つ
type ActionWeights map[matchaction.Action]Weight

// 登録したときの値。すべて 1 で、完全ランダムと同じ
func DefaultActionWeights() ActionWeights {
	weights := ActionWeights{}
	for _, action := range matchaction.Actions() {
		weights[action] = 1
	}
	return weights
}

// 行動の種類のすべてを過不足なく持つか
func (w ActionWeights) complete() bool {
	if len(w) != len(matchaction.Actions()) {
		return false
	}
	for _, action := range matchaction.Actions() {
		if _, ok := w[action]; !ok {
			return false
		}
	}
	return true
}

// 採点基準（2026-11-17 オーナー。core#190）。値は大きいほどよく、場面の候補の中で 0〜1 にそろえて重みを掛ける。
// 思考パターンが何を重く見るかの語彙なので、match より下の段のここに置く
type Criterion string

const (
	// 自分の総魔力 − 相手のうち一番高い総魔力
	CriterionTotalMagicLead Criterion = "total_magic_lead"
	// 自分の領地の数
	CriterionTerritoryCount Criterion = "territory_count"
	// 戦闘の候補だけ、戦闘の後にその領地を自分が持っている割合。戦闘にならない候補は 0
	CriterionBattleHold Criterion = "battle_hold"
	// 手持ちの魔力。当面は盤面の通行料の一番高い額で頭打ち
	CriterionMagicReserve Criterion = "magic_reserve"
)

// 外から来た値を採点基準にする。基準のすべて（Criteria）にない値は誤り
func NewCriterion(value string) (Criterion, error) {
	criterion := Criterion(value)
	for _, known := range Criteria() {
		if criterion == known {
			return criterion, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrCriterionUnknown, value)
}

// 採点基準の並び（計算の順。重みの map の順は不定なので、並びはここで固定する）
func Criteria() []Criterion {
	return []Criterion{CriterionTotalMagicLead, CriterionTerritoryCount, CriterionBattleHold, CriterionMagicReserve}
}

// 採点基準ごとの重み。思考パターンは基準のすべてを持つ
type CriterionWeights map[Criterion]Weight

// 登録したときの値。すべて 1
func DefaultCriterionWeights() CriterionWeights {
	weights := CriterionWeights{}
	for _, criterion := range Criteria() {
		weights[criterion] = 1
	}
	return weights
}

func (w CriterionWeights) complete() bool {
	if len(w) != len(Criteria()) {
		return false
	}
	for _, criterion := range Criteria() {
		if _, ok := w[criterion]; !ok {
			return false
		}
	}
	return true
}
