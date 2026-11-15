package match

import (
	"encoding/json"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/element"
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
type DrawPilesShuffledEvent struct {
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

// 山札の先頭を 1 枚引いた。どのカードかは山札の並び（混ぜたイベント）から決まるので持たない（core#171）
type CardDrawnEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
}

// 山札が空のまま引くときに、捨て札を混ぜて山札にした（新しい山札の並び。先頭から引く）。並びは見せないが、読み直しても同じ状態にするため残す
type DiscardPileShuffledEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	DrawPile    []card.Ref       `json:"draw_pile"`
}

// ターンを終えるときに、手札が上限を超えていた。上限まで捨ててからターンを終える
type HandLimitExceededEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
}

// 手札の何番目か（0 始まり）を捨てた。捨て札に入る。カードは手札の番号から決まるので持たない
type CardDiscardedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	HandIndex   int              `json:"hand_index"`
}

// 止まった空き地に、手札のクリーチャーを配置した（手札の何番目か。0 始まり）。コストの分の魔力を払い、その土地が領地になる。
// カード・位置・性能は、手札の番号・今いるマス・対戦を始めたときの写しから決まるので持たない
type CreaturePlacedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	HandIndex   int              `json:"hand_index"`
	Creature    PlacedCreatureID `json:"creature"`
	Cost        card.Cost        `json:"cost"`
}

// 他人の領地に止まって通行料を払った。足りなくても全額を払い、魔力は負になる（Magic は払った魔力で、今いるマスの領地の持ち主が同じだけ受け取る）
type TollPaidEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Magic       int              `json:"magic"`
}

// 通行料を払う直前に、今いるマスの領地のクリーチャーの通行料の倍率が発動した。倍率を掛けて切り捨てた額は、続く TollPaidEvent が払う。
// 倍率を持っていれば、効き目が無くても（100%）出す（2026-11-11 オーナー。core#159）。状態は変えない。
// 発動したことだけを持つ（位置・倍率・盤面の通行料は直前の状態から、掛けた額は続く払ったイベントから決まる。2026-11-13 オーナー。core#171）
type TollCreatureEffectActivatedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

// 魔力が負の間に、自分の領地を手放した（Magic は受け取った魔力）。クリーチャーは盤面から外れて持ち主の捨て札に入り、土地は空く（レベルは土地に残る。core#146）
type TerritoryRelinquishedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Position    board.Position   `json:"position"`
	Magic       int              `json:"magic"`
}

// 領地が無くなっても魔力が負のままだった。拠点からやり直す（Magic は与えられた開始時の魔力）
type MagicDepletedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Magic       int              `json:"magic"`
}

// 止まった他人の領地を、手札のクリーチャーで侵略した（手札の何番目か。0 始まり）。コストの分の魔力を払う。
// カード・位置・守るクリーチャーは、手札の番号・今いるマス・その領地から決まるので持たない
type TerritoryInvadedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	HandIndex   int              `json:"hand_index"`
	Cost        card.Cost        `json:"cost"`
}

// 戦闘に手札のアイテムを出した（手札の何番目か。0 始まり）。コストの分の魔力を払い、手札から捨て札に移る。攻める側も守る側も同じ形で、効果は続く戦闘で BattleItemEffectActivatedEvent になる
type ItemUsedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	HandIndex   int              `json:"hand_index"`
	Cost        card.Cost        `json:"cost"`
}

// サイコロを振る前に、手札のスペルを使った（手札の何番目か。0 始まり）。コストの分の魔力を払い、手札から捨て札に移る。効果は続くイベントになる
type SpellUsedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	HandIndex   int              `json:"hand_index"`
	Cost        card.Cost        `json:"cost"`
}

// スペルの効果で魔力を得た（Magic は得た魔力 = (周回を終えた回数 + 1) × スペルの 1 周あたり。根拠は直前の状態と写しから決まるので持たない）
type MagicGainedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Magic       int              `json:"magic"`
}

// スペルを使い、対象のプレイヤーにターンをまたいで効き続ける効果がかかった（Participant は使った人。続いていた効果は上書きする）
type SpellEffectStartedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex   `json:"participant"`
	Target      ParticipantIndex   `json:"target"`
	Effect      LastingSpellEffect `json:"effect"`
}

