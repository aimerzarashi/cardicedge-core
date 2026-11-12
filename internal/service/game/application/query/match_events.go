package query

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
)

// 対戦のイベントを版の順にすべて返す（イベントストアから読む）
type LoadMatchEventsFunc func(ctx context.Context, id match.ID) ([]match.Event, error)

// 差分の query が返すイベント。種類ごとの型で、見せないもの（山札の並び）は項目を持たない。
// 誰が（Participant）は Match.Participants の何番目か
type MatchEvent interface {
	Meta() MatchEventMeta
}

type MatchEventMeta struct {
	// 対戦の集約の版（抜けなく 1 から続く）
	Revision   int64
	OccurredAt time.Time
	Changes    MatchEventChanges
}

// このイベントで変わった、描画に要る値のうち、イベントの事実からは出せず core の決まりで決まるもの。
// 変わった分だけにするのは、イベントごとに全部を返すと盤面の広さの分だけ応答が膨らむため
type MatchEventChanges struct {
	// このイベントの後の段階。操作を待っていない間（始まる前・移動の途中）は nil
	Phase *MatchPhase
	// 総魔力が変わった参加者だけ
	TotalMagics []MatchTotalMagicChange
	// 価値か通行料が変わった領地だけ（できた順）
	TerritoryValues []MatchTerritoryValue
}

type MatchTotalMagicChange struct {
	Participant int
	TotalMagic  int
}

type MatchTerritoryValue struct {
	Position BoardPosition
	Value    int
	Toll     int
}

func (m MatchEventMeta) Meta() MatchEventMeta { return m }

// 始まった。参加者・盤面・ルールは対戦の状態の query で読む
type MatchCreatedEvent struct{ MatchEventMeta }

type MatchTurnOrderDecidedEvent struct {
	MatchEventMeta
	// 参加者の番号の並び。早い順
	Order []int
}

// 山札を混ぜた。並びは見せないので、何も持たない
type MatchDecksShuffledEvent struct{ MatchEventMeta }

type MatchHandsDealtEvent struct {
	MatchEventMeta
	// 全員に配った枚数（山札が足りなければ、ある分だけ）
	Cards int
}

type MatchCardDrawnEvent struct {
	MatchEventMeta
	Participant int
	Card        MatchCard
}

// 山札が空のまま引くときに、捨て札を混ぜて山札にした。並びは見せないので、枚数だけを持つ
type MatchDiscardPileShuffledEvent struct {
	MatchEventMeta
	Participant int
	// 山札にした枚数（捨て札はこれで空になる）
	DrawPileCount int
}

// ターンを終えるときに、手札が上限を超えていた。上限まで捨ててからターンを終える
type MatchHandLimitExceededEvent struct {
	MatchEventMeta
	Participant int
}

type MatchCardDiscardedEvent struct {
	MatchEventMeta
	Participant int
	// 手札の何番目か（0 始まり）
	HandIndex int
	Card      MatchCard
}

type MatchDiceRolledEvent struct {
	MatchEventMeta
	Participant int
	// 出た目の合計（進む歩数）
	Value int
	// 複数のサイコロを同時に振ったときの 1 個ずつの目（1 個のときは空）
	Dice []int
	// 呪いで目が決まった
	FixedByCurse bool
}

// スペルで、対象のプレイヤーに呪いをかけた
type MatchCurseCastEvent struct {
	MatchEventMeta
	Participant int
	Target      int
	Curse       MatchCurse
}

// スペルで、領地のクリーチャーにダメージを与えた（RemainingHitPoints が 0 なら続けて破壊される）
type MatchCreatureDamagedEvent struct {
	MatchEventMeta
	Participant        int
	Position           BoardPosition
	Creature           int
	Damage             int
	RemainingHitPoints int
}

// スペルのダメージで、領地のクリーチャーが破壊された
type MatchCreatureDestroyedEvent struct {
	MatchEventMeta
	Participant int
	Position    BoardPosition
	Creature    int
}

// 自分のターンを終えて、かかっていた呪いが解けた
type MatchCurseRemovedEvent struct {
	MatchEventMeta
	Participant int
}

