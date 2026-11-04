package card

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

const MaxNameLength = 50

// Creature・Item・Spell で共有する。カードという集約は作らず、名前の重複だけを種類をまたいで防ぐ
type Name string

// 長さは正規化する前の値（表示する値）で数える。NFKC は「㍿」を「株式会社」にするように長くなる文字があり、
// 正規化した値で数えると、見た目の文字数と上限が食い違うため
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

// Creature・Item・Spell で共有する、カードの状態。準備中は自由に変えられてデッキに入れられず、公開中はデッキに入れられ、
// 使用不可は新しくデッキに入れられない（プレイヤーが持っているカードは使い続けられる）。
// 公開中 → 準備中に戻せないのは、デッキに入れた・game に写した中身が途中で変わるため。変えたいなら使用不可にして新しく作る
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusRetired   Status = "retired"
)

// DB の cards.status など、外から来た値を状態にする
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

// 使用不可にした後の状態と、状態が変わるか。一度も公開していない準備中のカードは、使用不可（使えなくした）ではなく
// 準備中のままなので、使用不可にできない。使用不可にもう一度しても変わらない
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

// 所持するカードの枚数。1 以上（0 枚のカードは持たない）
type OwnedQuantity int

func NewOwnedQuantity(value int) (OwnedQuantity, error) {
	if value <= 0 {
		return 0, ErrQuantityNotPositive
	}
	return OwnedQuantity(value), nil
}

func (q OwnedQuantity) Int() int {
	return int(q)
}

// 同じカードをデッキに入れられる上限の枚数（game_rules の deck.max_card_duplicates）。1 以上
type MaxDeckQuantity int

func NewMaxDeckQuantity(value int) (MaxDeckQuantity, error) {
	if value <= 0 {
		return 0, ErrQuantityNotPositive
	}
	return MaxDeckQuantity(value), nil
}

func (m MaxDeckQuantity) Int() int {
	return int(m)
}

// デッキに入れるカードの枚数。1 以上で、上限を超えない（デッキから外すことは枚数ではなく、コマンドの入力の側で表す）。
//
// 上限を確かめるのは作るとき（コマンド）だけで、保存済みのイベントを読み直すときは確かめない（JSON をそのまま読む）。
// ルールを変えても、すでにあるデッキを読めるようにするため。上限を引数で受け取るのは、確かめずに作れないようにするため
type DeckQuantity int

func NewDeckQuantity(value int, max MaxDeckQuantity) (DeckQuantity, error) {
	if value <= 0 {
		return 0, ErrQuantityNotPositive
	}
	if value > max.Int() {
		return 0, ErrDeckQuantityOverMax
	}
	return DeckQuantity(value), nil
}

func (q DeckQuantity) Int() int {
	return int(q)
}

// 差分を足した後の枚数。0 になったら remains が false（デッキから外れる）。デッキに無いカードの現在の枚数は、ゼロ値（0）で渡す。
// イベントを読み直して状態を作るときに使うため、上限は確かめない（NewDeckQuantity と同じ理由）
func (q DeckQuantity) Add(delta int) (next DeckQuantity, remains bool, err error) {
	total := q.Int() + delta
	switch {
	case total < 0:
		return 0, false, ErrQuantityNotPositive
	case total == 0:
		return 0, false, nil
	default:
		return DeckQuantity(total), true, nil
	}
}

// Creature・Item・Spell の集約の種別名。この 3 つは card を import するため、card から import できず（循環）、
// 種別名をここで宣言して各集約が使う。Ref が受け付ける種類を、ここで 1 か所に決めるため
var (
	CreatureEntityType = domain.NewEntityType("creature")
	ItemEntityType     = domain.NewEntityType("item")
	SpellEntityType    = domain.NewEntityType("spell")
)

// カードを指す、種類（集約の種別）と内部 ID の組。ほかの集約は同じ境界の内部 ID で指す決まりだが、カードは Creature・Item・Spell で
// 集約が分かれ、内部 ID だけではどのカードかが決まらないため。内部 ID は studio と game が別々に振るので、
// 持つ側（studio の集約か game の集約か）の境界のものを入れる
type Ref struct {
	EntityType domain.EntityType `json:"entity_type"`
	ID         int64             `json:"id"`
}

// カードの種類は Creature・Item・Spell だけ（ほかの集約を指さないように）。
// 保存済みのイベントを読み直すときは JSON のタグで読むだけで、ここは通さない（ルールを変えても読めるように）
func NewRef(entityType domain.EntityType, id int64) (Ref, error) {
	switch entityType {
	case CreatureEntityType, ItemEntityType, SpellEntityType:
	default:
		return Ref{}, fmt.Errorf("%w: %s", ErrNotCard, entityType)
	}
	if id <= 0 {
		return Ref{}, errors.New("card: ref id must be positive")
	}
	return Ref{EntityType: entityType, ID: id}, nil
}

// 種類、内部 ID の順。map の順は決まらないため、同じ操作から同じイベントを作るときに並べるのに使う
func (r Ref) Compare(other Ref) int {
	if r.EntityType != other.EntityType {
		if r.EntityType < other.EntityType {
			return -1
		}
		return 1
	}
	switch {
	case r.ID < other.ID:
		return -1
	case r.ID > other.ID:
		return 1
	default:
		return 0
	}
}
