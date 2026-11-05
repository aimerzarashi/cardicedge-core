package match

import (
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type ID int64

var entityType = domain.NewEntityType("match")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("match id must be positive")
	}
	return ID(value), nil
}

// 参加者の何番目か（席の順。0 始まり）。対戦の中では、参加者をこの番号で指す
type ParticipantIndex int

func NewParticipantIndex(value int) (ParticipantIndex, error) {
	if value < 0 {
		return 0, errors.New("participant index must not be negative")
	}
	return ParticipantIndex(value), nil
}

func (i ParticipantIndex) Int() int {
	return int(i)
}

// 乱数の元（順番と山札を混ぜる）。Decide の中で引かず、time.Now と同じく外から受け取る（テストで並びを固定するため）。
// 混ぜた結果をコマンドで渡す形にしないのは、山札の並べ方（カードを枚数の分だけ並べる）を Decide の 1 か所に置くため。math/rand/v2 の *rand.Rand が満たす
type Random interface {
	Shuffle(n int, swap func(i, j int))
	// 0 以上 n 未満の整数（サイコロの目）
	IntN(n int) int
}

// 対戦の段階。イベントは事実だけを持ち、段階のイベントは作らず Evolve が状態から決める
type Phase string

const (
	PhaseDrawing      Phase = "drawing"
	PhaseDiscarding   Phase = "discarding"
	PhaseRolling      Phase = "rolling"
	PhaseChoosingPath Phase = "choosing_path"
	PhaseEnding       Phase = "ending"
	PhaseEnded        Phase = "ended"
	// 進む途中だけの段階。Decide は進み終えるまで続けるので、保存された最後の状態が moving になることは無い
	PhaseMoving Phase = "moving"
)

type EndReason string

const (
	EndReasonTargetMagic EndReason = "target_magic"
	EndReasonRoundLimit  EndReason = "round_limit"
)

// 対戦の結果。勝者は魔力が同じならすべて（参加者の番号の昇順）
type Result struct {
	Winners []ParticipantIndex `json:"winners"`
	Reason  EndReason          `json:"reason"`
}

// デッキのカード 1 種類。始めた時点のデッキの写し
type DeckCard struct {
	Card     card.Ref          `json:"card"`
	Quantity card.DeckQuantity `json:"quantity"`
}

type PlayerParticipant struct {
	Player player.ID `json:"player"`
	Avatar avatar.ID `json:"avatar"`
	Deck   deck.ID   `json:"deck"`
}

type NPCParticipant struct {
	Avatar npcavatar.ID `json:"avatar"`
	Deck   npcdeck.ID   `json:"deck"`
}

// 参加者 1 人。プレイヤーか NPC かのどちらか 1 つだけを持つ（項目をまたぐ決まりがあるので生成関数でだけ作る）。
// デッキは ID のほかにカードも写す（始めた後にデッキを変えても対戦に響かないため）
type Participant struct {
	Player *PlayerParticipant `json:"player,omitempty"`
	NPC    *NPCParticipant    `json:"npc,omitempty"`
	Cards  []DeckCard         `json:"cards"`
}

func NewPlayerParticipant(playerID player.ID, avatarID avatar.ID, deckID deck.ID, cards []DeckCard) Participant {
	return Participant{Player: &PlayerParticipant{Player: playerID, Avatar: avatarID, Deck: deckID}, Cards: cards}
}

func NewNPCParticipant(avatarID npcavatar.ID, deckID npcdeck.ID, cards []DeckCard) Participant {
	return Participant{NPC: &NPCParticipant{Avatar: avatarID, Deck: deckID}, Cards: cards}
}

// 山札にする並び（カードを枚数の分だけ並べる。混ぜる前）
func (p Participant) deckCards() []card.Ref {
	var refs []card.Ref
	for _, entry := range p.Cards {
		for range entry.Quantity.Int() {
			refs = append(refs, entry.Card)
		}
	}
	return refs
}

// 始めた時点のボードの写し。⑥ の動きに使い、始めた後にマスタを変えても対戦に響かないよう、形もすべて写す
type Board struct {
	ID          board.ID           `json:"id"`
	Size        board.Size         `json:"size"`
	Tiles       []board.Tile       `json:"tiles"`
	Connections []board.Connection `json:"connections"`
	Pairs       []board.Pair       `json:"pairs,omitempty"`
}

// 拠点の位置。公開したボードは拠点をちょうど 1 つ持つ（公開の条件）ので、見つからないのは写し方の誤り
func (b Board) base() (board.Position, bool) {
	for _, tile := range b.Tiles {
		if tile.Kind == board.TileKindBase {
			return tile.Position, true
		}
	}
	return board.Position{}, false
}

// 対戦のルール。部屋の対戦ルール（ボードの既定のルールと同じ 7 つの値）と、全体のルールの手札の決まりの写し（始めた後にルールを変えても対戦に響かないため）。
// 7 つの値は board.DefaultRules で受け取る（目の範囲が 2 種類以上であることなどを、確かめずに作れないように）
type Rules struct {
	TargetMagic       int                         `json:"target_magic"`
	StartingMagic     int                         `json:"starting_magic"`
	LapBonus          int                         `json:"lap_bonus"`
	CheckpointBonus   int                         `json:"checkpoint_bonus"`
	RoundLimit        int                         `json:"round_limit"`
	DiceMin           int                         `json:"dice_min"`
	DiceMax           int                         `json:"dice_max"`
	MaxHandCards      gamerules.MaxHandCards      `json:"max_hand_cards"`
	StartingHandCards gamerules.StartingHandCards `json:"starting_hand_cards"`
}

func NewRules(values board.DefaultRules, maxHandCards gamerules.MaxHandCards, startingHandCards gamerules.StartingHandCards) Rules {
	return Rules{
		TargetMagic: values.TargetMagic, StartingMagic: values.StartingMagic, LapBonus: values.LapBonus,
		CheckpointBonus: values.CheckpointBonus, RoundLimit: values.RoundLimit, DiceMin: values.DiceMin, DiceMax: values.DiceMax,
		MaxHandCards: maxHandCards, StartingHandCards: startingHandCards,
	}
}