// スペルの効果で魔力を得た
type MatchMagicGainedEvent struct {
	MatchEventMeta
	Participant int
	Magic       int
}

// スペルの効果で、このターンに振るサイコロの個数が変わった
type MatchDiceCountChangedEvent struct {
	MatchEventMeta
	Participant int
	Count       int
}

type MatchMovedEvent struct {
	MatchEventMeta
	Participant int
	To          BoardPosition
}

type MatchCheckpointPassedEvent struct {
	MatchEventMeta
	Participant int
	Checkpoint  BoardPosition
	// 足した魔力
	Magic int
}

type MatchLapCompletedEvent struct {
	MatchEventMeta
	Participant int
	// 足した魔力
	Magic int
}

type MatchTurnEndedEvent struct {
	MatchEventMeta
	Participant int
}

type MatchEndedEvent struct {
	MatchEventMeta
	Winners []int
	Ranks   []int
	Reason  MatchEndReason
}

// 通行料を払う直前に、領地のクリーチャーの通行料の倍率が発動した。Toll は盤面の通行料、MultipliedToll は払う額
type MatchTollMultiplierActivatedEvent struct {
	MatchEventMeta
	Position       BoardPosition
	Percent        int
	Toll           int
	MultipliedToll int
}

// 他人の領地に止まって通行料を払った（足りなくても全額。魔力は負になる）。Owner は受け取った参加者の番号
type MatchTollPaidEvent struct {
	MatchEventMeta
	Participant int
	Owner       int
	Position    BoardPosition
	Magic       int
}

// 魔力が負の間に自分の領地を手放した。Magic は受け取った魔力
type MatchTerritoryRelinquishedEvent struct {
	MatchEventMeta
	Participant int
	Position    BoardPosition
	Creature    int
	Magic       int
}

// 領地が無くなっても魔力が負のままで、拠点からやり直す。Magic は与えられた開始時の魔力
type MatchMagicDepletedEvent struct {
	MatchEventMeta
	Participant int
	Magic       int
}

// 止まった空き地に、手札のクリーチャーを配置した（そのままターンを終える）。Creature は配置したクリーチャーの ID（Match.territories と同じ番号）
type MatchCreaturePlacedEvent struct {
	MatchEventMeta
	Participant int
	// 手札の何番目か（0 始まり）
	HandIndex int
	Card      MatchCard
	Creature  int
	Position  BoardPosition
	// 払った魔力
	Cost int
}

// 止まった他人の領地を、手札のクリーチャーで侵略した。Cost は払った魔力
type MatchTerritoryInvadedEvent struct {
	MatchEventMeta
	Participant int
	// 手札の何番目か（0 始まり）
	HandIndex int
	Card      MatchCard
	Position  BoardPosition
	Cost      int
}

// サイコロを振る前に、手札のスペルを使った。Cost は払った魔力。効果は続くイベント（引いたカード等）
type MatchSpellUsedEvent struct {
	MatchEventMeta
	Participant int
	// 手札の何番目か（0 始まり）
	HandIndex int
	Card      MatchCard
	Cost      int
}

// 戦闘に手札のアイテムを出した（攻める側も守る側も同じ）。Cost は払った魔力
type MatchItemUsedEvent struct {
	MatchEventMeta
	Participant int
	// 手札の何番目か（0 始まり）
	HandIndex int
	Card      MatchCard
	Cost      int
}

// 戦闘にアイテムを出さなかった（攻める側も守る側も同じ）
type MatchItemSkippedEvent struct {
	MatchEventMeta
	Participant int
}

// 戦闘のどちら側か
type MatchBattleSide string

const (
	MatchBattleSideAttacker MatchBattleSide = "attacker"
	MatchBattleSideDefender MatchBattleSide = "defender"
)

// 戦闘の中の AT・HP
type MatchBattleStats struct {
	Attack    int
	HitPoints int
}

// 戦闘が始まった。効果を当てる前の両方の AT・HP
type MatchBattleStartedEvent struct {
	MatchEventMeta
	Attacker MatchBattleStats
	Defender MatchBattleStats
}

