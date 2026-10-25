package presetdeck

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
)

type ID int64

var entityType = domain.NewEntityType("preset_deck")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("starter deck id must be positive")
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

// 0 はデッキから外すことを表す。枚数の上限は値の形ではなく全体のルール（gamerules）なので、ここでは確かめない
// （保存済みのイベントから状態を作り直すとき、ルールが変わっていても読めるように）
type CardCount int

func NewCardCount(value int) (CardCount, error) {
	if value < 0 {
		return 0, ErrCardCountOutOfRange
	}
	return CardCount(value), nil
}

func (c CardCount) Int() int {
	return int(c)
}

// プリセットデッキに入れるカード。ほかの集約は内部 ID で指す決まりのため、種類（集約の種別）と内部 ID の組で持つ
// （内部 ID だけでは、どの種類のカードかを、引かないとわからないため）
type CardRef struct {
	entityType domain.EntityType
	id         int64
}

// カードの種類は Creature・Item・Spell だけ（ほかの集約を指さないように）
func NewCardRef(entityType domain.EntityType, id int64) (CardRef, error) {
	switch entityType {
	case creature.Types().EntityType, item.Types().EntityType, spell.Types().EntityType:
	default:
		return CardRef{}, fmt.Errorf("%w: %s", ErrNotCard, entityType)
	}
	if id <= 0 {
		return CardRef{}, errors.New("starter deck: card id must be positive")
	}
	return CardRef{entityType: entityType, id: id}, nil
}

func (r CardRef) EntityType() domain.EntityType {
	return r.entityType
}

func (r CardRef) ID() int64 {
	return r.id
}

type cardRefJSON struct {
	EntityType string `json:"entity_type"`
	ID         int64  `json:"id"`
}

func (r CardRef) MarshalJSON() ([]byte, error) {
	return json.Marshal(cardRefJSON{EntityType: r.entityType.String(), ID: r.id})
}

func (r *CardRef) UnmarshalJSON(data []byte) error {
	var v cardRefJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	ref, err := NewCardRef(domain.NewEntityType(v.EntityType), v.ID)
	if err != nil {
		return err
	}
	*r = ref
	return nil
}
