package match

import (
	"encoding/json"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
)

type sealedEvent interface {
	isMatchEvent()
}

type eventMarker struct{}

func (eventMarker) isMatchEvent() {}

type Event interface {
	sealedEvent
	domain.Event[ID]
}

type EventMeta = domain.EventMeta[ID]

// 始めた時点の条件の写し。部屋は持たない（部屋のイベントが対戦の ID を持つので、対戦が部屋を指すとパッケージが循環する。
// 対戦から部屋をたどるのは、結果を知らせる ⑦ で部屋の read model から引く）
type CreatedEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	Participants []Participant `json:"participants"`
	Board        Board         `json:"board"`
	Rules        Rules         `json:"rules"`
}

// 無作為に決めた順番（参加者の番号の並び。早い順）。サイコロの振り合いにしないのは、順番を決める場面を画面に出さないため（core#100）
type TurnOrderDecidedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Order       []ParticipantIndex `json:"order"`
}

// 参加者ごとの山札の並び（参加者の順。先頭から引く）。並びは見せないが、読み直しても同じ状態にするため残す
type DecksShuffledEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	DrawPiles   [][]card.Ref `json:"draw_piles"`
}

// 全員が山札の先頭から同じ枚数を手札に引いた（山札が足りなければ、ある分だけ）
type HandsDealtEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Cards       int `json:"cards"`
}

// 引いたカード。山札の先頭と同じはず（読み直しで食い違えば不整合）
type CardDrawnEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Card        card.Ref         `json:"card"`
}

// 手札から捨てたカード（手札の何番目か。0 始まり）
type CardDiscardedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	HandIndex   int              `json:"hand_index"`
	Card        card.Ref         `json:"card"`
}

// 止まった空き地に、手札のクリーチャーを配置した（手札の何番目か。0 始まり）。コストの分の魔力を払い、その土地が領地になる。
// 性能は対戦を始めたときの写しから決まるので、イベントには書き写さない
type CreatureDeployedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex   `json:"participant"`
	HandIndex   int                `json:"hand_index"`
	Card        card.Ref           `json:"card"`
	Creature    DeployedCreatureID `json:"creature"`
	Position    board.Position     `json:"position"`
	Cost        card.Cost          `json:"cost"`
}

// 他人の領地に止まって通行料を払った。足りなくても全額を払い、魔力は負になる（Magic は払った魔力で、持ち主が同じだけ受け取る）
type TollPaidEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Owner       ParticipantIndex `json:"owner"`
	Position    board.Position   `json:"position"`
	Magic       int              `json:"magic"`
}

// 魔力が負の間に、自分の領地を手放した（Magic は受け取った魔力）。クリーチャーは盤面から外れ、土地は空く（レベルは土地に残る。core#146）
type TerritoryRelinquishedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex   `json:"participant"`
	Position    board.Position     `json:"position"`
	Creature    DeployedCreatureID `json:"creature"`
	Magic       int                `json:"magic"`
}

// 領地が無くなっても魔力が負のままだった。拠点からやり直す（Magic は与えられた開始時の魔力）
type MagicDepletedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Magic       int              `json:"magic"`
}

// 止まった他人の領地を、手札のクリーチャーで侵略した（手札の何番目か。0 始まり）。コストの分の魔力を払う。
// 性能は対戦を始めたときの写しから決まるので、イベントには書き写さない
type TerritoryInvadedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	HandIndex   int              `json:"hand_index"`
	Card        card.Ref         `json:"card"`
	Position    board.Position   `json:"position"`
	Cost        card.Cost        `json:"cost"`
}

// 戦闘に手札のアイテムを出した（手札の何番目か。0 始まり）。コストの分の魔力を払い、手札から無くなる。攻める側も守る側も同じ形で、効果は続く戦闘で BattleStatModifiedEvent になる
type ItemUsedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	HandIndex   int              `json:"hand_index"`
	Card        card.Ref         `json:"card"`
	Cost        card.Cost        `json:"cost"`
}

