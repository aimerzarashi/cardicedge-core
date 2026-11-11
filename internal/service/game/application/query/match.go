package query

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
)

// 公開 ID に当たる対戦が、そのプレイヤーの対戦として無い（無い・参加していない対戦。存在を明かさないため区別しない）
var ErrMatchNotFound = errors.New("query: match not found")

type MatchParticipantKind string

const (
	MatchParticipantKindPlayer MatchParticipantKind = "player"
	MatchParticipantKindNPC    MatchParticipantKind = "npc"
)

// 対戦の段階。今の番の人が次にする操作を表す
type MatchPhase string

const (
	MatchPhaseDrawing             MatchPhase = "drawing"
	MatchPhaseDiscarding          MatchPhase = "discarding"
	MatchPhaseRolling             MatchPhase = "rolling"
	MatchPhaseChoosingPath        MatchPhase = "choosing_path"
	MatchPhaseDeploying           MatchPhase = "deploying"
	MatchPhaseInvading            MatchPhase = "invading"
	MatchPhaseAttacking           MatchPhase = "attacking"
	MatchPhaseDefending           MatchPhase = "defending"
	MatchPhaseRelinquishing       MatchPhase = "relinquishing"
	MatchPhaseTerritoryCommanding MatchPhase = "territory_commanding"
	MatchPhaseEnding              MatchPhase = "ending"
	MatchPhaseEnded               MatchPhase = "ended"
)

type MatchEndReason string

const (
	MatchEndReasonTargetMagic MatchEndReason = "target_magic"
	MatchEndReasonRoundLimit  MatchEndReason = "round_limit"
)

// 対戦の結果。勝者は参加者の番号（Match.Participants の何番目か）
type MatchOutcome struct {
	Winners []int
	Ranks   []int
	Reason  MatchEndReason
}

// 使えるスペル 1 枚。Targets は選べる対象のプレイヤー（参加者の番号）で、対象を選ばないスペルは空
type MatchSpellChoice struct {
	HandIndex int
	Targets   []int
	// 選べる対象のクリーチャーがいる領地の位置（クリーチャーを選ぶスペルだけ）
	TargetPositions []BoardPosition
}

// プレイヤーにかかっている呪い。効き方は種類ごとの値のどれか 1 つ（今は次のサイコロの目だけ）
type MatchCurse struct {
	// 次のサイコロの目（目を決める呪いのときだけ）
	FixedDice *int
	// 解けるまでのターン数
	RemainingTurns int
}

func fromCurse(curse *match.Curse) *MatchCurse {
	if curse == nil {
		return nil
	}
	converted := &MatchCurse{RemainingTurns: curse.Expiry.RemainingTurns}
	if curse.FixedDice != nil {
		value := curse.FixedDice.Value
		converted.FixedDice = &value
	}
	return converted
}

// 手札のカード 1 枚。名前と種類は写した cards から引く
type MatchCard struct {
	ID   uuid.UUID
	Name string
	Type CardType
	// 対戦の中（手札・引いた・捨てた）は始めたときの写しの性能、戦利品は写した cards の性能
	Cost   int
	Rarity string
	// クリーチャーだけの性能（クリーチャー以外は nil）。コスト・希少度と同じく、対戦の中は写しの性能
	Creature *CreatureSpec
	// アイテムだけの性能（アイテム以外は nil）。クリーチャーと同じく、対戦の中は写しの性能
	Item *ItemSpec
	// スペルだけの性能（スペル以外は nil）。クリーチャーと同じく、対戦の中は写しの性能
	Spell *SpellSpec
}

// 対戦の盤面の写し。始めた時点のボードの形（対戦が自分の写しを持つので、ボードのマスタが変わっても変わらない）
type MatchBoard struct {
	Width       int
	Height      int
	Tiles       []BoardTile
	Connections []BoardConnection
	Pairs       []BoardPair
}