// 戦闘の AT・HP を変えた効果の出どころ。どれか 1 つだけ持つ
type MatchBattleStatSource struct {
	AdjacentTerritories []BoardPosition
	Terrain             *MatchTerrainSource
	Item                *MatchCard
}

// 地形効果が効いた土地の属性とレベル
type MatchTerrainSource struct {
	Element BoardTileElement
	Level   int
}

// 効果 1 つが戦闘の AT・HP を変えた。Attack・HitPoints は当てた後の値
type MatchBattleStatModifiedEvent struct {
	MatchEventMeta
	Side           MatchBattleSide
	Source         MatchBattleStatSource
	AttackDelta    int
	HitPointsDelta int
	Attack         int
	HitPoints      int
}

// 戦闘で能力が発動した（効き目が無くても、持っていれば出る）
type MatchBattleAbilityActivatedEvent struct {
	MatchEventMeta
	Side    MatchBattleSide
	Ability MatchBattleAbility
}

// 戦闘で発動する能力の種類
type MatchBattleAbility string

const (
	MatchBattleAbilityFirstStrike MatchBattleAbility = "first_strike"
	MatchBattleAbilityLastStrike  MatchBattleAbility = "last_strike"
)

// 戦闘の攻撃が 1 回当たった。RemainingHitPoints は受けた側の、効果で増えた分も含む残りの体力で、0 なら倒れた
type MatchCreatureStruckEvent struct {
	MatchEventMeta
	Striker            MatchBattleSide
	Damage             int
	RemainingHitPoints int
}

// 守る側のクリーチャーを倒して土地を奪った。Creature は領地に入ったクリーチャーの新しい ID
type MatchTerritoryConqueredEvent struct {
	MatchEventMeta
	Participant        int
	Position           BoardPosition
	DefeatedCreature   int
	Creature           int
	TerritoryHitPoints int
}

// 守る側のクリーチャーを倒せなかった。攻めたクリーチャーは生きていれば手札に戻り、倒れていれば捨て札になる
type MatchInvasionRepelledEvent struct {
	MatchEventMeta
	Participant        int
	Position           BoardPosition
	Card               MatchCard
	ReturnedToHand     bool
	TerritoryHitPoints int
	// 移動で侵略して生き残ったときだけ。戻った元の領地の位置と体力
	ReturnedTo        *BoardPosition
	ReturnedHitPoints *int
}

// 通った自分の領地のレベルを上げた
type MatchTerritoryLeveledUpEvent struct {
	MatchEventMeta
	Participant int
	Position    BoardPosition
	Level       int
	Magic       int
}

// 通った自分の領地のクリーチャーを空き地へ動かした
type MatchCreatureMovedEvent struct {
	MatchEventMeta
	Participant int
	From        BoardPosition
	To          BoardPosition
	Creature    int
}

// 通った自分の領地のクリーチャーを他人の領地へ動かし、侵略した
type MatchTerritoryInvadedByMoveEvent struct {
	MatchEventMeta
	Participant int
	From        BoardPosition
	Position    BoardPosition
	Creature    int
}

// 通った自分の領地の土地の属性を変えた
type MatchLandElementChangedEvent struct {
	MatchEventMeta
	Participant int
	Position    BoardPosition
	Element     BoardTileElement
	Magic       int
}

// 領地のクリーチャーを手札のクリーチャーと入れ替えた
type MatchCreatureExchangedEvent struct {
	MatchEventMeta
	Participant  int
	Position     BoardPosition
	HandIndex    int
	Card         MatchCard
	Creature     int
	Cost         int
	ReturnedCard MatchCard
}

// 戦利品のパックから引いた
type MatchRewardDrawnEvent struct {
	MatchEventMeta
	Participant int
	Pack        string
	Cards       []MatchCard
}

// プレイヤー全員が戦利品を受け取った（対戦の流れの終わり）
type MatchRewardsCompletedEvent struct{ MatchEventMeta }

