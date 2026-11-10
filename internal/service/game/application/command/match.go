package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

// 部屋の MatchStartedEvent を受けて、対戦を始めるポリシー。部屋の命令の中で作らず、イベントを受けて別のトランザクションで作る
// （1 つのトランザクションで変える集約を 1 つにするため）
type MatchStartHandler struct {
	rooms           domain.EventStore[room.ID, room.Event, room.State]
	matches         domain.EventStore[match.ID, match.Event, match.State]
	getRules        rules.GetRulesFunc
	getBoard        master.GetMatchBoardFunc
	getDeckCards    master.GetPlayerDeckCardsFunc
	getNPCDeckCards master.GetNPCDeckCardsFunc
	getCardSpecs    master.GetCardSpecsFunc
	// 乱数の元は対戦ごとに新しく作る（再実行で同じ並びになるとは限らないが、保存されるのは Decide が出したイベントだけで、
	// 二重に動いても対戦は 1 つしかできない）。テストは固定した乱数を返す関数を渡す
	newRandom func() match.Random
	now       func() time.Time
}

func NewMatchStartHandler(
	rooms domain.EventStore[room.ID, room.Event, room.State],
	matches domain.EventStore[match.ID, match.Event, match.State],
	getRules rules.GetRulesFunc,
	getBoard master.GetMatchBoardFunc,
	getDeckCards master.GetPlayerDeckCardsFunc,
	getNPCDeckCards master.GetNPCDeckCardsFunc,
	getCardSpecs master.GetCardSpecsFunc,
	newRandom func() match.Random,
	now func() time.Time,
) *MatchStartHandler {
	return &MatchStartHandler{
		rooms: rooms, matches: matches, getRules: getRules, getBoard: getBoard, getDeckCards: getDeckCards,
		getNPCDeckCards: getNPCDeckCards, getCardSpecs: getCardSpecs, newRandom: newRandom, now: now,
	}
}

// 購読する部屋のほかのイベントは何もしない
func (c *MatchStartHandler) Handle(ctx context.Context, event any) error {
	if e, ok := event.(*room.MatchStartedEvent); ok {
		return c.Start(ctx, e.GetMeta().EntityID, e.Match)
	}
	return nil
}

// 部屋の条件の写し（席の順の参加者・デッキのカード・ボードの形・ルール）から対戦を始める。ID は部屋の命令が取ったものを使う
// （部屋から対戦をすぐ引けるようにするため）。もう始まっていれば何もしない（ポリシーは二度動くことがある）。
// 部屋の条件が欠けている・写していないものがある失敗は、待てば直るもの（写しの遅れ）か、起きないはずの誤りとして返す
func (c *MatchStartHandler) Start(ctx context.Context, roomID room.ID, matchID match.ID) error {
	state, err := c.rooms.Load(ctx, roomID)
	if err != nil {
		return err
	}
	current, ok := state.(room.Room)
	if !ok || current.MatchRules == nil {
		return fmt.Errorf("match start: room %d has no conditions: %w", roomID, room.ErrInvalidState)
	}
	gameRules, found, err := c.getRules(ctx)
	if err != nil {
		return err
	}
	if !found {
		return rules.ErrNotPublished
	}
	// 戦利品の枚数を足す前に写した全体のルールは、枚数が空のまま。既定の枚数に黙って戻さず（写し忘れに気づけない）、写し直せば直る失敗にする
	if len(gameRules.Match.RewardCardsByRank) == 0 {
		return fmt.Errorf("match start: game rules have no reward cards by rank: %w", rules.ErrNotPublished)
	}
	if len(gameRules.Match.LandLevelMultipliers) == 0 || len(gameRules.Match.ChainMultipliers) == 0 {
		return fmt.Errorf("match start: game rules have no land multipliers: %w", rules.ErrNotPublished)
	}
	participants := make([]match.Participant, len(current.Seats))
	for i, seat := range current.Seats {
		if participants[i], err = c.participant(ctx, seat, gameRules.Deck.MaxCardDuplicates); err != nil {
			return fmt.Errorf("match start: seat %d: %w", i, err)
		}
	}
	snapshot, err := c.getBoard(ctx, current.MatchRules.Board)
	if err != nil {
		return err
	}
	values := current.MatchRules
	defaultRules, err := board.NewDefaultRules(values.TargetMagic, values.StartingMagic, values.LapBonus, values.CheckpointBonus,
		values.RoundLimit, values.DiceMin, values.DiceMax, values.BaseLandValue)
	if err != nil {
		return err
	}
	cmd := &match.StartCommand{
		ID: matchID, Participants: participants, Board: snapshot,
		Rules: match.NewRules(defaultRules, gameRules.Match.MaxHandCards, gameRules.Match.StartingHandCards, gameRules.Match.RewardCardsByRank, match.DefaultRewardPacks(),
			gameRules.Match.LandLevelMultipliers, gameRules.Match.ChainMultipliers, gameRules.Match.TollPercentage, gameRules.Match.RelinquishPercentage,
			gameRules.Match.TerrainBonusPerLevel, gameRules.Match.AdjacentTerritoryBonus,
			gameRules.Match.LandElementChangeMultiplier, gameRules.Match.MultiLandElementChangeMultiplier),
	}
	random := c.newRandom()
	_, err = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		existing, err := c.matches.Load(ctx, matchID)
		if err != nil {
			return struct{}{}, err
		}
		events, err := match.Decide(existing, cmd, c.now(), random)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.matches.Save(ctx, events)
	})
	if errors.Is(err, match.ErrAlreadyCreated) {
		return nil
	}
	return err
}