// 対戦のルール。始めた時点の写し（全体のルール MatchRules とは別の値）
type MatchPlayRules struct {
	TargetMagic       int
	StartingMagic     int
	LapBonus          int
	CheckpointBonus   int
	RoundLimit        int
	DiceMin           int
	DiceMax           int
	BaseLandValue     int
	MaxHandCards      int
	StartingHandCards int
	// 土地のレベル・連鎖の数ごとの倍率と、通行料・手放すときの割合
	LandLevelMultipliers []int
	ChainMultipliers     []int
	TollPercentage       int
	RelinquishPercentage int
}

// 参加者 1 人。山札は並びも中身も見せず、枚数だけを持つ。TurnOrder は順番の何番目か（0 始まり）
type MatchParticipant struct {
	Kind     MatchParticipantKind
	Player   *uuid.UUID
	Avatar   RoomNamed
	Position BoardPosition
	Magic    int
	// 手持ちの魔力 ＋ 自分の領地の価値の合計
	TotalMagic    int
	Hand          []MatchCard
	DrawPileCount int
	// 捨て札（入った順）。使った・捨てた・倒されたカードで、誰でも見られる
	DiscardPile []MatchCard
	TurnOrder   int
	// 拠点を出てから印を付けたチェックポイントの位置
	PassedCheckpoints []BoardPosition
	// かかっている呪い（無ければ nil）
	Curse *MatchCurse
	// 戦利品を受け取ったか（プレイヤーだけ。NPC は受け取らないので、いつも false）
	RewardReceived bool
	// 受け取った戦利品のカード（受け取るまでは空）
	RewardCards []MatchCard
}

// 盤面に配置したクリーチャー 1 体。カードの性能は配置したときの写し、HitPoints は今の体力
type MatchDeployedCreature struct {
	ID        int
	Card      MatchCard
	HitPoints int
}

// 領地。Owner は持ち主の参加者の番号（Match.Participants の何番目か）
type MatchTerritory struct {
	Position BoardPosition
	Owner    int
	Creature MatchDeployedCreature
	Level    int
	// 今の価値（連鎖を含む）・他人が止まったときの通行料・手放したときに受け取る魔力
	Value           int
	Toll            int
	RelinquishMagic int
}

// 戦闘が終わるまでの、侵略しているクリーチャー。カードの性能は出したときの写し、HitPoints は今の体力
// 土地のレベル（空き地にも残る）
type MatchLandLevel struct {
	Position BoardPosition
	Level    int
}

// 領地のレベルを上げる選択肢 1 つ。Level は上げた後のレベル、Magic は払う魔力
type MatchLevelUpChoice struct {
	Position BoardPosition
	Level    int
	Magic    int
}

// クリーチャーを交換する選択肢 1 つ。HandIndex は手札の番号、Magic は払う魔力
type MatchExchangeChoice struct {
	Position  BoardPosition
	HandIndex int
	Magic     int
}

// クリーチャーを動かす選択肢 1 つ（元の領地と行き先）
type MatchMoveChoice struct {
	From BoardPosition
	To   BoardPosition
}

// 土地の属性を変える選択肢 1 つ。Element は変えた後の属性、Magic は払う魔力
type MatchElementChangeChoice struct {
	Position BoardPosition
	Element  BoardTileElement
	Magic    int
}

type MatchInvasion struct {
	Position  BoardPosition
	Card      MatchCard
	HitPoints int
}

// 戦利品に選べるパック。キーを選ぶ操作に渡す
type MatchRewardPack struct {
	Key  string
	Name string
}