// 自分のターンを終えて、続いていたスペルの効果が終わった
type SpellEffectEndedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
}

// スペルで、Position の領地のクリーチャーにダメージを与えた（Participant は使った人。RemainingHitPoints は残りの HP で、0 なら続けて破壊される）
type CreatureDamagedEvent struct {
	EventMeta          `json:"-"`
	eventMarker        `json:"-"`
	Participant        ParticipantIndex   `json:"participant"`
	Position           board.Position     `json:"position"`
	Damage             int                `json:"damage"`
	RemainingHitPoints creature.HitPoints `json:"remaining_hit_points"`
}

// 破壊は場面と立場ごとに 3 つのイベントに分ける（侵略したクリーチャー・守るクリーチャー・ボード。2026-11-15 オーナー。core#181）。
// どれもカードは持ち主の捨て札に入り、領地にいたなら領地から外れて土地は空き地になる（レベルは土地に残る）。Participant は今の番の人で、
// どのクリーチャーかは状態から決まるので持たない（戦闘は戦闘の中で HP が 0 の側、ボードは HP が 0 の領地のクリーチャー）。
// 「破壊されたとき」の効果は、発動するイベントをこの 3 つから選ぶ

// 戦闘で、侵略したクリーチャーが破壊された（移動で侵略していたら元の土地が空く）
type InvadingCreatureDestroyedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
}

// 戦闘で、守るクリーチャーが破壊された
type DefendingCreatureDestroyedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
}

// 戦闘の外（今はスペルのダメージ）で、HP が 0 になった領地のクリーチャーが破壊された
type BoardCreatureDestroyedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
}

// 戦闘にアイテムを出さなかった（攻める側も守る側も同じ形）。段階のイベントは作らず、出さなかったという事実で次の段階に進める
type ItemSkippedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
}

// 戦闘が始まった。戦闘の入力（状況と、両方が選んだアイテム）をすべて持ち、続く戦闘の出来事はこの入力と乱数の元から決まる。
// 効果を当てる前の AT・HP は、両方のクリーチャーの性能の AT と戦闘の前の HP（core#171）
type BattleStartedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Input       BattleInput `json:"input"`
}

// 戦闘の発動のイベントは、出どころ（支援効果・地形効果・クリーチャーの能力・アイテムの能力）ごとに種類を分ける。1 つにまとめると、
// 名前がドメインの言葉にならず、出どころごとに要る根拠が違う payload を 1 つの形で持つことになり、足すときにほかへ響くため
// （2026-11-14 オーナー。core#175）。どれも、得た増減は下限で止まった後の実際の量だけを持つ。当てた後の AT・HP と根拠の値
// （隣の領地・土地・能力やアイテムの値）は持たない（当てた後の値は戦闘の中の値の層から計算でき、根拠は戦闘の入力にあるため。core#171）。
// スペルの効果（守る側のクリーチャーにつく）の発動は、戦闘に効くスペルを作るときに足す

// 支援効果が発動した（戦場の隣にあるその側の領地の数 × ルールの値だけ AT が増える）。隣の領地が無ければ発動しない
type BattleSupportEffectActivatedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Side        BattleSide `json:"side"`
	AttackDelta int        `json:"attack_delta"`
}

// 守る側に地形効果が発動した（土地の属性がクリーチャーの属性と合えば、レベル × ルールの値だけ HP が増える）。合わなければ発動しない
type BattleTerrainEffectActivatedEvent struct {
	EventMeta      `json:"-"`
	eventMarker    `json:"-"`
	HitPointsDelta int `json:"hit_points_delta"`
}

// Side の側のクリーチャーの効果が発動した（効果の種類ごとのタイミングは effect.Effect のコメント。アイテムと同じ書き方。core#178）。
// AT・HP を変える効果（増減・上書き）は Change を持ち、増減が 0 でも出す。得た状態（先に・後に攻撃するの攻撃の順、受けるダメージを 0 にする）は
// 効果の種類から決まるので書かない。先に・後に攻撃するは、効き目が無いとき（両方が先に攻撃するで打ち消し合う等）も持っていれば出す
// （2026-11-11 オーナー。core#158）。受けるダメージを 0 にするは、攻撃してきたクリーチャーが制約を満たしたときだけ出す
type BattleCreatureEffectActivatedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Side        BattleSide        `json:"side"`
	Effect      effect.Kind       `json:"effect"`
	Change      *BattleStatChange `json:"change,omitempty"`
}