func (c *MatchStartHandler) participant(ctx context.Context, seat room.SeatState, maxDuplicates card.MaxDeckQuantity) (match.Participant, error) {
	switch {
	case seat.Seat.Kind == room.SeatKindPlayer && seat.Choice != nil && seat.Choice.Player != nil:
		source, err := c.getDeckCards(ctx, seat.Choice.Player.Deck)
		if err != nil {
			return match.Participant{}, err
		}
		cards, err := c.deckCards(ctx, source, maxDuplicates)
		if err != nil {
			return match.Participant{}, err
		}
		return match.NewPlayerParticipant(seat.Seat.Player, seat.Choice.Player.Avatar, seat.Choice.Player.Deck, cards), nil
	case seat.Seat.Kind == room.SeatKindNPC && seat.Choice != nil && seat.Choice.NPC != nil:
		source, err := c.getNPCDeckCards(ctx, seat.Choice.NPC.Deck)
		if err != nil {
			return match.Participant{}, err
		}
		cards, err := c.deckCards(ctx, source, maxDuplicates)
		if err != nil {
			return match.Participant{}, err
		}
		return match.NewNPCParticipant(seat.Choice.NPC.Avatar, seat.Choice.NPC.Deck, cards), nil
	default:
		return match.Participant{}, room.ErrConditionsNotDecided
	}
}

// デッキのカードに、そのときの性能を写す（studio の値をそのまま信じず、domain の値の決まりで確かめてから写す）
func (c *MatchStartHandler) deckCards(ctx context.Context, source []master.DeckCard, maxDuplicates card.MaxDeckQuantity) ([]match.DeckCard, error) {
	refs := make([]card.Ref, len(source))
	for i, entry := range source {
		refs[i] = entry.Card
	}
	specs, err := c.getCardSpecs(ctx, refs)
	if err != nil {
		return nil, err
	}
	cards := make([]match.DeckCard, len(source))
	for i, entry := range source {
		quantity, err := card.NewDeckQuantity(entry.Count, maxDuplicates)
		if err != nil {
			return nil, fmt.Errorf("card %s/%d: %w", entry.Card.EntityType, entry.Card.ID, err)
		}
		spec, err := cardSpec(entry.Card, specs)
		if err != nil {
			return nil, err
		}
		cards[i] = match.DeckCard{Card: entry.Card, Quantity: quantity, Spec: spec}
	}
	return cards, nil
}