// 戦闘にアイテムを出さなかった（攻める側も守る側も同じ形）。段階のイベントは作らず、出さなかったという事実で次の段階に進める
type ItemSkippedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
}

// 戦闘が始まった。効果を当てる前の両方の AT・HP で、計算の経過の始まり
type BattleStartedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Attacker    BattleStats `json:"attacker"`
	Defender    BattleStats `json:"defender"`
}

// 効果 1 つが戦闘の AT・HP を変えた。量の増減は効果の種類によらずこのイベントで表し、何の効果かは Source が持つ。
// Delta は下限（0）で止まった後の実際の増減で、Attack・HitPoints は当てた後の値（画面が計算せずに再生するため）
type BattleStatModifiedEvent struct {
	EventMeta      `json:"-"`
	eventMarker    `json:"-"`
	Side           BattleSide         `json:"side"`
	Source         BattleStatSource   `json:"source"`
	AttackDelta    int                `json:"attack_delta"`
	HitPointsDelta int                `json:"hit_points_delta"`
	Attack         creature.Attack    `json:"attack"`
	HitPoints      creature.HitPoints `json:"hit_points"`
}

// 戦闘の攻撃が 1 回当たった（Striker が攻撃した側。RemainingHitPoints は受けた側の、効果で増えた分も含む戦闘の中の残りの体力で、0 なら倒れた）
type CreatureStruckEvent struct {
	EventMeta          `json:"-"`
	eventMarker        `json:"-"`
	Striker            BattleSide         `json:"striker"`
	Damage             creature.Attack    `json:"damage"`
	RemainingHitPoints creature.HitPoints `json:"remaining_hit_points"`
}

// 守る側のクリーチャーを倒して土地を奪った。土地のレベルは引き継ぐ。Creature は領地に入ったクリーチャーの新しい ID、TerritoryHitPoints はその体力
type TerritoryConqueredEvent struct {
	EventMeta          `json:"-"`
	eventMarker        `json:"-"`
	Participant        ParticipantIndex   `json:"participant"`
	Position           board.Position     `json:"position"`
	DefeatedCreature   DeployedCreatureID `json:"defeated_creature"`
	Creature           DeployedCreatureID `json:"creature"`
	TerritoryHitPoints creature.HitPoints `json:"territory_hit_points"`
}

// 守る側のクリーチャーを倒せなかった。攻めたクリーチャーは生きていれば手札に戻り、倒れていれば捨て札になる。続けて通行料を払う。TerritoryHitPoints は領地のクリーチャーに残る体力
type InvasionRepelledEvent struct {
	EventMeta          `json:"-"`
	eventMarker        `json:"-"`
	Participant        ParticipantIndex   `json:"participant"`
	Position           board.Position     `json:"position"`
	Card               card.Ref           `json:"card"`
	ReturnedToHand     bool               `json:"returned_to_hand"`
	TerritoryHitPoints creature.HitPoints `json:"territory_hit_points"`
	// 移動で侵略して生き残ったときだけ持つ。戻った元の領地の位置と、戻ったクリーチャーの体力（倒れたときは無く、元の土地は空き地になる）
	ReturnedTo        *board.Position     `json:"returned_to,omitempty"`
	ReturnedHitPoints *creature.HitPoints `json:"returned_hit_points,omitempty"`
}

// 通った自分の領地の土地のレベルを上げた（Level は上げた後のレベル、Magic は払った魔力）
type TerritoryLeveledUpEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Position    board.Position   `json:"position"`
	Level       int              `json:"level"`
	Magic       int              `json:"magic"`
}

