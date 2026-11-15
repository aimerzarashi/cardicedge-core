package query

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
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
	// 価値・通行料・土地のレベル・連鎖の数のどれかが変わった領地だけ（できた順）
	TerritoryValues []MatchTerritoryValue
}

type MatchTotalMagicChange struct {
	Participant int
	TotalMagic  int
}

// 価値の根拠（土地のレベル・連鎖の数）も持つ。連鎖は保存せず数える値（core#128）で、増減のイベントが無いため、
// 再生で連鎖の増減と価値の理由を説明するにはここに持たせる（core#168）
type MatchTerritoryValue struct {
	Position   BoardPosition
	Value      int
	Toll       int
	Level      int
	ChainCount int
	// このイベントの前の連鎖の数。同じ連鎖に入っていなかった（配置した・奪った）なら nil。
	// 画面が前の版をたどって比べると、移動・交換でクリーチャーや位置が変わったときに取り違えるため、ここで決める
	PreviousChainCount *int
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
	Participant  int
	HandCount    int
	MaxHandCards int
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
	// 振った人に続いていたスペルの効果（振り方を決めたもの。無ければ nil）。直前の状態から作る（core#173）
	SpellEffect *SpellEffect
}

// スペルを使い、対象のプレイヤーにターンをまたいで効き続ける効果がかかった
type MatchSpellEffectStartedEvent struct {
	MatchEventMeta
	Participant int
	Target      int
	Effect      MatchLastingSpellEffect
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

// 自分のターンを終えて、続いていたスペルの効果が終わった
type MatchSpellEffectEndedEvent struct {
	MatchEventMeta
	Participant int
}

// スペルの効果で魔力を得た
type MatchMagicGainedEvent struct {
	MatchEventMeta
	Participant int
	Magic       int
	Laps        int
	MagicPerLap int
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
	// 領地にいて、これから戦う守る側のクリーチャーのカード
	DefendingCreatureCard MatchCard
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

// 戦闘の AT・HP の増減（下限で止まった後の実際の量）と当てた後の値。当てた後の値はイベントに無く、直前の状態の戦闘の中の値に
// 増減を当てて作る（画面が計算せずに再生するため）
type MatchBattleStatChange struct {
	AttackDelta    int
	HitPointsDelta int
	Attack         int
	HitPoints      int
}

// 攻める側に支援効果が発動した。根拠（隣の領地の位置と 1 つあたりの AT の増加）は戦闘の入力から添える
type MatchBattleSupportEffectActivatedEvent struct {
	MatchEventMeta
	Side              MatchBattleSide
	Positions         []BoardPosition
	BonusPerTerritory int
	Change            MatchBattleStatChange
}

// 守る側に地形効果が発動した。根拠（土地の属性・レベルと 1 レベルあたりの HP の増加）は戦闘の入力から添える
type MatchBattleTerrainEffectActivatedEvent struct {
	MatchEventMeta
	Side          MatchBattleSide
	Element       BoardTileElement
	Level         int
	BonusPerLevel int
	Change        MatchBattleStatChange
}

// クリーチャーの能力が発動した。AT・HP を変える能力だけ Change を持ち、能力ごとの根拠はその能力のときだけ持つ（ほかは nil）
type MatchBattleCreatureAbilityActivatedEvent struct {
	MatchEventMeta
	Side              MatchBattleSide
	Ability           MatchBattleAbility
	Change            *MatchBattleStatChange
	BattleModifier    *MatchBattleModifierBasis
	BattleRandomStats *MatchBattleRandomStatsBasis
}

// アイテムの効果が発動した。アイテムのカードと、発動したカードの効果（値は下限で止まる前の値）を戦闘の入力から添える。
// カードの効果は同じ種類の最初のもの（倍率は制約を満たす一番高いもの）を採る（1 枚が同じ種類を重ねる見本が無いため。重ねるカードを
// 作るときに見直す）。地形効果を無視するは、相手の HP が地形効果の分を除いた値になる変化を OpponentChange に持つ（core#177）
type MatchBattleItemEffectActivatedEvent struct {
	MatchEventMeta
	Side           MatchBattleSide
	Effect         MatchBattleItemEffect
	Card           MatchCard
	CardEffect     ItemEffect
	Change         *MatchBattleStatChange
	OpponentChange *MatchBattleStatChange
}

// 戦闘で発動するアイテムの効果の種類
type MatchBattleItemEffect string

const (
	MatchBattleItemEffectAttackModifier      MatchBattleItemEffect = "attack_modifier"
	MatchBattleItemEffectHitPointsModifier   MatchBattleItemEffect = "hit_points_modifier"
	MatchBattleItemEffectAttackOverride      MatchBattleItemEffect = "attack_override"
	MatchBattleItemEffectAttackMultiplier    MatchBattleItemEffect = "attack_multiplier"
	MatchBattleItemEffectIgnoreTerrainEffect MatchBattleItemEffect = "ignore_terrain_effect"
)

// 戦闘中の増減の決まった値（下限で止まる前の値）
type MatchBattleModifierBasis struct {
	AttackModifier    int
	HitPointsModifier int
}

// 戦闘中のランダムの範囲（引いた値は Change の当てた後の値）
type MatchBattleRandomStatsBasis struct {
	Minimum int
	Maximum int
}

// 戦闘で発動する能力の種類
type MatchBattleAbility string

const (
	MatchBattleAbilityFirstStrike       MatchBattleAbility = "first_strike"
	MatchBattleAbilityLastStrike        MatchBattleAbility = "last_strike"
	MatchBattleAbilityBattleModifier    MatchBattleAbility = "battle_modifier"
	MatchBattleAbilityBattleRandomStats MatchBattleAbility = "battle_random_stats"
	MatchBattleAbilityAttackImmunity    MatchBattleAbility = "attack_immunity"
)

// 戦闘の攻撃が 1 回当たった。Attack は攻撃した側のそのときの戦闘中の AT、Damage は受けた側が受けた量（攻撃無効なら 0）。
// RemainingHitPoints は受けた側の、効果で増えた分も含む残りの体力で、0 なら倒れた
type MatchCreatureStruckEvent struct {
	MatchEventMeta
	Striker            MatchBattleSide
	Attack             int
	Damage             int
	RemainingHitPoints int
}

// 守る側のクリーチャーを倒して土地を奪った。Creature は領地に入ったクリーチャーの新しい ID
type MatchTerritoryConqueredEvent struct {
	MatchEventMeta
	Participant        int
	Position           BoardPosition
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
	// 動いたクリーチャーとそのカード
	Creature int
	Card     MatchCard
	// 領地にいて、これから戦う守る側のクリーチャーのカード
	DefendingCreatureCard MatchCard
}

// 通った自分の領地の土地の属性を変えた
type MatchLandElementChangedEvent struct {
	MatchEventMeta
	Participant int
	Position    BoardPosition
	Element     BoardTileElement
	Magic       int
	// 払った魔力の根拠（Magic = BaseLandValue × Multiplier）
	BaseLandValue int
	Multiplier    int
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
	changes, previousStates, err := changesByRevision(loaded, after)
	if err != nil {
		return nil, err
	}
	inputs := battleInputsByRevision(loaded)
	cards, err := q.cardsOf(ctx, events, snapshotSpecs(loaded), previousStates, inputs)
	if err != nil {
		return nil, err
	}
	result := make([]MatchEvent, 0, len(events))
	for _, event := range events {
		version := event.GetMeta().AggregateVersion
		converted, err := fromMatchEvent(event, previousStates[version], cards, inputs, changes[version])
		if err != nil {
			return nil, err
		}
		result = append(result, converted)
	}
	return result, nil
}

// イベントを始めから順に当てて、after より後の版ごとに変わった値と、そのイベントを当てる直前の状態を作る。after より前も当てるのは、
// 途中からでは状態が作れないため。payload に持たせないのは、計算で出せる値を保存しないため。直前の状態は、payload が持たない
// 直前の状態から引ける値（手札の番号のカード・位置の領地のクリーチャー等。core#171）を作るのに使う
func changesByRevision(loaded []match.Event, after int64) (map[int64]MatchEventChanges, map[int64]match.Match, error) {
	changes := make(map[int64]MatchEventChanges, len(loaded))
	previousStates := make(map[int64]match.Match, len(loaded))
	var state match.State = match.InitialState()
	var previous match.Match
	for _, event := range loaded {
		next, err := match.Evolve(state, event)
		if err != nil {
			return nil, nil, fmt.Errorf("query: evolve match event %d: %w", event.GetMeta().AggregateVersion, err)
		}
		current, ok := next.(match.Match)
		if !ok {
			return nil, nil, fmt.Errorf("query: unexpected match state: %T", next)
		}
		if current.Version > after {
			changed, err := changesBetween(previous, current)
			if err != nil {
				return nil, nil, err
			}
			changes[current.Version] = changed
			previousStates[current.Version] = previous
		}
		state, previous = next, current
	}
	return changes, previousStates, nil
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
		level, chainCount := current.LandLevel(territory.Position), current.TerritoryChainCount(territory)
		var previousChainCount *int
		if old, ok := sameTerritoryBefore(previous, current, territory); ok {
			count := previous.TerritoryChainCount(old)
			previousChainCount = &count
		}
		// 連鎖の倍率は上限で止まる（既定は 5 連鎖以上で同じ）ので、価値が変わらなくても連鎖の数の増減は届ける（core#168）
		if old, ok := previous.TerritoryAt(territory.Position); ok && previous.TerritoryValue(old) == value && previous.TerritoryToll(old) == toll &&
			previous.LandLevel(territory.Position) == level && previousChainCount != nil && *previousChainCount == chainCount {
			continue
		}
		changes.TerritoryValues = append(changes.TerritoryValues, MatchTerritoryValue{
			Position: NewBoardPosition(territory.Position.X.Int(), territory.Position.Y.Int()), Value: value, Toll: toll,
			Level: level, ChainCount: chainCount, PreviousChainCount: previousChainCount,
		})
	}
	return changes, nil
}

// イベントの前にその領地に当たる領地。同じクリーチャー（移動・属性の変更）を先に探し、無ければ同じ位置で同じ持ち主の領地（交換で
// クリーチャーの ID が変わる）。どちらも無ければ、この持ち主の連鎖に新しく入った（配置した・奪った）
func sameTerritoryBefore(previous, current match.Match, territory match.Territory) (match.Territory, bool) {
	for _, old := range previous.Territories {
		if old.Creature == territory.Creature {
			return old, true
		}
	}
	old, ok := previous.TerritoryAt(territory.Position)
	if !ok {
		return match.Territory{}, false
	}
	oldOwner, oldOK := previous.TerritoryOwner(old)
	owner, currentOK := current.TerritoryOwner(territory)
	if !oldOK || !currentOK || oldOwner != owner {
		return match.Territory{}, false
	}
	return old, true
}

// 戦闘の出来事の版ごとの、その戦闘の入力（直前の BattleStartedEvent）。after によらず読んだイベントのすべてから探す。
// 効果の根拠は入力にだけあり、出来事は種類しか持たないため（core#171）
func battleInputsByRevision(loaded []match.Event) map[int64]match.BattleInput {
	inputs := map[int64]match.BattleInput{}
	var current match.BattleInput
	for _, event := range loaded {
		switch e := event.(type) {
		case *match.BattleStartedEvent:
			current = e.Input
		case *match.BattleSupportEffectActivatedEvent, *match.BattleTerrainEffectActivatedEvent, *match.BattleCreatureAbilityActivatedEvent, *match.BattleItemEffectActivatedEvent:
			inputs[event.GetMeta().AggregateVersion] = current
		}
	}
	return inputs
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
func (q *MatchEventsHandler) cardsOf(ctx context.Context, events []match.Event, specs map[card.Ref]match.CardSpec, previousStates map[int64]match.Match, battleInputs map[int64]match.BattleInput) (map[domain.EntityRef]MatchCard, error) {
	var refs []domain.EntityRef
	inDeck := map[domain.EntityRef]match.CardSpec{}
	for _, event := range events {
		version := event.GetMeta().AggregateVersion
		if e, ok := event.(*match.RewardDrawnEvent); ok {
			for _, reward := range e.Cards {
				refs = append(refs, domain.NewEntityRef(reward.EntityType, reward.ID))
			}
			continue
		}
		for _, used := range cardsOfEvent(event, previousStates[version], battleInputs[version]) {
			ref := domain.NewEntityRef(used.EntityType, used.ID)
			refs = append(refs, ref)
			inDeck[ref] = specs[used]
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

// イベントが述べる、対戦の中のカード（名前を 1 回でまとめて引くため）。fromMatchEvent が同じ関数で引く
func cardsOfEvent(event match.Event, previous match.Match, input match.BattleInput) []card.Ref {
	switch e := event.(type) {
	case *match.CardDrawnEvent:
		return []card.Ref{topOfDrawPile(previous, e.Participant)}
	case *match.CardDiscardedEvent:
		return []card.Ref{handCard(previous, e.Participant, e.HandIndex)}
	case *match.CreaturePlacedEvent:
		return []card.Ref{handCard(previous, e.Participant, e.HandIndex)}
	case *match.CreatureExchangedEvent:
		return []card.Ref{handCard(previous, e.Participant, e.HandIndex), creatureAt(previous, e.Position).Card}
	case *match.TerritoryInvadedEvent:
		return []card.Ref{handCard(previous, e.Participant, e.HandIndex), creatureAt(previous, positionOf(previous, e.Participant)).Card}
	case *match.TerritoryInvadedByMoveEvent:
		return []card.Ref{creatureAt(previous, e.From).Card, creatureAt(previous, e.Position).Card}
	case *match.ItemUsedEvent:
		return []card.Ref{handCard(previous, e.Participant, e.HandIndex)}
	case *match.SpellUsedEvent:
		return []card.Ref{handCard(previous, e.Participant, e.HandIndex)}
	case *match.InvasionRepelledEvent:
		if previous.Invasion != nil {
			return []card.Ref{previous.Invasion.Card}
		}
	case *match.BattleItemEffectActivatedEvent:
		// 戦闘の開始が after より前でも引けるように、出来事の版から入力を引く
		if used := battleCombatant(input, e.Side).Item; used != nil {
			return []card.Ref{used.Card}
		}
	}
	return nil
}

// 直前の状態から引く値（payload に持たない。core#171）。引けない形のイベントは Evolve が断るので、ここでは零の値で返す
func handCard(previous match.Match, participant match.ParticipantIndex, handIndex int) card.Ref {
	if int(participant) >= len(previous.Participants) {
		return card.Ref{}
	}
	hand := previous.Participants[participant].Hand
	if handIndex < 0 || handIndex >= len(hand) {
		return card.Ref{}
	}
	return hand[handIndex]
}

func topOfDrawPile(previous match.Match, participant match.ParticipantIndex) card.Ref {
	if int(participant) >= len(previous.Participants) || len(previous.Participants[participant].DrawPile) == 0 {
		return card.Ref{}
	}
	return previous.Participants[participant].DrawPile[0]
}

func positionOf(previous match.Match, participant match.ParticipantIndex) board.Position {
	if int(participant) >= len(previous.Participants) {
		return board.Position{}
	}
	return previous.Participants[participant].Position
}

func creatureAt(previous match.Match, position board.Position) match.PlacedCreature {
	territory, ok := previous.TerritoryAt(position)
	if !ok {
		return match.PlacedCreature{}
	}
	placed, _ := previous.PlacedCreature(territory.Creature)
	return placed
}

// 今の番の人がいるマス
func currentPosition(previous match.Match) board.Position {
	index, ok := previous.CurrentParticipant()
	if !ok {
		return board.Position{}
	}
	return positionOf(previous, index)
}

// 振った人に続いていたスペルの効果（振るときに効く）
func rollingSpellEffect(previous match.Match, participant match.ParticipantIndex) *SpellEffect {
	if int(participant) >= len(previous.Participants) || previous.Participants[participant].LastingSpellEffect == nil {
		return nil
	}
	effect := fromSpellEffect(previous.Participants[participant].LastingSpellEffect.Effect)
	return &effect
}

// 魔力を得たスペルの根拠（周回を終えた回数と 1 周あたり）。使ったスペルは直前に捨て札の最後に入っている
func magicGainedBasis(previous match.Match, participant match.ParticipantIndex) (int, int) {
	if int(participant) >= len(previous.Participants) {
		return 0, 0
	}
	state := previous.Participants[participant]
	if len(state.DiscardPile) == 0 {
		return state.Laps, 0
	}
	spec := previous.CardSpecs()[state.DiscardPile[len(state.DiscardPile)-1]]
	if spec.Spell == nil || spec.Spell.Effect.GainMagic == nil {
		return state.Laps, 0
	}
	return state.Laps, spec.Spell.Effect.GainMagic.PerLap.Int()
}

// 属性の変更に掛けた倍率（変える前の属性で決まる）。直前の状態の選択肢から引く
func landElementChangeMultiplier(previous match.Match, position board.Position, element board.Element) int {
	for _, choice := range previous.ElementChangeChoices() {
		if choice.Position == position && choice.Element == element {
			return choice.Multiplier
		}
	}
	return 0
}

func boardPosition(position board.Position) BoardPosition {
	return NewBoardPosition(position.X.Int(), position.Y.Int())
}

func cardOf(ref card.Ref, cards map[domain.EntityRef]MatchCard) MatchCard {
	return cards[domain.NewEntityRef(ref.EntityType, ref.ID)]
}

func fromMatchEvent(event match.Event, previous match.Match, cards map[domain.EntityRef]MatchCard, battleInputs map[int64]match.BattleInput, changes MatchEventChanges) (MatchEvent, error) {
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
		return MatchCardDrawnEvent{meta, e.Participant.Int(), cardOf(topOfDrawPile(previous, e.Participant), cards)}, nil
	case *match.DiscardPileShuffledEvent:
		return MatchDiscardPileShuffledEvent{meta, e.Participant.Int(), len(e.DrawPile)}, nil
	case *match.HandLimitExceededEvent:
		handCount := 0
		if int(e.Participant) < len(previous.Participants) {
			handCount = len(previous.Participants[e.Participant].Hand)
		}
		return MatchHandLimitExceededEvent{meta, e.Participant.Int(), handCount, previous.Rules.MaxHandCards.Int()}, nil
	case *match.CardDiscardedEvent:
		return MatchCardDiscardedEvent{meta, e.Participant.Int(), e.HandIndex, cardOf(handCard(previous, e.Participant, e.HandIndex), cards)}, nil
	case *match.DiceRolledEvent:
		dice := e.Dice
		if dice == nil {
			dice = []int{}
		}
		return MatchDiceRolledEvent{meta, e.Participant.Int(), e.Value, dice, rollingSpellEffect(previous, e.Participant)}, nil
	case *match.SpellEffectStartedEvent:
		lasting := e.Effect
		return MatchSpellEffectStartedEvent{meta, e.Participant.Int(), e.Target.Int(), *fromLastingSpellEffect(&lasting)}, nil
	case *match.SpellEffectEndedEvent:
		return MatchSpellEffectEndedEvent{meta, e.Participant.Int()}, nil
	case *match.CreatureDamagedEvent:
		return MatchCreatureDamagedEvent{meta, e.Participant.Int(), boardPosition(e.Position), creatureAt(previous, e.Position).ID.Int(), e.Damage, e.RemainingHitPoints.Int()}, nil
	case *match.CreatureDestroyedEvent:
		return MatchCreatureDestroyedEvent{meta, e.Participant.Int(), boardPosition(e.Position), creatureAt(previous, e.Position).ID.Int()}, nil
	case *match.MagicGainedEvent:
		laps, perLap := magicGainedBasis(previous, e.Participant)
		return MatchMagicGainedEvent{meta, e.Participant.Int(), e.Magic, laps, perLap}, nil
	case *match.MovedEvent:
		return MatchMovedEvent{meta, e.Participant.Int(), boardPosition(e.To)}, nil
	case *match.CheckpointPassedEvent:
		return MatchCheckpointPassedEvent{meta, e.Participant.Int(), boardPosition(positionOf(previous, e.Participant)), e.Magic}, nil
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
			drawn[i] = cardOf(reward, cards)
		}
		return MatchRewardDrawnEvent{meta, e.Participant.Int(), string(e.Pack), drawn}, nil
	case *match.RewardsCompletedEvent:
		return MatchRewardsCompletedEvent{meta}, nil
	case *match.TollMultiplierActivatedEvent:
		// 今の番の人が止まっている領地。倍率は盤面の通行料に掛けて切り捨てる（Decide の payToll と同じ。払った額は続く TollPaidEvent）
		position := currentPosition(previous)
		territory, _ := previous.TerritoryAt(position)
		percent, _ := creatureAt(previous, position).Base.Abilities.TollMultiplier()
		toll := previous.TerritoryToll(territory)
		return MatchTollMultiplierActivatedEvent{meta, boardPosition(position), percent.Int(), toll, toll * percent.Int() / 100}, nil
	case *match.TollPaidEvent:
		position := positionOf(previous, e.Participant)
		territory, _ := previous.TerritoryAt(position)
		owner, _ := previous.TerritoryOwner(territory)
		return MatchTollPaidEvent{meta, e.Participant.Int(), owner.Int(), boardPosition(position), e.Magic}, nil
	case *match.TerritoryRelinquishedEvent:
		return MatchTerritoryRelinquishedEvent{meta, e.Participant.Int(), boardPosition(e.Position), creatureAt(previous, e.Position).ID.Int(), e.Magic}, nil
	case *match.MagicDepletedEvent:
		return MatchMagicDepletedEvent{meta, e.Participant.Int(), e.Magic}, nil
	case *match.CreaturePlacedEvent:
		placed := cardOf(handCard(previous, e.Participant, e.HandIndex), cards)
		return MatchCreaturePlacedEvent{meta, e.Participant.Int(), e.HandIndex, placed, e.Creature.Int(), boardPosition(positionOf(previous, e.Participant)), e.Cost.Int()}, nil
	case *match.TerritoryInvadedEvent:
		position := positionOf(previous, e.Participant)
		invading := cardOf(handCard(previous, e.Participant, e.HandIndex), cards)
		return MatchTerritoryInvadedEvent{meta, e.Participant.Int(), e.HandIndex, invading, boardPosition(position), e.Cost.Int(), cardOf(creatureAt(previous, position).Card, cards)}, nil
	case *match.SpellUsedEvent:
		return MatchSpellUsedEvent{meta, e.Participant.Int(), e.HandIndex, cardOf(handCard(previous, e.Participant, e.HandIndex), cards), e.Cost.Int()}, nil
	case *match.ItemUsedEvent:
		return MatchItemUsedEvent{meta, e.Participant.Int(), e.HandIndex, cardOf(handCard(previous, e.Participant, e.HandIndex), cards), e.Cost.Int()}, nil
	case *match.ItemSkippedEvent:
		return MatchItemSkippedEvent{meta, e.Participant.Int()}, nil
	case *match.TerritoryLeveledUpEvent:
		return MatchTerritoryLeveledUpEvent{meta, e.Participant.Int(), boardPosition(e.Position), e.Level, e.Magic}, nil
	case *match.CreatureMovedEvent:
		return MatchCreatureMovedEvent{meta, e.Participant.Int(), boardPosition(e.From), boardPosition(e.To), creatureAt(previous, e.From).ID.Int()}, nil
	case *match.TerritoryInvadedByMoveEvent:
		mover := creatureAt(previous, e.From)
		return MatchTerritoryInvadedByMoveEvent{meta, e.Participant.Int(), boardPosition(e.From), boardPosition(e.Position), mover.ID.Int(), cardOf(mover.Card, cards), cardOf(creatureAt(previous, e.Position).Card, cards)}, nil
	case *match.LandElementChangedEvent:
		element, err := NewBoardTileElement(e.Element.String())
		if err != nil {
			return nil, err
		}
		return MatchLandElementChangedEvent{meta, e.Participant.Int(), boardPosition(e.Position), element, e.Magic, previous.Rules.BaseLandValue, landElementChangeMultiplier(previous, e.Position, e.Element)}, nil
	case *match.CreatureExchangedEvent:
		exchanged := cardOf(handCard(previous, e.Participant, e.HandIndex), cards)
		returned := cardOf(creatureAt(previous, e.Position).Card, cards)
		return MatchCreatureExchangedEvent{meta, e.Participant.Int(), boardPosition(e.Position), e.HandIndex, exchanged, e.Creature.Int(), e.Cost.Int(), returned}, nil
	case *match.BattleStartedEvent:
		attacker, defender := e.Input.Attacker, e.Input.Defender
		return MatchBattleStartedEvent{meta, MatchBattleStats{attacker.Attack.Int(), attacker.HitPoints.Int()}, MatchBattleStats{defender.Attack.Int(), defender.HitPoints.Int()}}, nil
	case *match.BattleSupportEffectActivatedEvent:
		input, ok := battleInputs[e.AggregateVersion]
		if !ok || previous.Battle == nil {
			return nil, fmt.Errorf("query: support effect event %d without battle", e.AggregateVersion)
		}
		positions := make([]BoardPosition, len(input.AdjacentTerritories))
		for i, position := range input.AdjacentTerritories {
			positions[i] = boardPosition(position)
		}
		change := statChange(previous.Battle.Attacker, match.BattleStatChange{AttackDelta: e.AttackDelta})
		return MatchBattleSupportEffectActivatedEvent{meta, MatchBattleSideAttacker, positions, input.Rules.AdjacentTerritoryBonus.Int(), change}, nil
	case *match.BattleTerrainEffectActivatedEvent:
		input, ok := battleInputs[e.AggregateVersion]
		if !ok || previous.Battle == nil {
			return nil, fmt.Errorf("query: terrain effect event %d without battle", e.AggregateVersion)
		}
		element, err := NewBoardTileElement(input.Land.Element.String())
		if err != nil {
			return nil, err
		}
		change := statChange(previous.Battle.Defender, match.BattleStatChange{HitPointsDelta: e.HitPointsDelta})
		return MatchBattleTerrainEffectActivatedEvent{meta, MatchBattleSideDefender, element, input.Land.Level, input.Rules.TerrainBonusPerLevel.Int(), change}, nil
	case *match.BattleCreatureAbilityActivatedEvent:
		input, ok := battleInputs[e.AggregateVersion]
		if !ok || previous.Battle == nil {
			return nil, fmt.Errorf("query: creature ability event %d without battle", e.AggregateVersion)
		}
		side, err := fromBattleSide(e.Side)
		if err != nil {
			return nil, err
		}
		ability, err := fromBattleAbility(e.Ability)
		if err != nil {
			return nil, err
		}
		converted := MatchBattleCreatureAbilityActivatedEvent{MatchEventMeta: meta, Side: side, Ability: ability}
		if e.Change != nil {
			change := statChange(previous.Battle.Creature(e.Side), *e.Change)
			converted.Change = &change
		}
		for _, owned := range battleCombatant(input, e.Side).Spec.Abilities {
			switch {
			case e.Ability == match.BattleAbilityBattleModifier && owned.BattleModifier != nil:
				converted.BattleModifier = &MatchBattleModifierBasis{AttackModifier: owned.BattleModifier.AttackModifier.Int(), HitPointsModifier: owned.BattleModifier.HitPointsModifier.Int()}
			case e.Ability == match.BattleAbilityBattleRandomStats && owned.BattleRandomStats != nil:
				converted.BattleRandomStats = &MatchBattleRandomStatsBasis{Minimum: owned.BattleRandomStats.Minimum.Int(), Maximum: owned.BattleRandomStats.Maximum.Int()}
			}
		}
		return converted, nil
	case *match.BattleItemEffectActivatedEvent:
		input, ok := battleInputs[e.AggregateVersion]
		if !ok || previous.Battle == nil {
			return nil, fmt.Errorf("query: item effect event %d without battle", e.AggregateVersion)
		}
		side, err := fromBattleSide(e.Side)
		if err != nil {
			return nil, err
		}
		used := battleCombatant(input, e.Side).Item
		if used == nil {
			return nil, fmt.Errorf("query: item effect event %d without item on %s", e.AggregateVersion, e.Side)
		}
		converted := MatchBattleItemEffectActivatedEvent{MatchEventMeta: meta, Side: side, Card: cardOf(used.Card, cards)}
		cardEffect, found := activatedItemEffect(used.Spec.Effects, e.Effect, battleCombatant(input, opponentSide(e.Side)).Spec.Elements.Strings())
		if !found {
			return nil, fmt.Errorf("query: item effect event %d without the effect %q on the item", e.AggregateVersion, e.Effect)
		}
		converted.CardEffect = queryItemEffects(item.Effects{cardEffect})[0]
		switch e.Effect {
		case item.EffectKindAttackModifier:
			converted.Effect = MatchBattleItemEffectAttackModifier
		case item.EffectKindHitPointsModifier:
			converted.Effect = MatchBattleItemEffectHitPointsModifier
		case item.EffectKindAttackOverride:
			converted.Effect = MatchBattleItemEffectAttackOverride
		case item.EffectKindAttackMultiplier:
			converted.Effect = MatchBattleItemEffectAttackMultiplier
		case item.EffectKindIgnoreTerrainEffect:
			converted.Effect = MatchBattleItemEffectIgnoreTerrainEffect
			before := previous.Battle.Creature(opponentSide(e.Side))
			ignored := before
			ignored.TerrainEffectIgnored = true
			change := statChange(before, match.BattleStatChange{HitPointsDelta: ignored.Stats().HitPoints.Int() - before.Stats().HitPoints.Int()})
			converted.OpponentChange = &change
		default:
			return nil, fmt.Errorf("query: unknown item effect: %q", e.Effect)
		}
		if e.Change != nil {
			change := statChange(previous.Battle.Creature(e.Side), *e.Change)
			converted.Change = &change
		}
		return converted, nil
	case *match.CreatureStruckEvent:
		striker, err := fromBattleSide(e.Striker)
		if err != nil {
			return nil, err
		}
		// 攻撃の値はイベントに持たせず、直前の状態の戦闘中の AT から作る（Evolve が使わず、直前の状態から引けるため。core#174）
		if previous.Battle == nil {
			return nil, fmt.Errorf("query: creature struck event %d without battle", e.AggregateVersion)
		}
		attack := previous.Battle.Attacker.Stats().Attack
		if e.Striker == match.BattleSideDefender {
			attack = previous.Battle.Defender.Stats().Attack
		}
		return MatchCreatureStruckEvent{meta, striker, attack.Int(), e.Damage.Int(), e.RemainingHitPoints.Int()}, nil
	case *match.TerritoryConqueredEvent:
		if previous.Invasion == nil {
			return nil, fmt.Errorf("query: conquered event %d without invasion", e.AggregateVersion)
		}
		return MatchTerritoryConqueredEvent{meta, e.Participant.Int(), boardPosition(previous.Invasion.Position), e.Creature.Int(), e.TerritoryHitPoints.Int()}, nil
	case *match.InvasionRepelledEvent:
		invasion, battle := previous.Invasion, previous.Battle
		if invasion == nil || battle == nil {
			return nil, fmt.Errorf("query: repelled event %d without battle", e.AggregateVersion)
		}
		// 生き残れば、手札から侵略したときは手札に、移動で侵略したときは元の領地に戻る（Evolve と同じ決まり）
		survived := battle.Attacker.Stats().HitPoints.Int() > 0
		var returnedTo *BoardPosition
		var returnedHitPoints *int
		if survived && invasion.From != nil && e.ReturnedHitPoints != nil {
			to, hitPoints := boardPosition(*invasion.From), e.ReturnedHitPoints.Int()
			returnedTo, returnedHitPoints = &to, &hitPoints
		}
		return MatchInvasionRepelledEvent{meta, e.Participant.Int(), boardPosition(invasion.Position), cardOf(invasion.Card, cards), survived && invasion.From == nil, e.TerritoryHitPoints.Int(), returnedTo, returnedHitPoints}, nil
	default:
		return nil, fmt.Errorf("query: unexpected match event: %T", event)
	}
}

func battleCombatant(input match.BattleInput, side match.BattleSide) match.BattleCombatant {
	if side == match.BattleSideDefender {
		return input.Defender
	}
	return input.Attacker
}

// 直前の戦闘の中の値に増減を当てた後の値を添える（下限は増減が既に止まった後の量なので、足すだけで合う）
func statChange(before match.BattleCreature, change match.BattleStatChange) MatchBattleStatChange {
	stats := before.Stats()
	return MatchBattleStatChange{
		AttackDelta: change.AttackDelta, HitPointsDelta: change.HitPointsDelta,
		Attack: stats.Attack.Int() + change.AttackDelta, HitPoints: stats.HitPoints.Int() + change.HitPointsDelta,
	}
}

// 発動したカードの効果。同じ種類の最初のもの（倍率は相手の属性で制約を満たす一番高いもの）
func activatedItemEffect(effects item.Effects, kind item.EffectKind, opponentElements []string) (item.Effect, bool) {
	var activated item.Effect
	found := false
	for _, effect := range effects {
		if effect.Kind() != kind {
			continue
		}
		if effect.AttackMultiplier == nil {
			return effect, true
		}
		if effect.AttackMultiplier.Constraints.SatisfiedByOpponentElements(opponentElements) && (!found || effect.AttackMultiplier.Percent > activated.AttackMultiplier.Percent) {
			activated, found = effect, true
		}
	}
	return activated, found
}

func opponentSide(side match.BattleSide) match.BattleSide {
	if side == match.BattleSideAttacker {
		return match.BattleSideDefender
	}
	return match.BattleSideAttacker
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
	case match.BattleAbilityBattleModifier:
		return MatchBattleAbilityBattleModifier, nil
	case match.BattleAbilityBattleRandomStats:
		return MatchBattleAbilityBattleRandomStats, nil
	case match.BattleAbilityAttackImmunity:
		return MatchBattleAbilityAttackImmunity, nil
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
