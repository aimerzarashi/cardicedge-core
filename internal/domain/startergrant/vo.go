package startergrant

import (
	"errors"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type ID int64

var entityType = domain.NewEntityType("starter_grant")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("starter grant id must be positive")
	}
	return ID(value), nil
}

// 何番目のデッキか（1 から、配るデッキの数まで）。デッキの集約の一意な key と、配布の記録の段に使う。
// 配るデッキの数は公開中の配布デッキの数で決まり、値の形では上限を持てないため、配るデッキの数との照合は Decide でする
type DeckNumber int

func NewDeckNumber(value int) (DeckNumber, error) {
	if value < 1 {
		return 0, ErrInvalidDeckNumber
	}
	return DeckNumber(value), nil
}

func (n DeckNumber) Int() int {
	return int(n)
}

type Deck struct {
	Name  DeckName   `json:"name"`
	Cards []DeckCard `json:"cards"`
}

// 配るデッキのカードと枚数（開始のときに写したもの）。deck.Card を使わないのは、deck が startergrant を参照し、
// 逆向きに参照すると循環するため（DeckName と同じ）
type DeckCard struct {
	Card     card.Ref          `json:"card"`
	Quantity card.DeckQuantity `json:"quantity"`
}

// 配る所持カードと枚数。デッキのカードのうち多いほうの枚数から Decide が計算する
type CollectionCard struct {
	Card     card.Ref           `json:"card"`
	Quantity card.OwnedQuantity `json:"quantity"`
}

// これから作るデッキに付ける名前（配布を始めたときに、マスタのデッキの名前を写したもの）。deck.Name を使わないのは、deck が
// startergrant（配布の ID・何番目か）を参照し、逆向きに参照すると循環するため。決まりは deck.NewName と同じにし、作るときに変える
const MaxDeckNameLength = 50

type DeckName string

func NewDeckName(value string) (DeckName, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ErrDeckNameEmpty
	}
	if utf8.RuneCountInString(trimmed) > MaxDeckNameLength {
		return "", ErrDeckNameTooLong
	}
	return DeckName(trimmed), nil
}

func (n DeckName) String() string {
	return string(n)
}