// 通った自分の領地のクリーチャーを手札のクリーチャーと入れ替えた。Creature は領地に入ったクリーチャーの新しい ID で、体力はカードの値。ReturnedCard は手札の最後に戻ったカード。Cost は払った魔力
type CreatureExchangedEvent struct {
	EventMeta    `json:"-"`
	eventMarker  `json:"-"`
	Participant  ParticipantIndex   `json:"participant"`
	Position     board.Position     `json:"position"`
	HandIndex    int                `json:"hand_index"`
	Card         card.Ref           `json:"card"`
	Creature     DeployedCreatureID `json:"creature"`
	Cost         card.Cost          `json:"cost"`
	ReturnedCard card.Ref           `json:"returned_card"`
}

// 通った自分の領地の土地の属性を変えた（Element は変えた後の属性、Magic は払った魔力）。土地のレベルとクリーチャーはそのまま
type LandElementChangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Position    board.Position   `json:"position"`
	Element     board.Element    `json:"element"`
	Magic       int              `json:"magic"`
}

// 通った自分の領地のクリーチャーを空き地へ動かした。行き先が領地になり、元の土地は空き地になる（土地のレベルはどちらも土地に残る）
type CreatureMovedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex   `json:"participant"`
	From        board.Position     `json:"from"`
	To          board.Position     `json:"to"`
	Creature    DeployedCreatureID `json:"creature"`
}

// 通った自分の領地のクリーチャーを他人の領地へ動かし、侵略した（続けてアイテムを選び、戦闘する）。Position は攻める領地
type TerritoryInvadedByMoveEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex   `json:"participant"`
	From        board.Position     `json:"from"`
	Position    board.Position     `json:"position"`
	Creature    DeployedCreatureID `json:"creature"`
}

// 振った目
type DiceRolledEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Value       int              `json:"value"`
}

// 1 歩進んで入ったマス
type MovedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	To          board.Position   `json:"to"`
}

// まだ印の無いチェックポイントに入って印を付け、魔力が増えた（Magic は足した量）
type CheckpointPassedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Checkpoint  board.Position   `json:"checkpoint"`
	Magic       int              `json:"magic"`
}

// 全チェックポイントに印を付けて拠点に入り、印を外して魔力が増えた（Magic は足した量）
type LapCompletedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Magic       int              `json:"magic"`
}

// 今の番の人がターンを終えた（次の人・ラウンドは Evolve が決める）
type TurnEndedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
}

// 対戦が終わった
type MatchEndedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Winners     []ParticipantIndex `json:"winners"`
	Ranks       []int              `json:"ranks"`
	Reason      EndReason          `json:"reason"`
}

// プレイヤーが戦利品のパックから引いたカード（重ならない。候補が足りなければある分だけ）。
// 引いた結果をイベントに残すのは、読み直しても同じ状態にし、ポリシーが所持品に配るときの元にするため
type RewardDrawnEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Pack        PackKey          `json:"pack"`
	Cards       []card.Ref       `json:"cards"`
}

// プレイヤー全員が戦利品を受け取った
type RewardsCompletedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

