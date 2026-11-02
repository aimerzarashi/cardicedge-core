package gamerules

import (
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

type ID int64

var entityType = domain.NewEntityType("game_rules")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("game rules id must be positive")
	}
	return ID(value), nil
}

// ゲーム全体のルールの値。何の決まりかで分け、どのサービスで使うかでは分けない（どの値もいずれ studio と game の両方で要る見込みのため）。
// カード名・デッキ名の長さは値の形の決まりなので、ここには入れない（それぞれの値オブジェクトの検証として残す）
type Rules struct {
	Player      Player      `json:"player"`
	Deck        Deck        `json:"deck"`
	StarterDeck StarterDeck `json:"starter_deck"`
	Match       Match       `json:"match"`
}

type Player struct {
	// 持てるデッキの数
	MaxDecks int `json:"max_decks"`
	// 持てるアバターの数
	MaxAvatars int `json:"max_avatars"`
}

// デッキ（プリセットデッキ・スターターデッキ・プレイヤーのデッキ）に共通の決まり
type Deck struct {
	// カードの合計の枚数（スターターデッキはちょうど、プリセットデッキは上限）
	TotalCardCount int `json:"total_card_count"`
	// 同じカードを入れられる枚数の上限
	MaxCardDuplicates int `json:"max_card_duplicates"`
}

// スターターデッキだけの決まり
type StarterDeck struct {
	// 公開中のスターターデッキの数の上限
	MaxPublished int `json:"max_published"`
}

// 対戦の決まり。盤面（ボード）や部屋によらず、ゲーム全体で同じ値を使う。
// 部屋の条件が読んで見せ、対戦が使う
type Match struct {
	// 対戦に参加できる人数の下限（この値を含む）
	MinPlayers int `json:"min_players"`
	// 対戦に参加できる人数の上限（この値を含む）
	MaxPlayers int `json:"max_players"`
	// 手札の上限の枚数
	MaxHandCards int `json:"max_hand_cards"`
}

var ErrInvalidRules = errors.New("game rules: every value must be at least 1, and a minimum must not exceed its maximum")

// 値の数は増えていくため、値ごとの型は作らず、ここで 1 以上をまとめて確かめる
func New(player Player, deck Deck, starterDeck StarterDeck, match Match) (Rules, error) {
	// map にしないのは、誤りが複数あるときに、どれを返すかが実行ごとに変わらないようにするため
	values := []struct {
		name  string
		value int
	}{
		{"player.max_decks", player.MaxDecks},
		{"player.max_avatars", player.MaxAvatars},
		{"deck.total_card_count", deck.TotalCardCount},
		{"deck.max_card_duplicates", deck.MaxCardDuplicates},
		{"starter_deck.max_published", starterDeck.MaxPublished},
		{"match.min_players", match.MinPlayers},
		{"match.max_players", match.MaxPlayers},
		{"match.max_hand_cards", match.MaxHandCards},
	}
	for _, v := range values {
		if v.value < 1 {
			return Rules{}, fmt.Errorf("%w: %s is %d", ErrInvalidRules, v.name, v.value)
		}
	}
	// 1 以上の確かめの後に置く（0 や負の値は、大小の比べより先に「1 以上」の誤りとして返すため）
	if match.MinPlayers > match.MaxPlayers {
		return Rules{}, fmt.Errorf("%w: match.max_players %d is below match.min_players %d", ErrInvalidRules, match.MaxPlayers, match.MinPlayers)
	}
	return Rules{Player: player, Deck: deck, StarterDeck: starterDeck, Match: match}, nil
}

// 作るときの値。値の変更は core#23 で作るまで、これが全体のルールになる。
// 作るとき（とテストの用意）のほかは、この値を直接読まず game_rules から読む（ここを読むと、値を変えても効かない所ができるため）
func Defaults() Rules {
	return Rules{
		Player:      Player{MaxDecks: 10, MaxAvatars: 5},
		Deck:        Deck{TotalCardCount: 40, MaxCardDuplicates: 4},
		StarterDeck: StarterDeck{MaxPublished: 2},
		Match:       Match{MinPlayers: 2, MaxPlayers: 4, MaxHandCards: 6},
	}
}