// 対戦。read model を作らず、集約の状態から組み立てる。Participants は席の順
type Match struct {
	ID                 uuid.UUID
	Revision           int64
	Round              int
	Rules              MatchPlayRules
	Board              MatchBoard
	Participants       []MatchParticipant
	CurrentParticipant *int
	// 今操作を待っている参加者。守る段階だけは領地の持ち主で、ほかは今の番の人
	AwaitingParticipant *int
	Phase               MatchPhase
	// このターンに振った目。振る前は nil
	Dice           *int
	RemainingSteps int
	// 今の番の人が選べる行き先。分かれ道で選ぶ段階のときだけ中身がある
	PathChoices []BoardPosition
	// 今の番の人が使えるスペルの手札の番号。振る段階で、このターンにまだ使っていないときだけ中身がある
	SpellChoices []MatchSpellChoice
	// 今の番の人が配置できる手札の番号。配置の段階のときだけ中身がある
	DeployChoices []int
	// 今の番の人が侵略に出せる手札の番号。侵略を選ぶ段階のときだけ中身がある
	InvadeChoices []int
	// 操作を待っている人が戦闘に出せるアイテムの手札の番号。攻める段階と守る段階のときだけ中身がある
	ItemChoices []int
	// 今の番の人が手放せる領地の位置。手放す段階のときだけ中身がある
	RelinquishChoices []BoardPosition
	// レベルを上げた土地（空き地を含む。上げた順。無い土地はレベル 1）
	LandLevels []MatchLandLevel
	// 今の番の人が選べるレベルアップ。止まった後の段階（配置・侵略・領地コマンド）のときだけ中身がある
	LevelUpChoices []MatchLevelUpChoice
	// 今の番の人が選べるクリーチャーの交換。止まった後の段階（配置・侵略・領地コマンド）のときだけ中身がある
	ExchangeChoices []MatchExchangeChoice
	// 今の番の人が選べる土地の属性の変更。止まった後の段階（配置・侵略・領地コマンド）のときだけ中身がある
	ElementChangeChoices []MatchElementChangeChoice
	// 今の番の人が選べるクリーチャーの移動。止まった後の段階（配置・侵略・領地コマンド）のときだけ中身がある
	MoveChoices []MatchMoveChoice
	// 侵略してから戦闘が終わるまでだけある
	Invasion *MatchInvasion
	// 領地（できた順）
	Territories []MatchTerritory
	// 終わるまでは nil
	Outcome *MatchOutcome
	// 見ているプレイヤーが今選べる戦利品のパック。終わった後で、まだ受け取っていないときだけ中身がある
	RewardPacks []MatchRewardPack
	// 始めた部屋の公開用の ID と、その部屋で何戦目か（1 始まり）。対戦の集約は部屋を持たない（パッケージが循環するため）ので、
	// 部屋の read model から引く。部屋の投影が遅れて引けない間は nil
	RoomID *uuid.UUID
	Number *int
}

// 名前を引く元（公開 ID）。アバター・NPC のアバターの名前と、カードの名前・種類を、それぞれの read model から 1 回で読む
type MatchNames struct {
	Avatars    map[uuid.UUID]string
	NPCAvatars map[uuid.UUID]string
	Cards      map[uuid.UUID]MatchCard
}

type LoadMatchNamesFunc func(ctx context.Context, avatarIDs, npcAvatarIDs, cardIDs []uuid.UUID) (MatchNames, error)

// 対戦を、read model ではなくイベントストアから読み直した集約の状態から返す（対戦に read model を作らない。組み立ての決まりを 1 か所にするため）。
// 参加者のプレイヤーだけが読める
type MatchHandler struct {
	access     *MatchAccess
	entityRefs domain.EntityRefResolver
	loadNames  LoadMatchNamesFunc
	findRoom   FindRoomByMatchFunc
}

func NewMatchHandler(
	eventStore domain.EventStore[match.ID, match.Event, match.State],
	matches domain.Identifier[match.ID],
	players domain.Identifier[player.ID],
	entityRefs domain.EntityRefResolver,
	loadNames LoadMatchNamesFunc,
	findRoom FindRoomByMatchFunc,
) *MatchHandler {
	return &MatchHandler{access: NewMatchAccess(eventStore, matches, players), entityRefs: entityRefs, loadNames: loadNames, findRoom: findRoom}
}