// Side の側が出したアイテムの効果が発動した（効果の種類ごとのタイミングは effect.Effect のコメント）。AT・HP を変える効果は Change を持つ
// （増減・上書きは増減が 0 でも出し、倍率は掛けたものだけが出す）。地形効果を無視するは Change を持たず、得た状態（相手の地形効果を
// 無視する）は効果の種類から決まるので書かない。アイテムの何番目の効果かは持たない（1 枚が同じ種類の効果を重ねて持つ見本が無く、
// どの制約が合ったかは入力から引けるため。重ねるカードを作るときに見直す。2026-11-14 オーナー。core#177）
type BattleItemEffectActivatedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Side        BattleSide        `json:"side"`
	Effect      effect.Kind       `json:"effect"`
	Change      *BattleStatChange `json:"change,omitempty"`
}

// 戦闘の攻撃が 1 回当たった（Striker が攻撃した側。RemainingHitPoints は受けた側の、効果で増えた分も含む戦闘の中の残りの体力で、0 なら倒れた）
type CreatureStruckEvent struct {
	EventMeta          `json:"-"`
	eventMarker        `json:"-"`
	Striker            BattleSide         `json:"striker"`
	Damage             creature.Attack    `json:"damage"`
	RemainingHitPoints creature.HitPoints `json:"remaining_hit_points"`
}

// 戦闘の出来事。resolveBattle が版より先に作り、recorder が版を入れる（戦闘を状態から切り離して、入力と乱数の元だけで決めるため）
type battleEvent interface {
	Event
	withMeta(meta EventMeta) Event
}

func (e BattleStartedEvent) withMeta(meta EventMeta) Event {
	e.EventMeta = meta
	return &e
}

func (e BattleSupportEffectActivatedEvent) withMeta(meta EventMeta) Event {
	e.EventMeta = meta
	return &e
}

func (e BattleTerrainEffectActivatedEvent) withMeta(meta EventMeta) Event {
	e.EventMeta = meta
	return &e
}

func (e BattleItemEffectActivatedEvent) withMeta(meta EventMeta) Event {
	e.EventMeta = meta
	return &e
}

func (e BattleCreatureEffectActivatedEvent) withMeta(meta EventMeta) Event {
	e.EventMeta = meta
	return &e
}

func (e CreatureStruckEvent) withMeta(meta EventMeta) Event {
	e.EventMeta = meta
	return &e
}

// 侵略に成功した（守る側は直前の DefendingCreatureDestroyedEvent で破壊されている）。攻めたクリーチャーが空いた土地に入って領地にし、土地のレベルは引き継ぐ。
// Creature は領地に入ったクリーチャーの ID（手札から侵略したときは新しい ID）、TerritoryHitPoints はその体力
type InvasionSucceededEvent struct {
	EventMeta          `json:"-"`
	eventMarker        `json:"-"`
	Participant        ParticipantIndex   `json:"participant"`
	Creature           PlacedCreatureID   `json:"creature"`
	TerritoryHitPoints creature.HitPoints `json:"territory_hit_points"`
}

// 侵略に失敗した（守る側を倒せなかった）。攻めたクリーチャーは生きていれば手札に戻り（移動で侵略したときは元の領地に戻り）、倒れていれば
// 直前の InvadingCreatureDestroyedEvent で捨て札に入っている。続けて通行料を払う。TerritoryHitPoints は領地のクリーチャーに残る体力
type InvasionFailedEvent struct {
	EventMeta          `json:"-"`
	eventMarker        `json:"-"`
	Participant        ParticipantIndex   `json:"participant"`
	TerritoryHitPoints creature.HitPoints `json:"territory_hit_points"`
	// 移動で侵略して生き残ったときだけ持つ。元の領地に戻ったクリーチャーの体力（戻る位置は侵略の元の領地から決まる）
	ReturnedHitPoints *creature.HitPoints `json:"returned_hit_points,omitempty"`
}