// 「この版より後のイベント」を返す差分の query。対戦は read model を作らず、イベントストアから読んで写す。
// 参加者のプレイヤーだけが読める
type MatchEventsHandler struct {
	authorize  AuthorizeMatchFunc
	loadEvents LoadMatchEventsFunc
	entityRefs domain.EntityRefResolver
	loadNames  LoadMatchNamesFunc
}

func NewMatchEventsHandler(authorize AuthorizeMatchFunc, loadEvents LoadMatchEventsFunc, entityRefs domain.EntityRefResolver, loadNames LoadMatchNamesFunc) *MatchEventsHandler {
	return &MatchEventsHandler{authorize: authorize, loadEvents: loadEvents, entityRefs: entityRefs, loadNames: loadNames}
}

// after の版より後を、古い順にすべて返す（対戦の長さはラウンドの上限で決まるので、ページ分けしない）。
// 参加していない・無い対戦は ErrMatchNotFound
func (q *MatchEventsHandler) ListAfter(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID, after int64) ([]MatchEvent, error) {
	id, err := q.authorize(ctx, playerID, matchID)
	if err != nil {
		return nil, err
	}
	loaded, err := q.loadEvents(ctx, id)
	if err != nil {
		return nil, err
	}
	var events []match.Event
	for _, event := range loaded {
		if event.GetMeta().AggregateVersion > after {
			events = append(events, event)
		}
	}
	cards, err := q.cardsOf(ctx, events, snapshotSpecs(loaded))
	if err != nil {
		return nil, err
	}
	changes, err := changesByRevision(loaded, after)
	if err != nil {
		return nil, err
	}
	result := make([]MatchEvent, 0, len(events))
	for _, event := range events {
		converted, err := fromMatchEvent(event, cards, changes[event.GetMeta().AggregateVersion])
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

// イベントを始めから順に当てて、after より後の版ごとに変わった値を作る。after より前も当てるのは、途中からでは状態が作れないため。
// payload に持たせないのは、計算で出せる値を保存しないため
func changesByRevision(loaded []match.Event, after int64) (map[int64]MatchEventChanges, error) {
	changes := make(map[int64]MatchEventChanges, len(loaded))
	var state match.State = match.InitialState()
	var previous match.Match
	for _, event := range loaded {
		next, err := match.Evolve(state, event)
		if err != nil {
			return nil, fmt.Errorf("query: evolve match event %d: %w", event.GetMeta().AggregateVersion, err)
		}
		current, ok := next.(match.Match)
		if !ok {
			return nil, fmt.Errorf("query: unexpected match state: %T", next)
		}
		if current.Version > after {
			changed, err := changesBetween(previous, current)
			if err != nil {
				return nil, err
			}
			changes[current.Version] = changed
		}
		state, previous = next, current
	}
	return changes, nil
}

func changesBetween(previous, current match.Match) (MatchEventChanges, error) {
	changes := MatchEventChanges{TotalMagics: []MatchTotalMagicChange{}, TerritoryValues: []MatchTerritoryValue{}}
	if current.Phase.AwaitsOperation() {
		phase, err := fromMatchPhase(current.Phase)
		if err != nil {
			return MatchEventChanges{}, err
		}
		changes.Phase = &phase
	}
	before := previous.TotalMagics()
	for i, total := range current.TotalMagics() {
		if i >= len(before) || before[i] != total {
			changes.TotalMagics = append(changes.TotalMagics, MatchTotalMagicChange{Participant: i, TotalMagic: total})
		}
	}
	for _, territory := range current.Territories {
		value, toll := current.TerritoryValue(territory), current.TerritoryToll(territory)
		if old, ok := previous.TerritoryAt(territory.Position); ok && previous.TerritoryValue(old) == value && previous.TerritoryToll(old) == toll {
			continue
		}
		changes.TerritoryValues = append(changes.TerritoryValues, MatchTerritoryValue{
			Position: NewBoardPosition(territory.Position.X.Int(), territory.Position.Y.Int()), Value: value, Toll: toll,
		})
	}
	return changes, nil
}

// 対戦を作ったときに写した性能（最初のイベント。after によらず、読んだイベントのすべてから探す）
func snapshotSpecs(loaded []match.Event) map[card.Ref]match.CardSpec {
	specs := map[card.Ref]match.CardSpec{}
	for _, event := range loaded {
		if created, ok := event.(*match.CreatedEvent); ok {
			for _, participant := range created.Participants {
				for _, entry := range participant.Cards {
					specs[entry.Card] = entry.Spec
				}
			}
		}
	}
	return specs
}

// イベントが述べたカードの名前と種類を、1 回でまとめて引く。対戦の中のカードの性能は写し、戦利品は写した cards の性能
func (q *MatchEventsHandler) cardsOf(ctx context.Context, events []match.Event, specs map[card.Ref]match.CardSpec) (map[domain.EntityRef]MatchCard, error) {
	var refs []domain.EntityRef
	inDeck := map[domain.EntityRef]match.CardSpec{}
	for _, event := range events {
		switch e := event.(type) {
		case *match.CardDrawnEvent:
			ref := domain.NewEntityRef(e.Card.EntityType, e.Card.ID)
			refs = append(refs, ref)
			inDeck[ref] = specs[e.Card]
		case *match.CardDiscardedEvent:
			ref := domain.NewEntityRef(e.Card.EntityType, e.Card.ID)
			refs = append(refs, ref)
			inDeck[ref] = specs[e.Card]
		case *match.CreaturePlacedEvent:
			ref := domain.NewEntityRef(e.Card.EntityType, e.Card.ID)
			refs = append(refs, ref)
			inDeck[ref] = specs[e.Card]
		case *match.CreatureExchangedEvent:
			for _, exchanged := range []card.Ref{e.Card, e.ReturnedCard} {
				ref := domain.NewEntityRef(exchanged.EntityType, exchanged.ID)
				refs = append(refs, ref)
				inDeck[ref] = specs[exchanged]
			}
		case *match.TerritoryInvadedEvent:
			ref := domain.NewEntityRef(e.Card.EntityType, e.Card.ID)
			refs = append(refs, ref)
			inDeck[ref] = specs[e.Card]
		case *match.ItemUsedEvent:
			ref := domain.NewEntityRef(e.Card.EntityType, e.Card.ID)
			refs = append(refs, ref)
			inDeck[ref] = specs[e.Card]
		case *match.SpellUsedEvent:
			ref := domain.NewEntityRef(e.Card.EntityType, e.Card.ID)
			refs = append(refs, ref)
			inDeck[ref] = specs[e.Card]
		case *match.BattleStatModifiedEvent:
			if e.Source.Item != nil {
				ref := domain.NewEntityRef(e.Source.Item.Card.EntityType, e.Source.Item.Card.ID)
				refs = append(refs, ref)
				inDeck[ref] = specs[e.Source.Item.Card]
			}
		case *match.InvasionRepelledEvent:
			ref := domain.NewEntityRef(e.Card.EntityType, e.Card.ID)
			refs = append(refs, ref)
			inDeck[ref] = specs[e.Card]
		case *match.RewardDrawnEvent:
			for _, reward := range e.Cards {
				refs = append(refs, domain.NewEntityRef(reward.EntityType, reward.ID))
			}
		}
	}
	if len(refs) == 0 {
		return nil, nil
	}
	publicIDs, err := q.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(publicIDs))
	for _, publicID := range publicIDs {
		ids = append(ids, publicID)
	}
	names, err := q.loadNames(ctx, nil, nil, ids)
	if err != nil {
		return nil, err
	}
	cards := make(map[domain.EntityRef]MatchCard, len(refs))
	for _, ref := range refs {
		publicID, ok := publicIDs[ref]
		if !ok {
			return nil, fmt.Errorf("query: card %v of match event has no public id", ref)
		}
		found, ok := names.Cards[publicID]
		if !ok {
			return nil, fmt.Errorf("query: card %s of match event is not found", publicID)
		}
		if spec, ok := inDeck[ref]; ok {
			found = withSpec(found, spec)
		}
		cards[ref] = found
	}
	return cards, nil
}

func fromMatchEvent(event match.Event, cards map[domain.EntityRef]MatchCard, changes MatchEventChanges) (MatchEvent, error) {
	meta := MatchEventMeta{Revision: event.GetMeta().AggregateVersion, OccurredAt: event.GetMeta().OccurredAt, Changes: changes}
	switch e := event.(type) {
	case *match.CreatedEvent:
		return MatchCreatedEvent{meta}, nil
	case *match.TurnOrderDecidedEvent:
		return MatchTurnOrderDecidedEvent{meta, participantNumbers(e.Order)}, nil
	case *match.DecksShuffledEvent:
		return MatchDecksShuffledEvent{meta}, nil
	case *match.HandsDealtEvent:
		return MatchHandsDealtEvent{meta, e.Cards}, nil
	case *match.CardDrawnEvent:
		return MatchCardDrawnEvent{meta, e.Participant.Int(), cards[domain.NewEntityRef(e.Card.EntityType, e.Card.ID)]}, nil
	case *match.DiscardPileShuffledEvent:
		return MatchDiscardPileShuffledEvent{meta, e.Participant.Int(), len(e.DrawPile)}, nil
	case *match.HandLimitExceededEvent:
		return MatchHandLimitExceededEvent{meta, e.Participant.Int()}, nil
	case *match.CardDiscardedEvent:
		return MatchCardDiscardedEvent{meta, e.Participant.Int(), e.HandIndex, cards[domain.NewEntityRef(e.Card.EntityType, e.Card.ID)]}, nil
	case *match.DiceRolledEvent:
		dice := e.Dice
		if dice == nil {
			dice = []int{}
		}
		return MatchDiceRolledEvent{meta, e.Participant.Int(), e.Value, dice, e.FixedByCurse}, nil
	case *match.CurseCastEvent:
		curse := e.Curse
		return MatchCurseCastEvent{meta, e.Participant.Int(), e.Target.Int(), *fromCurse(&curse)}, nil
	case *match.CurseRemovedEvent:
		return MatchCurseRemovedEvent{meta, e.Participant.Int()}, nil
	case *match.CreatureDamagedEvent:
		return MatchCreatureDamagedEvent{meta, e.Participant.Int(), NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), e.Creature.Int(), e.Damage, e.RemainingHitPoints.Int()}, nil
	case *match.CreatureDestroyedEvent:
		return MatchCreatureDestroyedEvent{meta, e.Participant.Int(), NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), e.Creature.Int()}, nil
	case *match.MagicGainedEvent:
		return MatchMagicGainedEvent{meta, e.Participant.Int(), e.Magic}, nil
	case *match.DiceCountChangedEvent:
		return MatchDiceCountChangedEvent{meta, e.Participant.Int(), e.Count}, nil
	case *match.MovedEvent:
		return MatchMovedEvent{meta, e.Participant.Int(), NewBoardPosition(e.To.X.Int(), e.To.Y.Int())}, nil
	case *match.CheckpointPassedEvent:
		return MatchCheckpointPassedEvent{meta, e.Participant.Int(), NewBoardPosition(e.Checkpoint.X.Int(), e.Checkpoint.Y.Int()), e.Magic}, nil
	case *match.LapCompletedEvent:
		return MatchLapCompletedEvent{meta, e.Participant.Int(), e.Magic}, nil
	case *match.TurnEndedEvent:
		return MatchTurnEndedEvent{meta, e.Participant.Int()}, nil
	case *match.MatchEndedEvent:
		outcome, err := fromMatchResult(match.Result{Winners: e.Winners, Ranks: e.Ranks, Reason: e.Reason})
		if err != nil {
			return nil, err
		}
		return MatchEndedEvent{meta, outcome.Winners, outcome.Ranks, outcome.Reason}, nil
	case *match.RewardDrawnEvent:
		drawn := make([]MatchCard, len(e.Cards))
		for i, reward := range e.Cards {
			drawn[i] = cards[domain.NewEntityRef(reward.EntityType, reward.ID)]
		}
		return MatchRewardDrawnEvent{meta, e.Participant.Int(), string(e.Pack), drawn}, nil
	case *match.RewardsCompletedEvent:
		return MatchRewardsCompletedEvent{meta}, nil
	case *match.TollMultiplierActivatedEvent:
		return MatchTollMultiplierActivatedEvent{meta, NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), e.Percent.Int(), e.Toll, e.MultipliedToll}, nil
	case *match.TollPaidEvent:
		return MatchTollPaidEvent{meta, e.Participant.Int(), e.Owner.Int(), NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), e.Magic}, nil
	case *match.TerritoryRelinquishedEvent:
		return MatchTerritoryRelinquishedEvent{meta, e.Participant.Int(), NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), e.Creature.Int(), e.Magic}, nil
	case *match.MagicDepletedEvent:
		return MatchMagicDepletedEvent{meta, e.Participant.Int(), e.Magic}, nil
	case *match.CreaturePlacedEvent:
		position := NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int())
		return MatchCreaturePlacedEvent{meta, e.Participant.Int(), e.HandIndex, cards[domain.NewEntityRef(e.Card.EntityType, e.Card.ID)], e.Creature.Int(), position, e.Cost.Int()}, nil
	case *match.TerritoryInvadedEvent:
		position := NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int())
		return MatchTerritoryInvadedEvent{meta, e.Participant.Int(), e.HandIndex, cards[domain.NewEntityRef(e.Card.EntityType, e.Card.ID)], position, e.Cost.Int()}, nil
	case *match.SpellUsedEvent:
		return MatchSpellUsedEvent{meta, e.Participant.Int(), e.HandIndex, cards[domain.NewEntityRef(e.Card.EntityType, e.Card.ID)], e.Cost.Int()}, nil
	case *match.ItemUsedEvent:
		return MatchItemUsedEvent{meta, e.Participant.Int(), e.HandIndex, cards[domain.NewEntityRef(e.Card.EntityType, e.Card.ID)], e.Cost.Int()}, nil
	case *match.ItemSkippedEvent:
		return MatchItemSkippedEvent{meta, e.Participant.Int()}, nil
	case *match.TerritoryLeveledUpEvent:
		return MatchTerritoryLeveledUpEvent{meta, e.Participant.Int(), NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), e.Level, e.Magic}, nil
	case *match.CreatureMovedEvent:
		return MatchCreatureMovedEvent{meta, e.Participant.Int(), NewBoardPosition(e.From.X.Int(), e.From.Y.Int()), NewBoardPosition(e.To.X.Int(), e.To.Y.Int()), e.Creature.Int()}, nil
	case *match.TerritoryInvadedByMoveEvent:
		return MatchTerritoryInvadedByMoveEvent{meta, e.Participant.Int(), NewBoardPosition(e.From.X.Int(), e.From.Y.Int()), NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), e.Creature.Int()}, nil
	case *match.LandElementChangedEvent:
		element, err := NewBoardTileElement(e.Element.String())
		if err != nil {
			return nil, err
		}
		return MatchLandElementChangedEvent{meta, e.Participant.Int(), NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), element, e.Magic}, nil
	case *match.CreatureExchangedEvent:
		position := NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int())
		return MatchCreatureExchangedEvent{meta, e.Participant.Int(), position, e.HandIndex, cards[domain.NewEntityRef(e.Card.EntityType, e.Card.ID)], e.Creature.Int(), e.Cost.Int(), cards[domain.NewEntityRef(e.ReturnedCard.EntityType, e.ReturnedCard.ID)]}, nil
	case *match.BattleStartedEvent:
		return MatchBattleStartedEvent{meta, fromBattleStats(e.Attacker), fromBattleStats(e.Defender)}, nil
	case *match.BattleStatModifiedEvent:
		side, err := fromBattleSide(e.Side)
		if err != nil {
			return nil, err
		}
		source, err := fromBattleStatSource(e.Source, cards)
		if err != nil {
			return nil, err
		}
		return MatchBattleStatModifiedEvent{meta, side, source, e.AttackDelta, e.HitPointsDelta, e.Attack.Int(), e.HitPoints.Int()}, nil
	case *match.BattleAbilityActivatedEvent:
		side, err := fromBattleSide(e.Side)
		if err != nil {
			return nil, err
		}
		ability, err := fromBattleAbility(e.Ability)
		if err != nil {
			return nil, err
		}
		return MatchBattleAbilityActivatedEvent{meta, side, ability}, nil
	case *match.CreatureStruckEvent:
		striker, err := fromBattleSide(e.Striker)
		if err != nil {
			return nil, err
		}
		return MatchCreatureStruckEvent{meta, striker, e.Damage.Int(), e.RemainingHitPoints.Int()}, nil
	case *match.TerritoryConqueredEvent:
		return MatchTerritoryConqueredEvent{meta, e.Participant.Int(), NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), e.DefeatedCreature.Int(), e.Creature.Int(), e.TerritoryHitPoints.Int()}, nil
	case *match.InvasionRepelledEvent:
		position := NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int())
		var returnedTo *BoardPosition
		var returnedHitPoints *int
		if e.ReturnedTo != nil && e.ReturnedHitPoints != nil {
			to, hitPoints := NewBoardPosition(e.ReturnedTo.X.Int(), e.ReturnedTo.Y.Int()), e.ReturnedHitPoints.Int()
			returnedTo, returnedHitPoints = &to, &hitPoints
		}
		return MatchInvasionRepelledEvent{meta, e.Participant.Int(), position, cards[domain.NewEntityRef(e.Card.EntityType, e.Card.ID)], e.ReturnedToHand, e.TerritoryHitPoints.Int(), returnedTo, returnedHitPoints}, nil
	default:
		return nil, fmt.Errorf("query: unexpected match event: %T", event)
	}
}