// 参加していない対戦も、無いのと同じ ErrMatchNotFound にする
func (q *MatchHandler) Get(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (Match, error) {
	_, current, viewer, err := q.access.Open(ctx, playerID, matchID)
	if err != nil {
		return Match{}, err
	}
	found, err := q.build(ctx, matchID, current, viewer)
	if err != nil {
		return Match{}, err
	}
	roomID, number, ok, err := q.findRoom(ctx, matchID)
	if err != nil {
		return Match{}, err
	}
	if ok {
		found.RoomID, found.Number = &roomID, &number
	}
	return found, nil
}

func (q *MatchHandler) build(ctx context.Context, publicID uuid.UUID, current match.Match, viewer player.ID) (Match, error) {
	var refs []domain.EntityRef
	for _, participant := range current.Participants {
		switch {
		case participant.Participant.Player != nil:
			p := participant.Participant.Player
			refs = append(refs, domain.NewEntityRef(player.Types().EntityType, p.Player.Int64()), domain.NewEntityRef(avatar.Types().EntityType, p.Avatar.Int64()))
		case participant.Participant.NPC != nil:
			refs = append(refs, domain.NewEntityRef(npcavatar.Types().EntityType, participant.Participant.NPC.Avatar.Int64()))
		}
		for _, hand := range participant.Hand {
			refs = append(refs, domain.NewEntityRef(hand.EntityType, hand.ID))
		}
		for _, discarded := range participant.DiscardPile {
			refs = append(refs, domain.NewEntityRef(discarded.EntityType, discarded.ID))
		}
		if participant.Reward != nil {
			for _, reward := range participant.Reward.Cards {
				refs = append(refs, domain.NewEntityRef(reward.EntityType, reward.ID))
			}
		}
	}
	for _, deployed := range current.DeployedCreatures {
		refs = append(refs, domain.NewEntityRef(deployed.Card.EntityType, deployed.Card.ID))
	}
	if current.Invasion != nil {
		refs = append(refs, domain.NewEntityRef(current.Invasion.Card.EntityType, current.Invasion.Card.ID))
	}
	publicIDs, err := q.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return Match{}, err
	}
	publicIDOf := func(entityType domain.EntityType, id int64) (uuid.UUID, error) {
		value, ok := publicIDs[domain.NewEntityRef(entityType, id)]
		if !ok {
			return uuid.Nil, fmt.Errorf("query: %s %d of match has no public id", entityType, id)
		}
		return value, nil
	}
	var avatarIDs, npcAvatarIDs, cardIDs []uuid.UUID
	participants := make([]MatchParticipant, len(current.Participants))
	totals := current.TotalMagics()
	for i, participant := range current.Participants {
		converted := MatchParticipant{
			Position:   BoardPosition{X: participant.Position.X.Int(), Y: participant.Position.Y.Int()},
			TotalMagic: totals[i],
			Magic:      participant.Magic, DrawPileCount: len(participant.DrawPile), TurnOrder: turnOrderOf(current.TurnOrder, i),
			PassedCheckpoints: fromBoardPositions(participant.PassedCheckpoints),
			Curse:             fromCurse(participant.Curse),
		}
		switch {
		case participant.Participant.Player != nil:
			p := participant.Participant.Player
			converted.Kind = MatchParticipantKindPlayer
			playerPublicID, err := publicIDOf(player.Types().EntityType, p.Player.Int64())
			if err != nil {
				return Match{}, err
			}
			converted.Player = &playerPublicID
			if converted.Avatar.ID, err = publicIDOf(avatar.Types().EntityType, p.Avatar.Int64()); err != nil {
				return Match{}, err
			}
			avatarIDs = append(avatarIDs, converted.Avatar.ID)
		case participant.Participant.NPC != nil:
			var err error
			converted.Kind = MatchParticipantKindNPC
			if converted.Avatar.ID, err = publicIDOf(npcavatar.Types().EntityType, participant.Participant.NPC.Avatar.Int64()); err != nil {
				return Match{}, err
			}
			npcAvatarIDs = append(npcAvatarIDs, converted.Avatar.ID)
		default:
			return Match{}, fmt.Errorf("query: participant %d of match has neither player nor npc", i)
		}
		converted.Hand = make([]MatchCard, len(participant.Hand))
		for j, hand := range participant.Hand {
			cardID, err := publicIDOf(hand.EntityType, hand.ID)
			if err != nil {
				return Match{}, err
			}
			converted.Hand[j].ID = cardID
			cardIDs = append(cardIDs, cardID)
		}
		converted.DiscardPile = make([]MatchCard, len(participant.DiscardPile))
		for j, discarded := range participant.DiscardPile {
			cardID, err := publicIDOf(discarded.EntityType, discarded.ID)
			if err != nil {
				return Match{}, err
			}
			converted.DiscardPile[j].ID = cardID
			cardIDs = append(cardIDs, cardID)
		}
		converted.RewardCards = []MatchCard{}
		if participant.Reward != nil {
			converted.RewardReceived = true
			for _, reward := range participant.Reward.Cards {
				cardID, err := publicIDOf(reward.EntityType, reward.ID)
				if err != nil {
					return Match{}, err
				}
				converted.RewardCards = append(converted.RewardCards, MatchCard{ID: cardID})
				cardIDs = append(cardIDs, cardID)
			}
		}
		participants[i] = converted
	}
	deployedCardIDs := make(map[match.DeployedCreatureID]uuid.UUID, len(current.DeployedCreatures))
	for _, deployed := range current.DeployedCreatures {
		cardID, err := publicIDOf(deployed.Card.EntityType, deployed.Card.ID)
		if err != nil {
			return Match{}, err
		}
		deployedCardIDs[deployed.ID] = cardID
		cardIDs = append(cardIDs, cardID)
	}
	var invasionCardID uuid.UUID
	if current.Invasion != nil {
		if invasionCardID, err = publicIDOf(current.Invasion.Card.EntityType, current.Invasion.Card.ID); err != nil {
			return Match{}, err
		}
		cardIDs = append(cardIDs, invasionCardID)
	}
	names, err := q.loadNames(ctx, avatarIDs, npcAvatarIDs, cardIDs)
	if err != nil {
		return Match{}, err
	}
	// 手札の性能は、始めたときの写し（判断と同じ値）。名前は今の read model から（戦闘に関わらないため）
	specs := current.CardSpecs()
	for i := range participants {
		participant := &participants[i]
		source := names.Avatars
		if participant.Kind == MatchParticipantKindNPC {
			source = names.NPCAvatars
		}
		name, ok := source[participant.Avatar.ID]
		if !ok {
			return Match{}, fmt.Errorf("query: avatar %s of match is not found", participant.Avatar.ID)
		}
		participant.Avatar.Name = name
		for j := range participant.Hand {
			found, ok := names.Cards[participant.Hand[j].ID]
			if !ok {
				return Match{}, fmt.Errorf("query: card %s of match is not found", participant.Hand[j].ID)
			}
			spec, ok := specs[current.Participants[i].Hand[j]]
			if !ok {
				return Match{}, fmt.Errorf("query: card %s of match hand has no spec", found.ID)
			}
			participant.Hand[j] = withSpec(found, spec)
		}
		for j := range participant.DiscardPile {
			found, ok := names.Cards[participant.DiscardPile[j].ID]
			if !ok {
				return Match{}, fmt.Errorf("query: card %s of match is not found", participant.DiscardPile[j].ID)
			}
			spec, ok := specs[current.Participants[i].DiscardPile[j]]
			if !ok {
				return Match{}, fmt.Errorf("query: card %s of match discard pile has no spec", found.ID)
			}
			participant.DiscardPile[j] = withSpec(found, spec)
		}
		for j := range participant.RewardCards {
			found, ok := names.Cards[participant.RewardCards[j].ID]
			if !ok {
				return Match{}, fmt.Errorf("query: card %s of match reward is not found", participant.RewardCards[j].ID)
			}
			participant.RewardCards[j] = found
		}
	}
	phase, err := fromMatchPhase(current.Phase)
	if err != nil {
		return Match{}, err
	}
	result := Match{
		ID: publicID, Revision: current.Version, Round: current.Round, Rules: fromMatchPlayRules(current.Rules),
		Board: fromMatchBoard(current.Board), Participants: participants,
		Phase: phase, RemainingSteps: current.RemainingSteps, PathChoices: []BoardPosition{}, RewardPacks: []MatchRewardPack{},
		SpellChoices: []MatchSpellChoice{}, DeployChoices: []int{}, InvadeChoices: []int{}, ItemChoices: []int{}, RelinquishChoices: fromBoardPositions(current.RelinquishChoices()), Territories: []MatchTerritory{},
		LandLevels: []MatchLandLevel{}, LevelUpChoices: []MatchLevelUpChoice{}, ExchangeChoices: []MatchExchangeChoice{}, ElementChangeChoices: []MatchElementChangeChoice{},
		MoveChoices: []MatchMoveChoice{},
	}
	for _, choice := range current.MoveChoices() {
		result.MoveChoices = append(result.MoveChoices, MatchMoveChoice{
			From: NewBoardPosition(choice.From.X.Int(), choice.From.Y.Int()), To: NewBoardPosition(choice.To.X.Int(), choice.To.Y.Int()),
		})
	}
	for _, land := range current.LandLevels {
		result.LandLevels = append(result.LandLevels, MatchLandLevel{Position: NewBoardPosition(land.Position.X.Int(), land.Position.Y.Int()), Level: land.Level})
	}
	for _, choice := range current.ElementChangeChoices() {
		element, err := NewBoardTileElement(choice.Element.String())
		if err != nil {
			return Match{}, err
		}
		result.ElementChangeChoices = append(result.ElementChangeChoices, MatchElementChangeChoice{
			Position: NewBoardPosition(choice.Position.X.Int(), choice.Position.Y.Int()), Element: element, Magic: choice.Magic,
		})
	}
	for _, choice := range current.ExchangeChoices() {
		result.ExchangeChoices = append(result.ExchangeChoices, MatchExchangeChoice{
			Position: NewBoardPosition(choice.Position.X.Int(), choice.Position.Y.Int()), HandIndex: choice.HandIndex, Magic: choice.Magic,
		})
	}
	for _, choice := range current.LevelUpChoices() {
		result.LevelUpChoices = append(result.LevelUpChoices, MatchLevelUpChoice{
			Position: NewBoardPosition(choice.Position.X.Int(), choice.Position.Y.Int()), Level: choice.Level, Magic: choice.Magic,
		})
	}
	for _, territory := range current.Territories {
		deployed, ok := current.DeployedCreature(territory.Creature)
		if !ok {
			return Match{}, fmt.Errorf("query: creature %d of territory is not deployed", territory.Creature.Int())
		}
		found, ok := names.Cards[deployedCardIDs[deployed.ID]]
		if !ok {
			return Match{}, fmt.Errorf("query: card %s of deployed creature is not found", deployedCardIDs[deployed.ID])
		}
		base := deployed.Base
		card := withSpec(found, match.CardSpec{Cost: current.CardSpecs()[deployed.Card].Cost, Rarity: current.CardSpecs()[deployed.Card].Rarity, Creature: &base})
		result.Territories = append(result.Territories, MatchTerritory{
			Position: NewBoardPosition(territory.Position.X.Int(), territory.Position.Y.Int()), Owner: deployed.Owner.Int(),
			Creature: MatchDeployedCreature{ID: deployed.ID.Int(), Card: card, HitPoints: deployed.HitPoints.Int()},
			Level:    current.LandLevel(territory.Position), Value: current.TerritoryValue(territory), Toll: current.TerritoryToll(territory), RelinquishMagic: current.TerritoryRelinquishMagic(territory),
		})
	}
	for i, participant := range current.Participants {
		index, err := match.NewParticipantIndex(i)
		if err != nil {
			return Match{}, err
		}
		if p := participant.Participant.Player; p != nil && p.Player == viewer && current.CanChooseRewardPack(index) {
			for _, pack := range current.Rules.RewardPacks {
				result.RewardPacks = append(result.RewardPacks, MatchRewardPack{Key: string(pack.Key), Name: pack.Name})
			}
		}
	}
	if turn, ok := current.CurrentParticipant(); ok {
		index := turn.Int()
		result.CurrentParticipant = &index
	}
	if awaiting, ok := current.AwaitingParticipant(); ok {
		index := awaiting.Int()
		result.AwaitingParticipant = &index
	}
	if current.Invasion != nil {
		found, ok := names.Cards[invasionCardID]
		if !ok {
			return Match{}, fmt.Errorf("query: card %s of invasion is not found", invasionCardID)
		}
		base := current.Invasion.Base
		spec := current.CardSpecs()[current.Invasion.Card]
		position := current.Invasion.Position
		result.Invasion = &MatchInvasion{
			Position:  NewBoardPosition(position.X.Int(), position.Y.Int()),
			Card:      withSpec(found, match.CardSpec{Cost: spec.Cost, Rarity: spec.Rarity, Creature: &base}),
			HitPoints: current.Invasion.HitPoints.Int(),
		}
	}
	if current.Dice > 0 {
		dice := current.Dice
		result.Dice = &dice
	}
	if phase == MatchPhaseChoosingPath {
		result.PathChoices = fromBoardPositions(current.PathChoices())
	}
	for _, choice := range current.SpellChoices() {
		targets := make([]int, len(choice.Targets))
		for i, target := range choice.Targets {
			targets[i] = target.Int()
		}
		result.SpellChoices = append(result.SpellChoices, MatchSpellChoice{HandIndex: choice.HandIndex, Targets: targets, TargetPositions: fromBoardPositions(choice.TargetPositions)})
	}
	if choices := current.DeployChoices(); len(choices) > 0 {
		result.DeployChoices = choices
	}
	if choices := current.InvadeChoices(); len(choices) > 0 {
		result.InvadeChoices = choices
	}
	if choices := current.ItemChoices(); len(choices) > 0 {
		result.ItemChoices = choices
	}
	if current.Result != nil {
		outcome, err := fromMatchResult(*current.Result)
		if err != nil {
			return Match{}, err
		}
		result.Outcome = &outcome
	}
	return result, nil
}