func cardSpec(ref card.Ref, specs map[card.Ref]master.CardSpec) (match.CardSpec, error) {
	source, ok := specs[ref]
	if !ok {
		return match.CardSpec{}, fmt.Errorf("card %s/%d has no spec", ref.EntityType, ref.ID)
	}
	cost, err := card.NewCost(source.Cost)
	if err != nil {
		return match.CardSpec{}, fmt.Errorf("card %s/%d: %w", ref.EntityType, ref.ID, err)
	}
	rarity, err := card.NewRarity(source.Rarity)
	if err != nil {
		return match.CardSpec{}, fmt.Errorf("card %s/%d: %w", ref.EntityType, ref.ID, err)
	}
	spec := match.CardSpec{Cost: cost, Rarity: rarity}
	// クリーチャーの性能が無いのは、cards.metadata の読み出しの誤り（読む側が弾く）だが、ここでも黙って空の性能を写さない
	if ref.EntityType == card.CreatureEntityType {
		if source.Creature == nil {
			return match.CardSpec{}, fmt.Errorf("card %s/%d has no creature spec", ref.EntityType, ref.ID)
		}
		creatureSpec, err := newCreatureSpec(source.Creature)
		if err != nil {
			return match.CardSpec{}, fmt.Errorf("card %s/%d: %w", ref.EntityType, ref.ID, err)
		}
		spec.Creature = &creatureSpec
	}
	// クリーチャーと同じく、アイテムの性能が無いのを黙って空の性能にしない
	if ref.EntityType == card.ItemEntityType {
		if source.Item == nil {
			return match.CardSpec{}, fmt.Errorf("card %s/%d has no item spec", ref.EntityType, ref.ID)
		}
		category, err := item.NewCategory(source.Item.Category)
		if err != nil {
			return match.CardSpec{}, fmt.Errorf("card %s/%d: %w", ref.EntityType, ref.ID, err)
		}
		spec.Item = &match.ItemSpec{Category: category, AttackBonus: item.NewAttackBonus(source.Item.AttackBonus), HitPointsBonus: item.NewHitPointsBonus(source.Item.HitPointsBonus)}
	}
	return spec, nil
}

func newCreatureSpec(source *master.CreatureSpec) (match.CreatureSpec, error) {
	elements, err := creature.NewElements(source.Elements)
	if err != nil {
		return match.CreatureSpec{}, err
	}
	attack, err := creature.NewAttack(source.Attack)
	if err != nil {
		return match.CreatureSpec{}, err
	}
	hitPoints, err := creature.NewHitPoints(source.HitPoints)
	if err != nil {
		return match.CreatureSpec{}, err
	}
	movement, err := creature.NewMovement(source.Movement)
	if err != nil {
		return match.CreatureSpec{}, err
	}
	return match.CreatureSpec{Elements: elements, Attack: attack, HitPoints: hitPoints, Movement: movement}, nil
}

// 変える操作の結果は、公開 ID と新しい revision だけにする（部屋の ChangedRoom と同じ。内容は返さず、画面は対戦を読み直す）
type ChangedMatch struct {
	ID       uuid.UUID
	Revision int64
}

// 対戦の番の操作。context のプレイヤーが、今の番の参加者として 1 つ操作する。NPC の番は MatchNPCTurnHandler が進める
type MatchHandler struct {
	eventStore    domain.EventStore[match.ID, match.Event, match.State]
	matches       domain.Identifier[match.ID]
	getCandidates master.GetPublishedCardsFunc
	newRandom     func() match.Random
	now           func() time.Time
}

func NewMatchHandler(
	eventStore domain.EventStore[match.ID, match.Event, match.State],
	matches domain.Identifier[match.ID],
	getCandidates master.GetPublishedCardsFunc,
	newRandom func() match.Random,
	now func() time.Time,
) *MatchHandler {
	return &MatchHandler{eventStore: eventStore, matches: matches, getCandidates: getCandidates, newRandom: newRandom, now: now}
}

// カードを引く。以下の操作はどれも、番でなければ ErrMatchNotYourTurn、段階が合わなければ ErrMatchPhaseNotAllowed、
// 無い・参加していない対戦は ErrMatchNotFound、revision が古ければ ErrRevisionConflict、未登録のプレイヤーは ErrPlayerNotRegistered
func (c *MatchHandler) Draw(ctx context.Context, id uuid.UUID, revision int64) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		return &match.DrawCommand{By: by}, nil
	})
}

