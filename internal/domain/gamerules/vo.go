package gamerules

import (
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
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
	Player Player `json:"player"`
	Deck   Deck   `json:"deck"`
	Match  Match  `json:"match"`
}

// 値の型は意味の持ち主の側に置き、ここは組み合わせて持つだけ（部品の決まりだけの構造体なので、Player・Deck・Match に生成関数は無い）
type Player struct {
	// 持てるデッキの数
	MaxDecks player.MaxDecks `json:"max_decks"`
	// 持てるアバターの数
	MaxAvatars player.MaxAvatars `json:"max_avatars"`
}

// デッキ（プリセットデッキ・配布デッキ・プレイヤーのデッキ）に共通の決まり
type Deck struct {
	// カードの合計の枚数（配布デッキはちょうど、プリセットデッキは上限）
	TotalCardCount card.DeckTotalCount `json:"total_card_count"`
	// 同じカードを入れられる枚数の上限
	MaxCardDuplicates card.MaxDeckQuantity `json:"max_card_duplicates"`
}

// 対戦の決まり。盤面（ボード）や部屋によらず、ゲーム全体で同じ値を使う。
// 部屋の条件が読んで見せ、対戦が使う
type Match struct {
	// 対戦に参加できる人数の下限（この値を含む）
	MinPlayers MinPlayers `json:"min_players"`
	// 対戦に参加できる人数の上限（この値を含む）
	MaxPlayers MaxPlayers `json:"max_players"`
	// 手札の上限の枚数
	MaxHandCards MaxHandCards `json:"max_hand_cards"`
	// 対戦の始めに配る手札の枚数
	StartingHandCards StartingHandCards `json:"starting_hand_cards"`
	// 順位（1 位から）ごとの戦利品のカードの枚数。順位より値が少なければ、下の順位は最後の値を使う
	RewardCardsByRank RewardCardsByRank `json:"reward_cards_by_rank"`
}

// 対戦の値の型は gamerules に置く。対戦の集約（match）は参加者のデッキを写すため npcdeck を読み、npcdeck が gamerules を読むので、match に置くとパッケージが循環する（core#71）
var ErrMatchValueNotPositive = errors.New("game rules: match value must be at least 1")

// 対戦に参加できる人数の下限。1 以上
type MinPlayers int

func NewMinPlayers(value int) (MinPlayers, error) {
	if value < 1 {
		return 0, fmt.Errorf("%w: min players is %d", ErrMatchValueNotPositive, value)
	}
	return MinPlayers(value), nil
}

func (m MinPlayers) Int() int {
	return int(m)
}

// 対戦に参加できる人数の上限。1 以上
type MaxPlayers int

func NewMaxPlayers(value int) (MaxPlayers, error) {
	if value < 1 {
		return 0, fmt.Errorf("%w: max players is %d", ErrMatchValueNotPositive, value)
	}
	return MaxPlayers(value), nil
}

func (m MaxPlayers) Int() int {
	return int(m)
}

// 手札の上限の枚数。1 以上
type MaxHandCards int

func NewMaxHandCards(value int) (MaxHandCards, error) {
	if value < 1 {
		return 0, fmt.Errorf("%w: max hand cards is %d", ErrMatchValueNotPositive, value)
	}
	return MaxHandCards(value), nil
}

func (m MaxHandCards) Int() int {
	return int(m)
}

// 対戦の始めに配る手札の枚数。1 以上
type StartingHandCards int

func NewStartingHandCards(value int) (StartingHandCards, error) {
	if value < 1 {
		return 0, fmt.Errorf("%w: starting hand cards is %d", ErrMatchValueNotPositive, value)
	}
	return StartingHandCards(value), nil
}

func (s StartingHandCards) Int() int {
	return int(s)
}

// 戦利品のカードの枚数（1 つの順位の分）。1 以上
type RewardCards int

func NewRewardCards(value int) (RewardCards, error) {
	if value < 1 {
		return 0, fmt.Errorf("%w: reward cards is %d", ErrMatchValueNotPositive, value)
	}
	return RewardCards(value), nil
}