// moving・battling は Decide の途中だけの段階で、対戦の最後の状態には出ない（版ごとの値は AwaitsOperation で先に除く）。出たらコードの誤り
func fromMatchPhase(phase match.Phase) (MatchPhase, error) {
	switch phase {
	case match.PhaseDrawing:
		return MatchPhaseDrawing, nil
	case match.PhaseDiscarding:
		return MatchPhaseDiscarding, nil
	case match.PhaseRolling:
		return MatchPhaseRolling, nil
	case match.PhaseChoosingPath:
		return MatchPhaseChoosingPath, nil
	case match.PhaseDeploying:
		return MatchPhaseDeploying, nil
	case match.PhaseInvading:
		return MatchPhaseInvading, nil
	case match.PhaseAttacking:
		return MatchPhaseAttacking, nil
	case match.PhaseDefending:
		return MatchPhaseDefending, nil
	case match.PhaseRelinquishing:
		return MatchPhaseRelinquishing, nil
	case match.PhaseTerritoryCommanding:
		return MatchPhaseTerritoryCommanding, nil
	case match.PhaseEnding:
		return MatchPhaseEnding, nil
	case match.PhaseEnded:
		return MatchPhaseEnded, nil
	default:
		return "", fmt.Errorf("query: unexpected match phase: %q", phase)
	}
}