// 手札を 1 枚捨てる。手札の番号が範囲の外なら input.Errors（OutOfRange・項目名 handIndex。番の判定と段階の判定が先）
func (c *MatchHandler) Discard(ctx context.Context, id uuid.UUID, revision int64, handIndex int) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		return &match.DiscardCommand{By: by, HandIndex: handIndex}, nil
	})
}

func (c *MatchHandler) Roll(ctx context.Context, id uuid.UUID, revision int64) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		return &match.RollCommand{By: by}, nil
	})
}

// 分かれ道で行き先を選ぶ。選べる行き先の中に無ければ input.Errors（NotFound・項目名 to。位置が負の値でも同じ）
func (c *MatchHandler) ChoosePath(ctx context.Context, id uuid.UUID, revision int64, x, y int) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		// 負の位置は盤面に無く、選べる行き先の中にも無い。Decide まで通さず、選べない行き先と同じ NotFound にそろえる
		to, err := newPosition(x, y)
		if err != nil {
			return nil, input.Errors{&input.NotFound{Field: "to"}}
		}
		return &match.ChoosePathCommand{By: by, To: to}, nil
	})
}

// 止まった空き地に、手札のクリーチャーを配置する（配置したらターンを終える）。配置できない手札は input.Errors（NotFound・項目名 handIndex）、
// 手札の範囲外は input.Errors（OutOfRange）
func (c *MatchHandler) Deploy(ctx context.Context, id uuid.UUID, revision int64, handIndex int) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		return &match.DeployCreatureCommand{By: by, HandIndex: handIndex}, nil
	})
}

// 止まった他人の領地を、手札のクリーチャーで侵略する（選んだ時点で確定する）。侵略できない手札は input.Errors（NotFound・項目名 handIndex）、
// 手札の範囲外は input.Errors（OutOfRange）
func (c *MatchHandler) Invade(ctx context.Context, id uuid.UUID, revision int64, handIndex int) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		return &match.InvadeCommand{By: by, HandIndex: handIndex}, nil
	})
}

// 侵略を確定した人が、アイテムを選ぶ（出さなければ nil）。出せないアイテムは input.Errors（NotFound・項目名 itemHandIndex）
func (c *MatchHandler) Attack(ctx context.Context, id uuid.UUID, revision int64, itemHandIndex *int) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		return &match.AttackCommand{By: by, ItemHandIndex: itemHandIndex}, nil
	})
}

// 止まった他人の領地の通行料を払う
func (c *MatchHandler) PayToll(ctx context.Context, id uuid.UUID, revision int64) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		return &match.PayTollCommand{By: by}, nil
	})
}

// 侵略された領地の持ち主が、アイテムを選んで戦闘に進める（出さなければ nil）。出せないアイテムは input.Errors（NotFound・項目名 itemHandIndex）
func (c *MatchHandler) Defend(ctx context.Context, id uuid.UUID, revision int64, itemHandIndex *int) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		return &match.DefendCommand{By: by, ItemHandIndex: itemHandIndex}, nil
	})
}

// 魔力が負の間に、自分の領地を 1 つ手放す。手放せない位置（自分の領地でない）は input.Errors（NotFound・項目名 position。位置が負の値でも同じ）
func (c *MatchHandler) RelinquishTerritory(ctx context.Context, id uuid.UUID, revision int64, x, y int) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		position, err := newPosition(x, y)
		if err != nil {
			return nil, input.Errors{&input.NotFound{Field: "position"}}
		}
		return &match.RelinquishTerritoryCommand{By: by, Position: position}, nil
	})
}

// 通った自分の領地のレベルを上げる。選べない位置とレベルの組は input.Errors（NotFound・項目名 position。位置が負の値でも同じ）
func (c *MatchHandler) LevelUpTerritory(ctx context.Context, id uuid.UUID, revision int64, x, y, level int) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		position, err := newPosition(x, y)
		if err != nil {
			return nil, input.Errors{&input.NotFound{Field: "position"}}
		}
		return &match.LevelUpTerritoryCommand{By: by, Position: position, Level: level}, nil
	})
}