func fromBattleStats(stats match.BattleStats) MatchBattleStats {
	return MatchBattleStats{Attack: stats.Attack.Int(), HitPoints: stats.HitPoints.Int()}
}

func fromBattleStatSource(source match.BattleStatSource, cards map[domain.EntityRef]MatchCard) (MatchBattleStatSource, error) {
	switch {
	case source.AdjacentTerritories != nil:
		positions := make([]BoardPosition, len(source.AdjacentTerritories.Positions))
		for i, position := range source.AdjacentTerritories.Positions {
			positions[i] = NewBoardPosition(position.X.Int(), position.Y.Int())
		}
		return MatchBattleStatSource{AdjacentTerritories: positions}, nil
	case source.Terrain != nil:
		element, err := NewBoardTileElement(source.Terrain.Element.String())
		if err != nil {
			return MatchBattleStatSource{}, err
		}
		return MatchBattleStatSource{Terrain: &MatchTerrainSource{Element: element, Level: source.Terrain.Level}}, nil
	case source.Item != nil:
		used := cards[domain.NewEntityRef(source.Item.Card.EntityType, source.Item.Card.ID)]
		return MatchBattleStatSource{Item: &used}, nil
	default:
		return MatchBattleStatSource{}, fmt.Errorf("query: battle stat source has no kind")
	}
}

func fromBattleSide(side match.BattleSide) (MatchBattleSide, error) {
	switch side {
	case match.BattleSideAttacker:
		return MatchBattleSideAttacker, nil
	case match.BattleSideDefender:
		return MatchBattleSideDefender, nil
	default:
		return "", fmt.Errorf("query: unknown battle side: %q", side)
	}
}

func fromBattleAbility(ability match.BattleAbility) (MatchBattleAbility, error) {
	switch ability {
	case match.BattleAbilityFirstStrike:
		return MatchBattleAbilityFirstStrike, nil
	case match.BattleAbilityLastStrike:
		return MatchBattleAbilityLastStrike, nil
	default:
		return "", fmt.Errorf("query: unknown battle ability: %q", ability)
	}
}

func participantNumbers(indexes []match.ParticipantIndex) []int {
	numbers := make([]int, len(indexes))
	for i, index := range indexes {
		numbers[i] = index.Int()
	}
	return numbers
}