var (
	createdEventType                = domain.NewEventName("CreatedEvent")
	turnOrderDecidedEventType       = domain.NewEventName("TurnOrderDecidedEvent")
	decksShuffledEventType          = domain.NewEventName("DecksShuffledEvent")
	handsDealtEventType             = domain.NewEventName("HandsDealtEvent")
	cardDrawnEventType              = domain.NewEventName("CardDrawnEvent")
	cardDiscardedEventType          = domain.NewEventName("CardDiscardedEvent")
	diceRolledEventType             = domain.NewEventName("DiceRolledEvent")
	movedEventType                  = domain.NewEventName("MovedEvent")
	checkpointPassedEventType       = domain.NewEventName("CheckpointPassedEvent")
	lapCompletedEventType           = domain.NewEventName("LapCompletedEvent")
	turnEndedEventType              = domain.NewEventName("TurnEndedEvent")
	matchEndedEventType             = domain.NewEventName("MatchEndedEvent")
	rewardDrawnEventType            = domain.NewEventName("RewardDrawnEvent")
	rewardsCompletedEventType       = domain.NewEventName("RewardsCompletedEvent")
	creatureDeployedEventType       = domain.NewEventName("CreatureDeployedEvent")
	tollPaidEventType               = domain.NewEventName("TollPaidEvent")
	territoryRelinquishedEventType  = domain.NewEventName("TerritoryRelinquishedEvent")
	magicDepletedEventType          = domain.NewEventName("MagicDepletedEvent")
	territoryInvadedEventType       = domain.NewEventName("TerritoryInvadedEvent")
	itemUsedEventType               = domain.NewEventName("ItemUsedEvent")
	creatureStruckEventType         = domain.NewEventName("CreatureStruckEvent")
	territoryConqueredEventType     = domain.NewEventName("TerritoryConqueredEvent")
	invasionRepelledEventType       = domain.NewEventName("InvasionRepelledEvent")
	itemSkippedEventType            = domain.NewEventName("ItemSkippedEvent")
	battleStartedEventType          = domain.NewEventName("BattleStartedEvent")
	battleStatModifiedEventType     = domain.NewEventName("BattleStatModifiedEvent")
	territoryLeveledUpEventType     = domain.NewEventName("TerritoryLeveledUpEvent")
	creatureExchangedEventType      = domain.NewEventName("CreatureExchangedEvent")
	landElementChangedEventType     = domain.NewEventName("LandElementChangedEvent")
	creatureMovedEventType          = domain.NewEventName("CreatureMovedEvent")
	territoryInvadedByMoveEventType = domain.NewEventName("TerritoryInvadedByMoveEvent")
)

func (*CreatedEvent) EventType() domain.EventName {
	return createdEventType
}

func (*TurnOrderDecidedEvent) EventType() domain.EventName {
	return turnOrderDecidedEventType
}

func (*DecksShuffledEvent) EventType() domain.EventName {
	return decksShuffledEventType
}

func (*HandsDealtEvent) EventType() domain.EventName {
	return handsDealtEventType
}

// payload の形を変えるときは上げ、Unmarshal で古い版を読み替える（保存済みのイベントは書き換えないため）
func (*CreatedEvent) SchemaVersion() int {
	return 1
}

func (*TurnOrderDecidedEvent) SchemaVersion() int {
	return 1
}

func (*DecksShuffledEvent) SchemaVersion() int {
	return 1
}

func (*HandsDealtEvent) SchemaVersion() int {
	return 1
}

func (*CardDrawnEvent) EventType() domain.EventName {
	return cardDrawnEventType
}

func (*CardDrawnEvent) SchemaVersion() int {
	return 1
}

func (*CardDiscardedEvent) EventType() domain.EventName {
	return cardDiscardedEventType
}

func (*CardDiscardedEvent) SchemaVersion() int {
	return 1
}

func (*DiceRolledEvent) EventType() domain.EventName {
	return diceRolledEventType
}

func (*DiceRolledEvent) SchemaVersion() int {
	return 1
}

func (*MovedEvent) EventType() domain.EventName {
	return movedEventType
}

func (*MovedEvent) SchemaVersion() int {
	return 1
}

func (*CheckpointPassedEvent) EventType() domain.EventName {
	return checkpointPassedEventType
}

func (*CheckpointPassedEvent) SchemaVersion() int {
	return 1
}

func (*LapCompletedEvent) EventType() domain.EventName {
	return lapCompletedEventType
}

func (*LapCompletedEvent) SchemaVersion() int {
	return 1
}

func (*TurnEndedEvent) EventType() domain.EventName {
	return turnEndedEventType
}

func (*TurnEndedEvent) SchemaVersion() int {
	return 1
}

func (*MatchEndedEvent) EventType() domain.EventName {
	return matchEndedEventType
}

func (*MatchEndedEvent) SchemaVersion() int {
	return 1
}

func (*RewardDrawnEvent) EventType() domain.EventName {
	return rewardDrawnEventType
}

func (*RewardDrawnEvent) SchemaVersion() int {
	return 1
}