// 通った自分の領地のクリーチャーを手札のクリーチャーと交換する。選べない組は input.Errors（NotFound・項目名 position。位置が負の値でも同じ）
func (c *MatchHandler) ExchangeCreature(ctx context.Context, id uuid.UUID, revision int64, x, y, handIndex int) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		position, err := newPosition(x, y)
		if err != nil {
			return nil, input.Errors{&input.NotFound{Field: "position"}}
		}
		return &match.ExchangeCreatureCommand{By: by, Position: position, HandIndex: handIndex}, nil
	})
}

// 通った自分の領地の土地の属性を変える。element は board の属性の名前（fire 等）。選べない位置・属性の組は input.Errors（NotFound・項目名 position、属性の名前が無いものは項目名 element）
func (c *MatchHandler) ChangeLandElement(ctx context.Context, id uuid.UUID, revision int64, x, y int, element string) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		position, err := newPosition(x, y)
		if err != nil {
			return nil, input.Errors{&input.NotFound{Field: "position"}}
		}
		parsed, err := board.NewElement(element)
		if err != nil {
			return nil, input.Errors{&input.NotFound{Field: "element"}}
		}
		return &match.ChangeLandElementCommand{By: by, Position: position, Element: parsed}, nil
	})
}

// 通った自分の領地のクリーチャーを動かす（他人の領地へなら続けて戦闘）。選べない組は input.Errors（NotFound・項目名 to。位置が負の値でも同じ）
func (c *MatchHandler) MoveCreature(ctx context.Context, id uuid.UUID, revision int64, fromX, fromY, toX, toY int) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		from, err := newPosition(fromX, fromY)
		if err != nil {
			return nil, input.Errors{&input.NotFound{Field: "to"}}
		}
		to, err := newPosition(toX, toY)
		if err != nil {
			return nil, input.Errors{&input.NotFound{Field: "to"}}
		}
		return &match.MoveCreatureCommand{By: by, From: from, To: to}, nil
	})
}

func (c *MatchHandler) EndTurn(ctx context.Context, id uuid.UUID, revision int64) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(_ context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		return &match.EndTurnCommand{By: by}, nil
	})
}

// 終わった後に、戦利品のパックを選ぶ（番は無く、受け取っていないプレイヤーが 1 回だけ。NPC は使えない）。
// 終わっていなければ ErrMatchPhaseNotAllowed、受け取り済みなら ErrMatchRewardAlreadyReceived、選べないパックは input.Errors（NotFound・項目名 pack）。
// 引くカードの候補は、そのときの公開中のカード
func (c *MatchHandler) ChooseRewardPack(ctx context.Context, id uuid.UUID, revision int64, pack string) (ChangedMatch, error) {
	return c.change(ctx, id, revision, func(ctx context.Context, _ match.Match, by match.ParticipantIndex) (match.Command, error) {
		key, err := match.NewPackKey(pack)
		if err != nil {
			return nil, input.Errors{&input.NotFound{Field: "pack"}}
		}
		candidates, err := c.getCandidates(ctx)
		if err != nil {
			return nil, err
		}
		return &match.ChooseRewardPackCommand{By: by, Pack: key, Candidates: candidates}, nil
	})
}

// 対戦を読み、参加しているプレイヤーであることと revision を確かめてから、build が作ったコマンドを動かして保存する
func (c *MatchHandler) change(ctx context.Context, publicID uuid.UUID, revision int64, build func(ctx context.Context, current match.Match, by match.ParticipantIndex) (match.Command, error)) (ChangedMatch, error) {
	actor, ok := authentication.Player(ctx)
	if !ok {
		return ChangedMatch{}, ErrPlayerNotRegistered
	}
	random := c.newRandom()
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedMatch, error) {
		id, err := c.matches.Resolve(ctx, publicID)
		if errors.Is(err, domain.ErrEntityNotFound) {
			return ChangedMatch{}, ErrMatchNotFound
		}
		if err != nil {
			return ChangedMatch{}, err
		}
		state, err := c.eventStore.Load(ctx, id)
		if err != nil {
			return ChangedMatch{}, err
		}
		current, ok := state.(match.Match)
		if !ok {
			return ChangedMatch{}, ErrMatchNotFound
		}
		by, joined := participantOf(current, actor)
		if !joined {
			return ChangedMatch{}, ErrMatchNotFound
		}
		// revision を Load の直後に比べる理由は RoomHandler.change と同じ
		if current.Version != revision {
			return ChangedMatch{}, ErrRevisionConflict
		}
		cmd, err := build(ctx, current, by)
		if err != nil {
			return ChangedMatch{}, err
		}
		events, err := match.Decide(current, cmd, c.now(), random)
		if err != nil {
			return ChangedMatch{}, mapMatchError(err, current, by)
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return ChangedMatch{}, err
		}
		return ChangedMatch{ID: publicID, Revision: events[len(events)-1].GetMeta().AggregateVersion}, nil
	})
	return changed, revisionConflict(err)
}