func (r RewardCards) Int() int {
	return int(r)
}

// 順位ごとの戦利品の枚数（1 位から）。1 つ以上
type RewardCardsByRank []RewardCards

var ErrRewardCardsByRankEmpty = errors.New("game rules: reward cards by rank must not be empty")

func NewRewardCardsByRank(values ...int) (RewardCardsByRank, error) {
	if len(values) == 0 {
		return nil, ErrRewardCardsByRankEmpty
	}
	counts := make(RewardCardsByRank, len(values))
	for i, value := range values {
		count, err := NewRewardCards(value)
		if err != nil {
			return nil, err
		}
		counts[i] = count
	}
	return counts, nil
}

var ErrInvalidRules = errors.New("game rules: a minimum must not exceed its maximum")

// 項目をまたぐ決まり（人数の最小 ≤ 最大、配る手札 ≤ 手札の上限・デッキの枚数）があるため、生成関数を残す。1 以上は各値の生成関数が確かめる
func New(player Player, deck Deck, match Match) (Rules, error) {
	var errs []error
	if match.MinPlayers.Int() > match.MaxPlayers.Int() {
		errs = append(errs, fmt.Errorf("%w: match.max_players %d is below match.min_players %d", ErrInvalidRules, match.MaxPlayers, match.MinPlayers))
	}
	if match.StartingHandCards.Int() > match.MaxHandCards.Int() {
		errs = append(errs, fmt.Errorf("%w: match.starting_hand_cards %d is above match.max_hand_cards %d", ErrInvalidRules, match.StartingHandCards, match.MaxHandCards))
	}
	if match.StartingHandCards.Int() > deck.TotalCardCount.Int() {
		errs = append(errs, fmt.Errorf("%w: match.starting_hand_cards %d is above deck.total_card_count %d", ErrInvalidRules, match.StartingHandCards, deck.TotalCardCount))
	}
	// 保存済みの JSON を読むときは通らないので、足す前に作られた全体のルールは空のまま読める。使う側（対戦を始める）が空を断る
	if len(match.RewardCardsByRank) == 0 {
		errs = append(errs, ErrRewardCardsByRankEmpty)
	}
	if len(errs) > 0 {
		return Rules{}, errors.Join(errs...)
	}
	return Rules{Player: player, Deck: deck, Match: match}, nil
}

// 作るときの値。値の変更は core#23 で作るまで、これが全体のルールになる。
// 作るとき（とテストの用意）のほかは、この値を直接読まず game_rules から読む（ここを読むと、値を変えても効かない所ができるため）
func Defaults() Rules {
	// 定数で失敗しないため、呼ぶ側に error を返さず panic にする
	must := func(err error) {
		if err != nil {
			panic(err)
		}
	}
	maxDecks, err := player.NewMaxDecks(10)
	must(err)
	maxAvatars, err := player.NewMaxAvatars(5)
	must(err)
	totalCardCount, err := card.NewDeckTotalCount(40)
	must(err)
	maxCardDuplicates, err := card.NewMaxDeckQuantity(4)
	must(err)
	minPlayers, err := NewMinPlayers(2)
	must(err)
	maxPlayers, err := NewMaxPlayers(4)
	must(err)
	maxHandCards, err := NewMaxHandCards(6)
	must(err)
	// 手札の上限 6 で、最初のターンに 1 枚引くとちょうど上限になる
	startingHandCards, err := NewStartingHandCards(5)
	must(err)
	// 仮の値（目標の魔力と順位で決める式は次のマイルストーン）。4 位以下は 1 枚
	rewardCardsByRank, err := NewRewardCardsByRank(3, 2, 1, 1)
	must(err)
	return Rules{
		Player: Player{MaxDecks: maxDecks, MaxAvatars: maxAvatars},
		Deck:   Deck{TotalCardCount: totalCardCount, MaxCardDuplicates: maxCardDuplicates},
		Match: Match{
			MinPlayers: minPlayers, MaxPlayers: maxPlayers, MaxHandCards: maxHandCards, StartingHandCards: startingHandCards,
			RewardCardsByRank: rewardCardsByRank,
		},
	}
}