func (*RewardsCompletedEvent) EventType() domain.EventName {
	return rewardsCompletedEventType
}

func (*RewardsCompletedEvent) SchemaVersion() int {
	return 1
}

func (*CreatureDeployedEvent) EventType() domain.EventName {
	return creatureDeployedEventType
}

func (*CreatureDeployedEvent) SchemaVersion() int {
	return 1
}

func (*TollPaidEvent) EventType() domain.EventName {
	return tollPaidEventType
}

func (*TollPaidEvent) SchemaVersion() int {
	return 1
}

func (*TerritoryRelinquishedEvent) EventType() domain.EventName {
	return territoryRelinquishedEventType
}

func (*TerritoryRelinquishedEvent) SchemaVersion() int {
	return 1
}

func (*MagicDepletedEvent) EventType() domain.EventName {
	return magicDepletedEventType
}

func (*MagicDepletedEvent) SchemaVersion() int {
	return 1
}

func (*TerritoryInvadedEvent) EventType() domain.EventName {
	return territoryInvadedEventType
}

func (*TerritoryInvadedEvent) SchemaVersion() int {
	return 1
}

func (*ItemUsedEvent) EventType() domain.EventName {
	return itemUsedEventType
}

func (*ItemUsedEvent) SchemaVersion() int {
	return 1
}

func (*CreatureStruckEvent) EventType() domain.EventName {
	return creatureStruckEventType
}

func (*CreatureStruckEvent) SchemaVersion() int {
	return 1
}

func (*TerritoryConqueredEvent) EventType() domain.EventName {
	return territoryConqueredEventType
}

func (*TerritoryConqueredEvent) SchemaVersion() int {
	return 1
}

func (*InvasionRepelledEvent) EventType() domain.EventName {
	return invasionRepelledEventType
}

func (*InvasionRepelledEvent) SchemaVersion() int {
	return 1
}

func (*ItemSkippedEvent) EventType() domain.EventName {
	return itemSkippedEventType
}

func (*ItemSkippedEvent) SchemaVersion() int {
	return 1
}

func (*BattleStartedEvent) EventType() domain.EventName {
	return battleStartedEventType
}

func (*BattleStartedEvent) SchemaVersion() int {
	return 1
}

func (*BattleStatModifiedEvent) EventType() domain.EventName {
	return battleStatModifiedEventType
}

func (*BattleStatModifiedEvent) SchemaVersion() int {
	return 1
}

func (*TerritoryLeveledUpEvent) EventType() domain.EventName {
	return territoryLeveledUpEventType
}

func (*TerritoryLeveledUpEvent) SchemaVersion() int {
	return 1
}

func (*CreatureExchangedEvent) EventType() domain.EventName {
	return creatureExchangedEventType
}

func (*CreatureExchangedEvent) SchemaVersion() int {
	return 1
}

func (*LandElementChangedEvent) EventType() domain.EventName {
	return landElementChangedEventType
}

func (*LandElementChangedEvent) SchemaVersion() int {
	return 1
}

func (*CreatureMovedEvent) EventType() domain.EventName {
	return creatureMovedEventType
}

func (*CreatureMovedEvent) SchemaVersion() int {
	return 1
}

func (*TerritoryInvadedByMoveEvent) EventType() domain.EventName {
	return territoryInvadedByMoveEventType
}

func (*TerritoryInvadedByMoveEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{EntityType: entityType, EventTypes: []domain.EventName{
		createdEventType, turnOrderDecidedEventType, decksShuffledEventType, handsDealtEventType,
		cardDrawnEventType, cardDiscardedEventType, diceRolledEventType, movedEventType, checkpointPassedEventType, lapCompletedEventType, turnEndedEventType, matchEndedEventType,
		rewardDrawnEventType, rewardsCompletedEventType, creatureDeployedEventType, tollPaidEventType, territoryRelinquishedEventType, magicDepletedEventType,
		territoryInvadedEventType, itemUsedEventType, creatureStruckEventType, territoryConqueredEventType, invasionRepelledEventType, itemSkippedEventType,
		battleStartedEventType, battleStatModifiedEventType, territoryLeveledUpEventType, landElementChangedEventType, creatureExchangedEventType,
		creatureMovedEventType, territoryInvadedByMoveEventType,
	}}
}