func newPosition(x, y int) (board.Position, error) {
	column, err := board.NewX(x)
	if err != nil {
		return board.Position{}, err
	}
	row, err := board.NewY(y)
	if err != nil {
		return board.Position{}, err
	}
	return board.Position{X: column, Y: row}, nil
}

func participantOf(current match.Match, playerID player.ID) (match.ParticipantIndex, bool) {
	for i, participant := range current.Participants {
		if p := participant.Participant.Player; p != nil && p.Player == playerID {
			index, err := match.NewParticipantIndex(i)
			return index, err == nil
		}
	}
	return 0, false
}

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変える。
// 手札の番号の上限は、操作した人の手札の枚数から言う
func mapMatchError(err error, current match.Match, by match.ParticipantIndex) error {
	switch {
	case errors.Is(err, match.ErrNotYourTurn):
		return ErrMatchNotYourTurn
	case errors.Is(err, match.ErrPhaseNotAllowed):
		return ErrMatchPhaseNotAllowed
	case errors.Is(err, match.ErrHandIndexOutOfRange):
		return input.Errors{&input.OutOfRange{Field: "handIndex", Min: 0, Max: len(current.Participants[by].Hand) - 1}}
	case errors.Is(err, match.ErrCreatureNotDeployable), errors.Is(err, match.ErrCreatureNotInvadable):
		return input.Errors{&input.NotFound{Field: "handIndex"}}
	case errors.Is(err, match.ErrItemNotUsable):
		return input.Errors{&input.NotFound{Field: "itemHandIndex"}}
	case errors.Is(err, match.ErrTerritoryNotRelinquishable), errors.Is(err, match.ErrTerritoryNotLevelUpable), errors.Is(err, match.ErrCreatureNotExchangeable), errors.Is(err, match.ErrLandElementNotChangeable):
		return input.Errors{&input.NotFound{Field: "position"}}
	case errors.Is(err, match.ErrCreatureNotMovable):
		return input.Errors{&input.NotFound{Field: "to"}}
	case errors.Is(err, match.ErrPathNotAllowed):
		return input.Errors{&input.NotFound{Field: "to"}}
	case errors.Is(err, match.ErrRewardAlreadyReceived):
		return ErrMatchRewardAlreadyReceived
	case errors.Is(err, match.ErrRewardPackNotAvailable):
		return input.Errors{&input.NotFound{Field: "pack"}}
	}
	return err
}

// 対戦（match）のイベントを受けて、今の番が NPC なら 1 つ操作するポリシー。操作で出たイベントがまたこのポリシーを動かすので、
// NPC の番が続いても最後まで進む。待たない（見せる速さは画面が決める）
type MatchNPCTurnHandler struct {
	eventStore domain.EventStore[match.ID, match.Event, match.State]
	newRandom  func() match.Random
	now        func() time.Time
}

func NewMatchNPCTurnHandler(
	eventStore domain.EventStore[match.ID, match.Event, match.State],
	newRandom func() match.Random,
	now func() time.Time,
) *MatchNPCTurnHandler {
	return &MatchNPCTurnHandler{eventStore: eventStore, newRandom: newRandom, now: now}
}

func (c *MatchNPCTurnHandler) Handle(ctx context.Context, event any) error {
	if e, ok := event.(match.Event); ok {
		meta := e.GetMeta()
		return c.Play(ctx, meta.EntityID, meta.AggregateVersion)
	}
	return nil
}