func fromMatchResult(result match.Result) (MatchOutcome, error) {
	var reason MatchEndReason
	switch result.Reason {
	case match.EndReasonTargetMagic:
		reason = MatchEndReasonTargetMagic
	case match.EndReasonRoundLimit:
		reason = MatchEndReasonRoundLimit
	default:
		return MatchOutcome{}, fmt.Errorf("query: unknown match end reason: %q", result.Reason)
	}
	winners := make([]int, len(result.Winners))
	for i, winner := range result.Winners {
		winners[i] = winner.Int()
	}
	return MatchOutcome{Winners: winners, Ranks: slices.Clone(result.Ranks), Reason: reason}, nil
}

func fromBoardPositions(positions []board.Position) []BoardPosition {
	converted := make([]BoardPosition, len(positions))
	for i, position := range positions {
		converted[i] = NewBoardPosition(position.X.Int(), position.Y.Int())
	}
	return converted
}

func fromMatchPlayRules(rules match.Rules) MatchPlayRules {
	return MatchPlayRules{
		TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus, CheckpointBonus: rules.CheckpointBonus,
		RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax, BaseLandValue: rules.BaseLandValue,
		MaxHandCards: rules.MaxHandCards.Int(), StartingHandCards: rules.StartingHandCards.Int(),
		LandLevelMultipliers: slices.Clone(rules.LandLevelMultipliers), ChainMultipliers: slices.Clone(rules.ChainMultipliers),
		TollPercentage: rules.TollPercentage.Int(), RelinquishPercentage: rules.RelinquishPercentage.Int(),
	}
}