type EventType domain.EventName

func NewEventType(name domain.EventName) domain.EventType[ID, Event] {
	return EventType(name)
}

func (t EventType) Unmarshal(meta EventMeta, _ int, payload []byte) (Event, error) {
	var event Event
	switch t {
	case EventType(createdEventType):
		event = &CreatedEvent{EventMeta: meta}
	case EventType(turnOrderDecidedEventType):
		event = &TurnOrderDecidedEvent{EventMeta: meta}
	case EventType(decksShuffledEventType):
		event = &DecksShuffledEvent{EventMeta: meta}
	case EventType(handsDealtEventType):
		event = &HandsDealtEvent{EventMeta: meta}
	case EventType(cardDrawnEventType):
		event = &CardDrawnEvent{EventMeta: meta}
	case EventType(cardDiscardedEventType):
		event = &CardDiscardedEvent{EventMeta: meta}
	case EventType(diceRolledEventType):
		event = &DiceRolledEvent{EventMeta: meta}
	case EventType(movedEventType):
		event = &MovedEvent{EventMeta: meta}
	case EventType(checkpointPassedEventType):
		event = &CheckpointPassedEvent{EventMeta: meta}
	case EventType(lapCompletedEventType):
		event = &LapCompletedEvent{EventMeta: meta}
	case EventType(turnEndedEventType):
		event = &TurnEndedEvent{EventMeta: meta}
	case EventType(matchEndedEventType):
		event = &MatchEndedEvent{EventMeta: meta}
	case EventType(rewardDrawnEventType):
		event = &RewardDrawnEvent{EventMeta: meta}
	case EventType(rewardsCompletedEventType):
		event = &RewardsCompletedEvent{EventMeta: meta}
	case EventType(creatureDeployedEventType):
		event = &CreatureDeployedEvent{EventMeta: meta}
	case EventType(tollPaidEventType):
		event = &TollPaidEvent{EventMeta: meta}
	case EventType(territoryRelinquishedEventType):
		event = &TerritoryRelinquishedEvent{EventMeta: meta}
	case EventType(magicDepletedEventType):
		event = &MagicDepletedEvent{EventMeta: meta}
	case EventType(territoryInvadedEventType):
		event = &TerritoryInvadedEvent{EventMeta: meta}
	case EventType(itemUsedEventType):
		event = &ItemUsedEvent{EventMeta: meta}
	case EventType(creatureStruckEventType):
		event = &CreatureStruckEvent{EventMeta: meta}
	case EventType(territoryConqueredEventType):
		event = &TerritoryConqueredEvent{EventMeta: meta}
	case EventType(invasionRepelledEventType):
		event = &InvasionRepelledEvent{EventMeta: meta}
	case EventType(itemSkippedEventType):
		event = &ItemSkippedEvent{EventMeta: meta}
	case EventType(battleStartedEventType):
		event = &BattleStartedEvent{EventMeta: meta}
	case EventType(battleStatModifiedEventType):
		event = &BattleStatModifiedEvent{EventMeta: meta}
	case EventType(territoryLeveledUpEventType):
		event = &TerritoryLeveledUpEvent{EventMeta: meta}
	case EventType(creatureExchangedEventType):
		event = &CreatureExchangedEvent{EventMeta: meta}
	case EventType(landElementChangedEventType):
		event = &LandElementChangedEvent{EventMeta: meta}
	case EventType(creatureMovedEventType):
		event = &CreatureMovedEvent{EventMeta: meta}
	case EventType(territoryInvadedByMoveEventType):
		event = &TerritoryInvadedByMoveEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