// 受けたイベントの版が今の版と違えば何もしない（1 つの操作で出た複数のイベントのうち、最後のイベントでだけ動くため。
// 前のイベントの分は、後のイベントで動く）。終わった対戦・プレイヤーの操作を待っている間も何もしない。
// 保存の版の衝突は、ほかが先に進めたものとして成功で返す（ポリシーは二度動くことがある）
func (c *MatchNPCTurnHandler) Play(ctx context.Context, matchID match.ID, eventVersion int64) error {
	random := c.newRandom()
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		state, err := c.eventStore.Load(ctx, matchID)
		if err != nil {
			return struct{}{}, err
		}
		current, ok := state.(match.Match)
		if !ok {
			return struct{}{}, fmt.Errorf("match npc turn: match %d is not created: %w", matchID, match.ErrInvalidState)
		}
		if current.Version != eventVersion || current.Phase == match.PhaseEnded {
			return struct{}{}, nil
		}
		by, ok := current.AwaitingParticipant()
		if !ok || current.Participants[by].Participant.NPC == nil {
			return struct{}{}, nil
		}
		cmd, err := npcCommand(current, by, random)
		if err != nil {
			return struct{}{}, err
		}
		events, err := match.Decide(current, cmd, c.now(), random)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.eventStore.Save(ctx, events)
	})
	if errors.Is(err, domain.ErrVersionConflict) {
		return nil
	}
	return err
}

// 段階に合う操作を 1 つ選ぶ。手札と行き先は無作為に選ぶ
func npcCommand(current match.Match, by match.ParticipantIndex, random match.Random) (match.Command, error) {
	switch current.Phase {
	case match.PhaseDrawing:
		return &match.DrawCommand{By: by}, nil
	case match.PhaseDiscarding:
		return &match.DiscardCommand{By: by, HandIndex: random.IntN(len(current.Participants[by].Hand))}, nil
	case match.PhaseRolling:
		return &match.RollCommand{By: by}, nil
	case match.PhaseChoosingPath:
		choices := current.PathChoices()
		if len(choices) == 0 {
			return nil, fmt.Errorf("match npc turn: no path choices: %w", match.ErrInvalidState)
		}
		return &match.ChoosePathCommand{By: by, To: choices[random.IntN(len(choices))]}, nil
	case match.PhaseDeploying:
		// 払える最初のクリーチャーを配置し、無ければ配置せずにターンを終える（2026-11-08 オーナー。考え方の部品にするのは core#117 以降）
		if choices := current.DeployChoices(); len(choices) > 0 {
			return &match.DeployCreatureCommand{By: by, HandIndex: choices[0]}, nil
		}
		return &match.EndTurnCommand{By: by}, nil
	case match.PhaseTerritoryCommanding:
		// NPC は領地コマンドを使わない（考え方の部品にするのは core#117）
		return &match.EndTurnCommand{By: by}, nil
	case match.PhaseInvading:
		// 出せる最初のクリーチャーで侵略し、無ければ通行料を払う。アイテムは使わない（2026-11-09 オーナー。考え方の部品にするのは core#117 以降）
		if choices := current.InvadeChoices(); len(choices) > 0 {
			return &match.InvadeCommand{By: by, HandIndex: choices[0]}, nil
		}
		return &match.PayTollCommand{By: by}, nil
	case match.PhaseAttacking:
		return &match.AttackCommand{By: by}, nil
	case match.PhaseDefending:
		return &match.DefendCommand{By: by}, nil
	case match.PhaseRelinquishing:
		position, ok := current.LowestValueRelinquishChoice()
		if !ok {
			return nil, fmt.Errorf("match npc turn: no territories to relinquish: %w", match.ErrInvalidState)
		}
		return &match.RelinquishTerritoryCommand{By: by, Position: position}, nil
	case match.PhaseEnding:
		return &match.EndTurnCommand{By: by}, nil
	default:
		return nil, fmt.Errorf("match npc turn: phase %q: %w", current.Phase, match.ErrInvalidState)
	}
}