func fromMatchBoard(source match.Board) MatchBoard {
	position := func(p board.Position) BoardPosition {
		return NewBoardPosition(p.X.Int(), p.Y.Int())
	}
	result := MatchBoard{Width: source.Size.Width.Int(), Height: source.Size.Height.Int()}
	result.Tiles = make([]BoardTile, len(source.Tiles))
	for i, tile := range source.Tiles {
		result.Tiles[i] = BoardTile{X: tile.Position.X.Int(), Y: tile.Position.Y.Int(), Kind: BoardTileKind(tile.Kind), Element: BoardTileElement(tile.Element)}
	}
	result.Connections = make([]BoardConnection, len(source.Connections))
	for i, connection := range source.Connections {
		result.Connections[i] = BoardConnection{A: position(connection.A), B: position(connection.B)}
	}
	result.Pairs = make([]BoardPair, len(source.Pairs))
	for i, pair := range source.Pairs {
		result.Pairs[i] = BoardPair{A: position(pair.A), B: position(pair.B)}
	}
	return result
}

// 順番の何番目か。ParticipantIndex は席の番号なので、番号 i を持つものの位置を探す
func turnOrderOf(order []match.ParticipantIndex, seat int) int {
	return slices.IndexFunc(order, func(index match.ParticipantIndex) bool {
		return index.Int() == seat
	})
}