// 通った自分の領地の土地のレベルを上げた（Level は上げた後のレベル、Magic は払った魔力）
type LandLeveledUpEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Position    board.Position   `json:"position"`
	Level       int              `json:"level"`
	Magic       int              `json:"magic"`
}

// 通った自分の領地のクリーチャーを手札のクリーチャーと入れ替えた。Creature は領地に入ったクリーチャーの新しい ID で、体力はカードの値。元のクリーチャーのカードは手札の最後に戻る。Cost は払った魔力
type CreatureExchangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Position    board.Position   `json:"position"`
	HandIndex   int              `json:"hand_index"`
	Creature    PlacedCreatureID `json:"creature"`
	Cost        card.Cost        `json:"cost"`
}

// 通った自分の領地の土地の属性を変えた（Element は変えた後の属性、Magic は払った魔力 = 土地の基本の値 × 変える前の属性で決まる倍率）。土地のレベルとクリーチャーはそのまま
type LandElementChangedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	Position    board.Position   `json:"position"`
	Element     element.Land     `json:"element"`
	Magic       int              `json:"magic"`
}

// 通った自分の領地のクリーチャーを空き地へ動かした。行き先が領地になり、元の土地は空き地になる（土地のレベルはどちらも土地に残る）
type CreatureMovedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	From        board.Position   `json:"from"`
	To          board.Position   `json:"to"`
}

// 通った自分の領地のクリーチャーを他人の領地へ動かし、侵略した（続けてアイテムを選び、戦闘する）。Position は攻める領地
type TerritoryInvadedByMoveEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	From        board.Position   `json:"from"`
	Position    board.Position   `json:"position"`
}

// 振った目
type DiceRolledEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	// 出た目の合計（進む歩数）
	Value int `json:"value"`
	// 複数のサイコロを同時に振ったときの 1 個ずつの目（振った順。合計が Value）。1 個のときは持たない
	// （目を固定する効果で決まったかは、直前の状態の続いているスペルの効果から引けるので持たない。core#173）
	Dice []int `json:"dice,omitempty"`
}

// 1 歩進んで入ったマス
type PieceMovedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
	To          board.Position   `json:"to"`
}

// 今いるマスの、まだ印の無いチェックポイントに印を付け、魔力が増えた（Magic は足した量）
type CheckpointPassedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
	Participant ParticipantIndex `json:"participant"`
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
type AllRewardsReceivedEvent struct {
	EventMeta   `json:"-"`
	eventMarker `json:"-"`
}

