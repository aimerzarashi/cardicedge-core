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
	Value       int
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
type MatchCreatureDeployedEvent struct {
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
	result := make([]MatchEvent, 0, len(events))
	for _, event := range events {
		converted, err := fromMatchEvent(event, cards)
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
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

// 引いた・捨てた・配置したカードの名前と種類を、1 回でまとめて引く。引いた・捨てた・配置したカードの性能は写し、戦利品は写した cards の性能
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
		case *match.CreatureDeployedEvent:
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

func fromMatchEvent(event match.Event, cards map[domain.EntityRef]MatchCard) (MatchEvent, error) {
	meta := MatchEventMeta{Revision: event.GetMeta().AggregateVersion, OccurredAt: event.GetMeta().OccurredAt}
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
	case *match.CardDiscardedEvent:
		return MatchCardDiscardedEvent{meta, e.Participant.Int(), e.HandIndex, cards[domain.NewEntityRef(e.Card.EntityType, e.Card.ID)]}, nil
	case *match.DiceRolledEvent:
		return MatchDiceRolledEvent{meta, e.Participant.Int(), e.Value}, nil
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
	case *match.TollPaidEvent:
		return MatchTollPaidEvent{meta, e.Participant.Int(), e.Owner.Int(), NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), e.Magic}, nil
	case *match.TerritoryRelinquishedEvent:
		return MatchTerritoryRelinquishedEvent{meta, e.Participant.Int(), NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int()), e.Creature.Int(), e.Magic}, nil
	case *match.MagicDepletedEvent:
		return MatchMagicDepletedEvent{meta, e.Participant.Int(), e.Magic}, nil
	case *match.CreatureDeployedEvent:
		position := NewBoardPosition(e.Position.X.Int(), e.Position.Y.Int())
		return MatchCreatureDeployedEvent{meta, e.Participant.Int(), e.HandIndex, cards[domain.NewEntityRef(e.Card.EntityType, e.Card.ID)], e.Creature.Int(), position, e.Cost.Int()}, nil
	default:
		return nil, fmt.Errorf("query: unexpected match event: %T", event)
	}
}

func participantNumbers(indexes []match.ParticipantIndex) []int {
	numbers := make([]int, len(indexes))
	for i, index := range indexes {
		numbers[i] = index.Int()
	}
	return numbers
}