// 対戦の写しの性能で、read model から引いたカードの性能を置き換える
func withSpec(found MatchCard, spec match.CardSpec) MatchCard {
	found.Cost = spec.Cost.Int()
	found.Rarity = spec.Rarity.String()
	found.Creature = nil
	if spec.Creature != nil {
		found.Creature = &CreatureSpec{Elements: spec.Creature.Elements.Strings(), Attack: spec.Creature.Attack.Int(), HitPoints: spec.Creature.HitPoints.Int(), Movement: spec.Creature.Movement.Int(), ItemRestrictions: spec.Creature.ItemRestrictions.Strings()}
	}
	found.Item = nil
	if spec.Item != nil {
		found.Item = &ItemSpec{Category: spec.Item.Category.String(), AttackBonus: spec.Item.AttackBonus.Int(), HitPointsBonus: spec.Item.HitPointsBonus.Int()}
	}
	found.Spell = nil
	if spec.Spell != nil {
		found.Spell = &SpellSpec{Effect: fromSpellEffect(spec.Spell.Effect)}
	}
	return found
}

func fromSpellEffect(effect spell.Effect) SpellEffect {
	var converted SpellEffect
	if effect.DrawCards != nil {
		converted.DrawCards = &SpellDrawCards{Count: effect.DrawCards.Count.Int()}
	}
	if effect.GainMagic != nil {
		converted.GainMagic = &SpellGainMagic{PerLap: effect.GainMagic.PerLap.Int()}
	}
	if effect.RollDice != nil {
		converted.RollDice = &SpellRollDice{Count: effect.RollDice.Count.Int()}
	}
	if effect.FixedDiceCurse != nil {
		converted.FixedDiceCurse = &SpellFixedDiceCurse{Value: effect.FixedDiceCurse.Value.Int(), Turns: effect.FixedDiceCurse.Turns.Int()}
	}
	if effect.Damage != nil {
		converted.Damage = &SpellDamage{Amount: effect.Damage.Amount.Int()}
	}
	return converted
}