var (
	createdEventType                       = domain.NewEventName("CreatedEvent")
	turnOrderDecidedEventType              = domain.NewEventName("TurnOrderDecidedEvent")
	drawPilesShuffledEventType             = domain.NewEventName("DrawPilesShuffledEvent")
	handsDealtEventType                    = domain.NewEventName("HandsDealtEvent")
	cardDrawnEventType                     = domain.NewEventName("CardDrawnEvent")
	cardDiscardedEventType                 = domain.NewEventName("CardDiscardedEvent")
	diceRolledEventType                    = domain.NewEventName("DiceRolledEvent")
	pieceMovedEventType                    = domain.NewEventName("PieceMovedEvent")
	checkpointPassedEventType              = domain.NewEventName("CheckpointPassedEvent")
	lapCompletedEventType                  = domain.NewEventName("LapCompletedEvent")
	turnEndedEventType                     = domain.NewEventName("TurnEndedEvent")
	matchEndedEventType                    = domain.NewEventName("MatchEndedEvent")
	rewardDrawnEventType                   = domain.NewEventName("RewardDrawnEvent")
	allRewardsReceivedEventType            = domain.NewEventName("AllRewardsReceivedEvent")
	creaturePlacedEventType                = domain.NewEventName("CreaturePlacedEvent")
	tollPaidEventType                      = domain.NewEventName("TollPaidEvent")
	territoryRelinquishedEventType         = domain.NewEventName("TerritoryRelinquishedEvent")
	magicDepletedEventType                 = domain.NewEventName("MagicDepletedEvent")
	territoryInvadedEventType              = domain.NewEventName("TerritoryInvadedEvent")
	itemUsedEventType                      = domain.NewEventName("ItemUsedEvent")
	creatureStruckEventType                = domain.NewEventName("CreatureStruckEvent")
	invasionSucceededEventType             = domain.NewEventName("InvasionSucceededEvent")
	invasionFailedEventType                = domain.NewEventName("InvasionFailedEvent")
	itemSkippedEventType                   = domain.NewEventName("ItemSkippedEvent")
	battleStartedEventType                 = domain.NewEventName("BattleStartedEvent")
	battleSupportEffectActivatedEventType  = domain.NewEventName("BattleSupportEffectActivatedEvent")
	battleTerrainEffectActivatedEventType  = domain.NewEventName("BattleTerrainEffectActivatedEvent")
	battleCreatureEffectActivatedEventType = domain.NewEventName("BattleCreatureEffectActivatedEvent")
	battleItemEffectActivatedEventType     = domain.NewEventName("BattleItemEffectActivatedEvent")
	tollCreatureEffectActivatedEventType   = domain.NewEventName("TollCreatureEffectActivatedEvent")
	landLeveledUpEventType                 = domain.NewEventName("LandLeveledUpEvent")
	creatureExchangedEventType             = domain.NewEventName("CreatureExchangedEvent")
	landElementChangedEventType            = domain.NewEventName("LandElementChangedEvent")
	creatureMovedEventType                 = domain.NewEventName("CreatureMovedEvent")
	territoryInvadedByMoveEventType        = domain.NewEventName("TerritoryInvadedByMoveEvent")
	discardPileShuffledEventType           = domain.NewEventName("DiscardPileShuffledEvent")
	handLimitExceededEventType             = domain.NewEventName("HandLimitExceededEvent")
	spellUsedEventType                     = domain.NewEventName("SpellUsedEvent")
	magicGainedEventType                   = domain.NewEventName("MagicGainedEvent")
	spellEffectStartedEventType            = domain.NewEventName("SpellEffectStartedEvent")
	spellEffectEndedEventType              = domain.NewEventName("SpellEffectEndedEvent")
	creatureDamagedEventType               = domain.NewEventName("CreatureDamagedEvent")
	boardCreatureDestroyedEventType        = domain.NewEventName("BoardCreatureDestroyedEvent")
	invadingCreatureDestroyedEventType     = domain.NewEventName("InvadingCreatureDestroyedEvent")
	defendingCreatureDestroyedEventType    = domain.NewEventName("DefendingCreatureDestroyedEvent")
)

func (*CreatedEvent) EventType() domain.EventName {
	return createdEventType
}

func (*TurnOrderDecidedEvent) EventType() domain.EventName {
	return turnOrderDecidedEventType
}

