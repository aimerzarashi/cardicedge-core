package player

import (
	"errors"
	"strings"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

type ID int64

var entityType = domain.NewEntityType("player")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("player id must be positive")
	}
	return ID(value), nil
}

type Issuer string

func NewIssuer(value string) (Issuer, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("issuer must not be empty")
	}
	return Issuer(value), nil
}

func (i Issuer) String() string {
	return string(i)
}

// 中身を解釈しない（better-auth の内部 ID は外部の識別子としてだけ扱う方針）
type Subject string

func NewSubject(value string) (Subject, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("subject must not be empty")
	}
	return Subject(value), nil
}

func (s Subject) String() string {
	return string(s)
}

// プレイヤー 1 人が持てるデッキの数の上限（game_rules の player.max_decks。quota deck.per_player の上限の値）。1 以上。
// deck に置かないのは、deck の下の層（startergrant → npcavatar → npcdeck）が gamerules を読み、gamerules が deck を読むと循環するため。
// 「プレイヤーが持てる数」なので、持ち主の player に置くのが意味にも合う
type MaxDecks int

func NewMaxDecks(value int) (MaxDecks, error) {
	if value <= 0 {
		return 0, ErrMaxDecksNotPositive
	}
	return MaxDecks(value), nil
}

func (m MaxDecks) Int() int {
	return int(m)
}

// プレイヤー 1 人が持てるアバターの数の上限（game_rules の player.max_avatars。quota avatar.per_player の上限の値）。1 以上
type MaxAvatars int

func NewMaxAvatars(value int) (MaxAvatars, error) {
	if value <= 0 {
		return 0, ErrMaxAvatarsNotPositive
	}
	return MaxAvatars(value), nil
}

func (m MaxAvatars) Int() int {
	return int(m)
}