func (*DrawPilesShuffledEvent) EventType() domain.EventName {
	return drawPilesShuffledEventType
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

func (*DrawPilesShuffledEvent) SchemaVersion() int {
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

func (*PieceMovedEvent) EventType() domain.EventName {
	return pieceMovedEventType
}

func (*PieceMovedEvent) SchemaVersion() int {
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

func (*AllRewardsReceivedEvent) EventType() domain.EventName {
	return allRewardsReceivedEventType
}

func (*AllRewardsReceivedEvent) SchemaVersion() int {
	return 1
}

func (*CreaturePlacedEvent) EventType() domain.EventName {
	return creaturePlacedEventType
}

func (*CreaturePlacedEvent) SchemaVersion() int {
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

func (*InvasionSucceededEvent) EventType() domain.EventName {
	return invasionSucceededEventType
}

func (*InvasionSucceededEvent) SchemaVersion() int {
	return 1
}

func (*InvasionFailedEvent) EventType() domain.EventName {
	return invasionFailedEventType
}

func (*InvasionFailedEvent) SchemaVersion() int {
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

func (*BattleSupportEffectActivatedEvent) EventType() domain.EventName {
	return battleSupportEffectActivatedEventType
}

func (*BattleSupportEffectActivatedEvent) SchemaVersion() int {
	return 1
}

func (*BattleTerrainEffectActivatedEvent) EventType() domain.EventName {
	return battleTerrainEffectActivatedEventType
}

func (*BattleTerrainEffectActivatedEvent) SchemaVersion() int {
	return 1
}

func (*BattleCreatureEffectActivatedEvent) EventType() domain.EventName {
	return battleCreatureEffectActivatedEventType
}

func (*BattleCreatureEffectActivatedEvent) SchemaVersion() int {
	return 1
}

func (*BattleItemEffectActivatedEvent) EventType() domain.EventName {
	return battleItemEffectActivatedEventType
}

func (*BattleItemEffectActivatedEvent) SchemaVersion() int {
	return 1
}

func (*TollCreatureEffectActivatedEvent) EventType() domain.EventName {
	return tollCreatureEffectActivatedEventType
}

func (*TollCreatureEffectActivatedEvent) SchemaVersion() int {
	return 1
}

func (*LandLeveledUpEvent) EventType() domain.EventName {
	return landLeveledUpEventType
}

func (*LandLeveledUpEvent) SchemaVersion() int {
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

func (*DiscardPileShuffledEvent) EventType() domain.EventName {
	return discardPileShuffledEventType
}

func (*DiscardPileShuffledEvent) SchemaVersion() int {
	return 1
}

func (*HandLimitExceededEvent) EventType() domain.EventName {
	return handLimitExceededEventType
}

func (*HandLimitExceededEvent) SchemaVersion() int {
	return 1
}

func (*SpellUsedEvent) EventType() domain.EventName {
	return spellUsedEventType
}

func (*SpellUsedEvent) SchemaVersion() int {
	return 1
}

func (*MagicGainedEvent) EventType() domain.EventName {
	return magicGainedEventType
}

func (*MagicGainedEvent) SchemaVersion() int {
	return 1
}

func (*SpellEffectStartedEvent) EventType() domain.EventName {
	return spellEffectStartedEventType
}

func (*SpellEffectStartedEvent) SchemaVersion() int {
	return 1
}

func (*SpellEffectEndedEvent) EventType() domain.EventName {
	return spellEffectEndedEventType
}

func (*SpellEffectEndedEvent) SchemaVersion() int {
	return 1
}

func (*CreatureDamagedEvent) EventType() domain.EventName {
	return creatureDamagedEventType
}

func (*CreatureDamagedEvent) SchemaVersion() int {
	return 1
}

func (*BoardCreatureDestroyedEvent) EventType() domain.EventName {
	return boardCreatureDestroyedEventType
}

func (*BoardCreatureDestroyedEvent) SchemaVersion() int {
	return 1
}

func (*InvadingCreatureDestroyedEvent) EventType() domain.EventName {
	return invadingCreatureDestroyedEventType
}

func (*InvadingCreatureDestroyedEvent) SchemaVersion() int {
	return 1
}

func (*DefendingCreatureDestroyedEvent) EventType() domain.EventName {
	return defendingCreatureDestroyedEventType
}

func (*DefendingCreatureDestroyedEvent) SchemaVersion() int {
	return 1
}

func Types() domain.AggregateTypes {
	return domain.AggregateTypes{EntityType: entityType, EventTypes: []domain.EventName{
		createdEventType, turnOrderDecidedEventType, drawPilesShuffledEventType, handsDealtEventType,
		cardDrawnEventType, cardDiscardedEventType, diceRolledEventType, pieceMovedEventType, checkpointPassedEventType, lapCompletedEventType, turnEndedEventType, matchEndedEventType,
		rewardDrawnEventType, allRewardsReceivedEventType, creaturePlacedEventType, tollPaidEventType, territoryRelinquishedEventType, magicDepletedEventType,
		territoryInvadedEventType, itemUsedEventType, creatureStruckEventType, invasionSucceededEventType, invasionFailedEventType, itemSkippedEventType,
		battleStartedEventType, battleSupportEffectActivatedEventType, battleTerrainEffectActivatedEventType,
		battleCreatureEffectActivatedEventType, battleItemEffectActivatedEventType, landLeveledUpEventType, landElementChangedEventType, creatureExchangedEventType,
		creatureMovedEventType, territoryInvadedByMoveEventType, discardPileShuffledEventType, handLimitExceededEventType,
		spellUsedEventType, magicGainedEventType, spellEffectStartedEventType, spellEffectEndedEventType,
		creatureDamagedEventType, boardCreatureDestroyedEventType, tollCreatureEffectActivatedEventType,
		invadingCreatureDestroyedEventType, defendingCreatureDestroyedEventType,
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
	case EventType(drawPilesShuffledEventType):
		event = &DrawPilesShuffledEvent{EventMeta: meta}
	case EventType(handsDealtEventType):
		event = &HandsDealtEvent{EventMeta: meta}
	case EventType(cardDrawnEventType):
		event = &CardDrawnEvent{EventMeta: meta}
	case EventType(cardDiscardedEventType):
		event = &CardDiscardedEvent{EventMeta: meta}
	case EventType(diceRolledEventType):
		event = &DiceRolledEvent{EventMeta: meta}
	case EventType(pieceMovedEventType):
		event = &PieceMovedEvent{EventMeta: meta}
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
	case EventType(allRewardsReceivedEventType):
		event = &AllRewardsReceivedEvent{EventMeta: meta}
	case EventType(creaturePlacedEventType):
		event = &CreaturePlacedEvent{EventMeta: meta}
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
	case EventType(invasionSucceededEventType):
		event = &InvasionSucceededEvent{EventMeta: meta}
	case EventType(invasionFailedEventType):
		event = &InvasionFailedEvent{EventMeta: meta}
	case EventType(itemSkippedEventType):
		event = &ItemSkippedEvent{EventMeta: meta}
	case EventType(battleStartedEventType):
		event = &BattleStartedEvent{EventMeta: meta}
	case EventType(battleSupportEffectActivatedEventType):
		event = &BattleSupportEffectActivatedEvent{EventMeta: meta}
	case EventType(battleTerrainEffectActivatedEventType):
		event = &BattleTerrainEffectActivatedEvent{EventMeta: meta}
	case EventType(battleCreatureEffectActivatedEventType):
		event = &BattleCreatureEffectActivatedEvent{EventMeta: meta}
	case EventType(battleItemEffectActivatedEventType):
		event = &BattleItemEffectActivatedEvent{EventMeta: meta}
	case EventType(tollCreatureEffectActivatedEventType):
		event = &TollCreatureEffectActivatedEvent{EventMeta: meta}
	case EventType(landLeveledUpEventType):
		event = &LandLeveledUpEvent{EventMeta: meta}
	case EventType(creatureExchangedEventType):
		event = &CreatureExchangedEvent{EventMeta: meta}
	case EventType(landElementChangedEventType):
		event = &LandElementChangedEvent{EventMeta: meta}
	case EventType(creatureMovedEventType):
		event = &CreatureMovedEvent{EventMeta: meta}
	case EventType(territoryInvadedByMoveEventType):
		event = &TerritoryInvadedByMoveEvent{EventMeta: meta}
	case EventType(discardPileShuffledEventType):
		event = &DiscardPileShuffledEvent{EventMeta: meta}
	case EventType(handLimitExceededEventType):
		event = &HandLimitExceededEvent{EventMeta: meta}
	case EventType(spellUsedEventType):
		event = &SpellUsedEvent{EventMeta: meta}
	case EventType(magicGainedEventType):
		event = &MagicGainedEvent{EventMeta: meta}
	case EventType(spellEffectStartedEventType):
		event = &SpellEffectStartedEvent{EventMeta: meta}
	case EventType(spellEffectEndedEventType):
		event = &SpellEffectEndedEvent{EventMeta: meta}
	case EventType(creatureDamagedEventType):
		event = &CreatureDamagedEvent{EventMeta: meta}
	case EventType(boardCreatureDestroyedEventType):
		event = &BoardCreatureDestroyedEvent{EventMeta: meta}
	case EventType(invadingCreatureDestroyedEventType):
		event = &InvadingCreatureDestroyedEvent{EventMeta: meta}
	case EventType(defendingCreatureDestroyedEventType):
		event = &DefendingCreatureDestroyedEvent{EventMeta: meta}
	default:
		return nil, domain.ErrUnknownEventType
	}
	if err := json.Unmarshal(payload, event); err != nil {
		return nil, err
	}
	return event, nil
}
