package match

import (
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type State interface {
	isMatchState()
}

type UncreatedMatch struct{}

func (UncreatedMatch) isMatchState() {}

// 参加者 1 人の今の状態
type ParticipantState struct {
	Participant Participant
	Position    board.Position
	Magic       int
	// 先頭から引く
	DrawPile []card.Ref
	Hand     []card.Ref
	// 捨て札（入った順）。山札が空のまま引くときに混ぜて山札にする
	DiscardPile []card.Ref
	// 直前にいたマス。向き無しのつながりで戻らないための記憶で、次のターンの最初の一歩でも使うのでターンを終えても残す。最初の一歩の前は無い
	Previous *board.Position
	// 拠点を出てから印を付けたチェックポイント
	PassedCheckpoints []board.Position
	// この対戦で周回を終えた回数（魔力を得るスペルが使う）
	Laps int
	// 続いているスペルの効果（1 つまで。新しく使うと上書きする）。無ければ nil
	LastingSpellEffect *LastingSpellEffect
	// 受け取った戦利品。終わった後にパックを選ぶまでは nil（NPC は受け取らない）
	Reward *Reward
}

// 受け取った戦利品
type Reward struct {
	Pack  PackKey
	Cards []card.Ref
}

// 盤面に配置したクリーチャー 1 体（2026-11-08 オーナー。core#113）。カードを指すだけにしないのは、戦闘の傷やスペルの効果を、
// この 1 体の今の状態として持つため。配置・侵略で奪う・交換・移動のどれから来ても同じ実体で、イベントは ID で指す
type PlacedCreature struct {
	ID    PlacedCreatureID
	Card  card.Ref
	Owner ParticipantIndex
	// 配置したときの性能の写し（対戦を始めたときの写しから写す。この 1 体だけを変える効果がほかの同じカードに響かないように）
	Base CreatureSpec
	// 今の AT（ボード上のスペル・能力で増減する。今は増減する効果が無いので性能の AT のまま。2026-11-14 オーナー。core#175）
	Attack creature.Attack
	// 今の体力
	HitPoints creature.HitPoints
}

// 領地。持ち主は、そこにいるクリーチャーの持ち主
type Territory struct {
	Position board.Position
	Creature PlacedCreatureID
}

// 土地のレベル。レベルは領地ではなく土地のもので、クリーチャーがいなくなっても（手放しても、倒されても、動いても）土地に残り、
// 次に配置・移動したクリーチャーが引き継ぐ（2026-11-10 オーナー。core#146）
type LandLevel struct {
	Position board.Position
	Level    int
}

// 戦闘が終わるまでの、侵略しているクリーチャー。領地（1 マスに 1 体）には倒してから入るので、盤面の実体とは別に持つ
type Invasion struct {
	Card card.Ref
	// 攻められている領地の位置（手札から侵略したときは止まったマス、移動で侵略したときは移動先）
	Position board.Position
	// 移動で侵略したときだけ持つ。元の領地の位置と、動いたクリーチャー（戦闘の間も元の領地にいる）
	From     *board.Position
	Creature PlacedCreatureID
	// 出したときの性能の写し
	Base CreatureSpec
	// 戦闘の前の AT・体力（戦闘の中の値は Battle が持つ）
	Attack    creature.Attack
	HitPoints creature.HitPoints
	// 出したアイテム。出すまでと、出さなかったときは nil
	AttackerItem *card.Ref
	DefenderItem *card.Ref
}

// 戦闘の中の片方のクリーチャー。値を層に分けて持ち、今の AT・HP（Stats）は層から計算する。今の値 1 つに混ぜると、
// どの効果でいくら増えたか・受けたダメージがいくつかが残らず、地形効果の分だけを除く（貫通）・基本の値を見る（制約）といった
// 計算ができないため（2026-11-14 オーナー。core#175）
type BattleCreature struct {
	// 基本の AT・HP（カードの性能の値）
	Base BattleStats
	// 戦闘の前の AT・体力（領地のクリーチャーは、ボード上の効果と傷が残った今の値。手札から侵略したクリーチャーは基本の値と同じ）
	InitialAttack    creature.Attack
	InitialHitPoints creature.HitPoints
	// 効果ごとの増減（当てた順。下限で止まった後の実際の量）
	Modifiers []BattleStatModifier
	// 攻撃で受けたダメージの合計（下限で止まった後の実際の量）
	Damage int
	// 能力（先制・後手）の発動で得た攻撃の順の強さ。先制は 1、後手は -1、どちらも発動していなければ 0。大きい方が先に攻撃する
	StrikePriority int
	// 攻撃で受けるダメージの割合（百分率）。ふつうは 100、攻撃無効の発動で 0。無効か否かの真偽にしないのは、攻撃半減のような
	// 割合で減らす能力を同じ形で持つため（2026-11-14 オーナー。core#175）。複数が発動したら一番小さい割合を採る（倍率の
	// 「一番高いものを 1 回だけ」と同じ考え方）
	ReceivedDamagePercent int
}

// 効果 1 つで増減した量と、その出どころ
type BattleStatModifier struct {
	Source         BattleModifierSource
	AttackDelta    int
	HitPointsDelta int
}

// 今の AT・HP。AT は戦闘の前の値に増減を、HP は戦闘の前の値に増減と受けたダメージを当てる。増減とダメージは下限で止まった後の
// 実際の量なので、足し合わせても途中で 0 を下回らない
func (c BattleCreature) Stats() BattleStats {
	attackDelta, hitPointsDelta := 0, 0
	for _, modifier := range c.Modifiers {
		attackDelta += modifier.AttackDelta
		hitPointsDelta += modifier.HitPointsDelta
	}
	return BattleStats{Attack: c.InitialAttack.Modified(attackDelta), HitPoints: c.InitialHitPoints.Modified(hitPointsDelta - c.Damage)}
}

func (c BattleCreature) hitPointsDelta() int {
	delta := 0
	for _, modifier := range c.Modifiers {
		delta += modifier.HitPointsDelta
	}
	return delta
}

// 戦闘の後に領地に残る体力。増えた分は先に減って残りは消え、下がった分は戦闘が終わると戻るので、残る傷は増えた分を超えたダメージだけ
func (c BattleCreature) territoryHitPoints() creature.HitPoints {
	return c.InitialHitPoints.Modified(-max(c.Damage-max(c.hitPointsDelta(), 0), 0))
}

// 戦闘が始まってから終わるまでの両方のクリーチャー
type Battle struct {
	Attacker BattleCreature
	Defender BattleCreature
}

// その側のクリーチャーの戦闘の中の値
func (b Battle) Creature(side BattleSide) BattleCreature {
	if side == BattleSideAttacker {
		return b.Attacker
	}
	return b.Defender
}

func (b Battle) withCreature(side BattleSide, updated BattleCreature) *Battle {
	if side == BattleSideAttacker {
		b.Attacker = updated
	} else {
		b.Defender = updated
	}
	return &b
}

func (b Battle) withModifier(side BattleSide, source BattleModifierSource, change BattleStatChange) *Battle {
	modified := b.Creature(side)
	modified.Modifiers = append(slices.Clone(modified.Modifiers), BattleStatModifier{Source: source, AttackDelta: change.AttackDelta, HitPointsDelta: change.HitPointsDelta})
	return b.withCreature(side, modified)
}

// どちらかの体力が 0 になっていて、戦闘が終わるか。AT は 0 になっても終わらない
func (b Battle) decided() bool {
	return b.Attacker.Stats().HitPoints.Int() == 0 || b.Defender.Stats().HitPoints.Int() == 0
}

// 対戦の今の状態。read model を作らず、query もこの状態を読み直して返す（組み立ての決まりをここ 1 か所にするため）
type Match struct {
	ID           ID
	Version      int64
	Board        Board
	Rules        Rules
	Participants []ParticipantState
	// 順番。決まるまでは空
	TurnOrder []ParticipantIndex
	// 今のラウンド（1 始まり）
	Round int
	// 今の番が、順番の何番目か（0 始まり）
	Turn  int
	Phase Phase
	// このターンに残っている歩数
	RemainingSteps int
	// このターンに振った目。振る前は 0
	Dice int
	// 終わるまでは nil
	Result *Result
	// 戦利品を、プレイヤー全員が受け取った。終わっても段階は ended のままで、これで受け取りまで済んだことを表す
	RewardsCompleted bool
	// 盤面にいるクリーチャー（配置した順）
	PlacedCreatures []PlacedCreature
	// 領地（できた順）
	Territories []Territory
	// レベルを上げた土地（上げた順）。無い土地はレベル 1
	LandLevels []LandLevel
	// 侵略してから戦闘が終わるまでだけある
	Invasion *Invasion
	// 戦闘が始まってから終わるまでだけある
	Battle *Battle
	// このターンに移動で入ったマス（入った順。止まったマスを含み、ターンの始めのマスは含まない）
	PassedPositions []board.Position
	// このターンにスペルを使ったか（1 ターンに 1 枚まで。ターンを終えると消える）
	SpellUsed bool
	// 最後に振った配置したクリーチャーの ID。盤面から外れたクリーチャーの ID を使い回さないため、数を数え直さずにここで持つ
	LastPlacedCreatureID PlacedCreatureID
}

func (Match) isMatchState() {}

func InitialState() State {
	return UncreatedMatch{}
}

// 対戦に参加しているプレイヤーか。NPC は数えない
func (m Match) HasPlayer(playerID player.ID) bool {
	for _, participant := range m.Participants {
		if participant.Participant.Player != nil && participant.Participant.Player.Player == playerID {
			return true
		}
	}
	return false
}

// 今の番の参加者。順番が決まるまでは無い
func (m Match) CurrentParticipant() (ParticipantIndex, bool) {
	if m.Turn >= len(m.TurnOrder) {
		return 0, false
	}
	return m.TurnOrder[m.Turn], true
}

// 選べる行き先（Position.Compare の順）。つながりは向きが無く、来たマスには戻らない。行き止まりでだけ来たマスへ戻る
func (m Match) PathChoices() []board.Position {
	index, ok := m.CurrentParticipant()
	if !ok || int(index) >= len(m.Participants) {
		return nil
	}
	participant := m.Participants[index]
	var neighbors []board.Position
	for _, connection := range m.Board.Connections {
		switch participant.Position {
		case connection.A:
			neighbors = append(neighbors, connection.B)
		case connection.B:
			neighbors = append(neighbors, connection.A)
		}
	}
	slices.SortFunc(neighbors, board.Position.Compare)
	neighbors = slices.Compact(neighbors)
	if participant.Previous == nil {
		return neighbors
	}
	var choices []board.Position
	for _, neighbor := range neighbors {
		if neighbor != *participant.Previous {
			choices = append(choices, neighbor)
		}
	}
	if len(choices) == 0 && slices.Contains(neighbors, *participant.Previous) {
		return []board.Position{*participant.Previous}
	}
	return choices
}

// 終わった後に、この参加者がまだパックを選べるか（受け取っていないプレイヤー）
func (m Match) CanChooseRewardPack(index ParticipantIndex) bool {
	if m.Phase != PhaseEnded || int(index) >= len(m.Participants) {
		return false
	}
	participant := m.Participants[index]
	return participant.Participant.Player != nil && participant.Reward == nil
}

func (m Match) allPlayersRewarded() bool {
	for _, participant := range m.Participants {
		if participant.Participant.Player != nil && participant.Reward == nil {
			return false
		}
	}
	return true
}

// 参加者のデッキに写したカードの性能（手札・山札のカードはどれかの参加者のデッキにある）。同じカードを複数の参加者が持てば、
// 同じ時点の写しなので同じ値
func (m Match) CardSpecs() map[card.Ref]CardSpec {
	specs := map[card.Ref]CardSpec{}
	for _, participant := range m.Participants {
		for _, entry := range participant.Participant.Cards {
			specs[entry.Card] = entry.Spec
		}
	}
	return specs
}

// その位置の領地
func (m Match) TerritoryAt(position board.Position) (Territory, bool) {
	for _, territory := range m.Territories {
		if territory.Position == position {
			return territory, true
		}
	}
	return Territory{}, false
}

// ID で指したクリーチャー
func (m Match) PlacedCreature(id PlacedCreatureID) (PlacedCreature, bool) {
	for _, placed := range m.PlacedCreatures {
		if placed.ID == id {
			return placed, true
		}
	}
	return PlacedCreature{}, false
}

// 今の番の人が配置できる手札の番号（手札の順）。配置の段階でだけ中身がある。クリーチャーで、コストが今の魔力以下のもの
func (m Match) PlaceChoices() []int {
	if m.Phase != PhasePlacing {
		return nil
	}
	return m.placeableCreaturesAtStop()
}

// 今の番の人が侵略に出せる手札の番号（手札の順）。侵略を選ぶ段階でだけ中身がある。配置と同じ決まりに、侵略できないクリーチャーを外す決まりを足したもの
func (m Match) InvadeChoices() []int {
	if m.Phase != PhaseInvading {
		return nil
	}
	index, ok := m.CurrentParticipant()
	if !ok || int(index) >= len(m.Participants) {
		return nil
	}
	hand := m.Participants[index].Hand
	specs := m.CardSpecs()
	var choices []int
	for _, handIndex := range m.placeableCreaturesAtStop() {
		if specs[hand[handIndex]].Creature.canInvade() {
			choices = append(choices, handIndex)
		}
	}
	return choices
}

// 使えるスペル 1 枚。Targets は選べる対象のプレイヤー（参加者の順）で、対象を選ばないスペルは空
type SpellChoice struct {
	HandIndex int
	Targets   []ParticipantIndex
	// 選べる対象のクリーチャーがいる領地の位置（クリーチャーを選ぶスペルだけ。自分の領地を含む。領地ができた順）
	TargetPositions []board.Position
}

// 今の番の人が使えるスペル（手札の順）。サイコロを振る段階で、このターンにまだ使っていないときだけ中身がある。
// 効果の無いスペルは使えない（2026-11-10 オーナー）。対象のプレイヤーは自分を含めて誰でも選べる
func (m Match) SpellChoices() []SpellChoice {
	if m.Phase != PhaseRolling || m.SpellUsed {
		return nil
	}
	index, ok := m.CurrentParticipant()
	if !ok || int(index) >= len(m.Participants) {
		return nil
	}
	participant := m.Participants[index]
	specs := m.CardSpecs()
	var choices []SpellChoice
	for i, ref := range participant.Hand {
		spec, ok := specs[ref]
		if !ok || spec.Spell == nil || spec.Spell.Effect.IsNone() || spec.Cost.Int() > participant.Magic {
			continue
		}
		choice := SpellChoice{HandIndex: i, Targets: []ParticipantIndex{}, TargetPositions: []board.Position{}}
		if spec.Spell.Effect.TargetsPlayer() {
			for target := range m.Participants {
				choice.Targets = append(choice.Targets, ParticipantIndex(target))
			}
		}
		// 自分の領地のクリーチャーも選べる（2026-11-11 オーナー）
		if spec.Spell.Effect.TargetsCreature() {
			for _, territory := range m.Territories {
				choice.TargetPositions = append(choice.TargetPositions, territory.Position)
			}
			// 対象がいなければ使えない（使っても何も起きないため）
			if len(choice.TargetPositions) == 0 {
				continue
			}
		}
		choices = append(choices, choice)
	}
	return choices
}

// 止まったマスに出せる手札（配置・侵略）
func (m Match) placeableCreaturesAtStop() []int {
	index, ok := m.CurrentParticipant()
	if !ok || int(index) >= len(m.Participants) {
		return nil
	}
	return m.creaturesPlaceableOn(m.Participants[index].Position)
}

// その土地で活動できる、魔力の足りる手札（core#155）
func (m Match) creaturesPlaceableOn(position board.Position) []int {
	index, ok := m.CurrentParticipant()
	if !ok || int(index) >= len(m.Participants) {
		return nil
	}
	element := m.landElement(position)
	hand := m.Participants[index].Hand
	specs := m.CardSpecs()
	var choices []int
	for _, handIndex := range m.payableCreatures() {
		if specs[hand[handIndex]].Creature.canBeActiveOn(element) {
			choices = append(choices, handIndex)
		}
	}
	return choices
}

func (m Match) payableCreatures() []int {
	index, ok := m.CurrentParticipant()
	if !ok || int(index) >= len(m.Participants) {
		return nil
	}
	participant := m.Participants[index]
	specs := m.CardSpecs()
	var choices []int
	for i, ref := range participant.Hand {
		spec, ok := specs[ref]
		if ok && spec.Creature != nil && spec.Cost.Int() <= participant.Magic {
			choices = append(choices, i)
		}
	}
	return choices
}

// 今選んでいる人が戦闘に出せるアイテムの手札の番号（手札の順）。攻める段階と守る段階でだけ中身がある
func (m Match) ItemChoices() []int {
	if m.Phase != PhaseAttacking && m.Phase != PhaseDefending {
		return nil
	}
	index, ok := m.AwaitingParticipant()
	if !ok || int(index) >= len(m.Participants) {
		return nil
	}
	fighter, ok := m.awaitingFighter()
	if !ok {
		return nil
	}
	participant := m.Participants[index]
	specs := m.CardSpecs()
	var choices []int
	for i, ref := range participant.Hand {
		spec, ok := specs[ref]
		if ok && ref.EntityType == card.ItemEntityType && spec.Cost.Int() <= participant.Magic && fighter.canUseItem(spec) {
			choices = append(choices, i)
		}
	}
	return choices
}

// アイテムの選びを待っている側の、戦うクリーチャーの性能。攻める段階は侵略しているクリーチャー、守る段階は領地のクリーチャー
func (m Match) awaitingFighter() (CreatureSpec, bool) {
	if m.Phase == PhaseAttacking {
		if m.Invasion == nil {
			return CreatureSpec{}, false
		}
		return m.Invasion.Base, true
	}
	territory, ok := m.battleTerritory()
	if !ok {
		return CreatureSpec{}, false
	}
	placed, ok := m.PlacedCreature(territory.Creature)
	return placed.Base, ok
}

// 今操作を待っている参加者。守る段階だけは領地の持ち主で、ほかは今の番の人
func (m Match) AwaitingParticipant() (ParticipantIndex, bool) {
	if m.Phase == PhaseDefending {
		territory, ok := m.battleTerritory()
		if !ok {
			return 0, false
		}
		return m.TerritoryOwner(territory)
	}
	return m.CurrentParticipant()
}

// 今の番の人が止まっている領地（侵略の間は、攻められている領地）
func (m Match) invadedTerritory() (Territory, bool) {
	index, ok := m.CurrentParticipant()
	if !ok || int(index) >= len(m.Participants) {
		return Territory{}, false
	}
	return m.TerritoryAt(m.Participants[index].Position)
}

// 領地のレベルを上げる選択肢 1 つ
type LevelUpChoice struct {
	Position board.Position
	// 上げた後のレベル
	Level int
	// 払う魔力
	Magic int
}

// 止まった後に、領地コマンドを受け付ける段階か。止まったマスの行動と同じ選択肢に並ぶ（2026-11-09 オーナー）
func (m Match) acceptsTerritoryCommand() bool {
	return m.Phase == PhasePlacing || m.Phase == PhaseInvading || m.Phase == PhaseTerritoryCommanding
}

// 今の番の人が選べるレベルアップ（通った順・低いレベルの順）。止まった後の段階でだけ中身がある。手持ちの魔力で払える分だけ
func (m Match) LevelUpChoices() []LevelUpChoice {
	index, ok := m.CurrentParticipant()
	if !m.acceptsTerritoryCommand() || !ok || int(index) >= len(m.Participants) {
		return nil
	}
	var choices []LevelUpChoice
	for _, territory := range m.passedOwnTerritories() {
		for level := m.LandLevel(territory.Position) + 1; level <= len(m.Rules.LandLevelMultipliers); level++ {
			if magic := m.levelUpMagic(territory, level); magic <= m.Participants[index].Magic {
				choices = append(choices, LevelUpChoice{Position: territory.Position, Level: level, Magic: magic})
			}
		}
	}
	return choices
}

// クリーチャーを交換する選択肢 1 つ
type ExchangeChoice struct {
	Position board.Position
	// 手札の番号（0 始まり）
	HandIndex int
	// 払う魔力（出すクリーチャーのコスト）
	Magic int
}

// 今の番の人が選べる交換（通った順・手札の順）。止まった後の段階でだけ中身がある。手札のクリーチャーで、手持ちの魔力で払える分だけ
func (m Match) ExchangeChoices() []ExchangeChoice {
	index, ok := m.CurrentParticipant()
	if !m.acceptsTerritoryCommand() || !ok || int(index) >= len(m.Participants) {
		return nil
	}
	specs := m.CardSpecs()
	var choices []ExchangeChoice
	for _, territory := range m.passedOwnTerritories() {
		for _, handIndex := range m.creaturesPlaceableOn(territory.Position) {
			choices = append(choices, ExchangeChoice{Position: territory.Position, HandIndex: handIndex, Magic: specs[m.Participants[index].Hand[handIndex]].Cost.Int()})
		}
	}
	return choices
}

// 土地の属性を変える選択肢 1 つ
type ElementChangeChoice struct {
	Position board.Position
	// 変えた後の属性
	Element board.Element
	// 払う魔力（基本の値 × Multiplier）
	Magic int
	// 掛けた倍率。複属性の土地からは安い倍率になる。イベントは持たないので、画面向けの写し（query）が直前の状態の選択肢から根拠を引く
	Multiplier int
}

// 変えられる先の属性。無属性・複属性へは変えられない（2026-11-10 オーナー）
var changeableLandElements = []board.Element{board.ElementFire, board.ElementEarth, board.ElementWater, board.ElementWind}

// 今の番の人が選べる属性の変更（通った順・火・地・水・風の順）。止まった後の段階でだけ中身がある。今と同じ属性は選べず、手持ちの魔力で払える分だけ
func (m Match) ElementChangeChoices() []ElementChangeChoice {
	index, ok := m.CurrentParticipant()
	if !m.acceptsTerritoryCommand() || !ok || int(index) >= len(m.Participants) {
		return nil
	}
	var choices []ElementChangeChoice
	for _, territory := range m.passedOwnTerritories() {
		current := m.landElement(territory.Position)
		multiplier := m.landElementChangeMultiplier(current)
		magic := m.Rules.BaseLandValue * multiplier
		placed, ok := m.PlacedCreature(territory.Creature)
		if !ok || magic > m.Participants[index].Magic {
			continue
		}
		for _, element := range changeableLandElements {
			// いるクリーチャーが活動できない属性には変えられない（core#155）
			if element != current && placed.Base.canBeActiveOn(element) {
				choices = append(choices, ElementChangeChoice{Position: territory.Position, Element: element, Magic: magic, Multiplier: multiplier})
			}
		}
	}
	return choices
}

// 払う魔力は基本の値 × この倍率。複属性の土地からは安い倍率（2026-11-10 オーナー）
func (m Match) landElementChangeMultiplier(current board.Element) int {
	if current == board.ElementMulti {
		return m.Rules.MultiLandElementChangeMultiplier.Int()
	}
	return m.Rules.LandElementChangeMultiplier.Int()
}

// 基本の値 × 倍率の差。連鎖の倍率は掛けないので、飛ばして上げた額は途中の額の合計になる（2026-11-08 オーナー）
func (m Match) levelUpMagic(territory Territory, level int) int {
	return m.Rules.BaseLandValue * (m.Rules.landLevelMultiplier(level) - m.Rules.landLevelMultiplier(m.LandLevel(territory.Position)))
}

// 今の番の人が、このターンに通った自分の領地（入った順で重ならない）
func (m Match) passedOwnTerritories() []Territory {
	index, ok := m.CurrentParticipant()
	if !ok {
		return nil
	}
	var owned []Territory
	var seen []board.Position
	for _, position := range m.PassedPositions {
		territory, occupied := m.TerritoryAt(position)
		if !occupied || slices.Contains(seen, position) {
			continue
		}
		if owner, ok := m.TerritoryOwner(territory); ok && owner == index {
			seen = append(seen, position)
			owned = append(owned, territory)
		}
	}
	return owned
}

// 戦闘で攻められている領地。侵略の間は侵略の位置（移動で侵略したときは止まったマスと違う）、ほかは止まったマス
func (m Match) battleTerritory() (Territory, bool) {
	if m.Invasion != nil {
		return m.TerritoryAt(m.Invasion.Position)
	}
	return m.invadedTerritory()
}

// クリーチャー移動で進むマスの数。2 マス動ける能力を足すときは、ここを能力で変える形にする（性能の項目にしない。2026-11-11 オーナー。core#156）
const creatureMoveSteps = 1

// クリーチャーを動かす選択肢 1 つ
type MoveChoice struct {
	From board.Position
	To   board.Position
}

// 今の番の人が選べる移動（通った順・行き先の位置の順）。止まった後の段階でだけ中身がある。ちょうど creatureMoveSteps だけ、ボードのつながりで
// 進み（転送円・橋の対は使わない）、途中はどのマスも通り抜けられ、来たマスへは折り返さない。行き先は土地の空き地か他人の領地
// （2026-11-10 オーナー。core#147）
func (m Match) MoveChoices() []MoveChoice {
	index, ok := m.CurrentParticipant()
	if !m.acceptsTerritoryCommand() || !ok || int(index) >= len(m.Participants) {
		return nil
	}
	var choices []MoveChoice
	for _, territory := range m.passedOwnTerritories() {
		placed, ok := m.PlacedCreature(territory.Creature)
		if !ok || !placed.Base.canMove() {
			continue
		}
		var destinations []board.Position
		m.walk(territory.Position, nil, creatureMoveSteps, &destinations)
		slices.SortFunc(destinations, board.Position.Compare)
		for _, to := range slices.Compact(destinations) {
			// 他人の領地への移動は侵略でもある（今は侵略できないクリーチャーは移動もできないが、能力ごとに分けて書く）
			invades := !m.isVacantLand(to)
			if m.isMoveDestination(to, index) && placed.Base.canBeActiveOn(m.landElement(to)) && (!invades || placed.Base.canInvade()) {
				choices = append(choices, MoveChoice{From: territory.Position, To: to})
			}
		}
	}
	return choices
}

// 来たマスへ折り返さずに steps 歩進んで届くマスを集める（行き止まりで歩数が残れば届かない）
func (m Match) walk(at board.Position, previous *board.Position, steps int, reached *[]board.Position) {
	if steps == 0 {
		*reached = append(*reached, at)
		return
	}
	for _, next := range m.neighbors(at) {
		if previous != nil && next == *previous {
			continue
		}
		from := at
		m.walk(next, &from, steps-1, reached)
	}
}

func (m Match) neighbors(position board.Position) []board.Position {
	var neighbors []board.Position
	for _, connection := range m.Board.Connections {
		switch position {
		case connection.A:
			neighbors = append(neighbors, connection.B)
		case connection.B:
			neighbors = append(neighbors, connection.A)
		}
	}
	return neighbors
}

// 移動先にできるか。土地で、空き地か他人の領地（自分の領地は選べない）
func (m Match) isMoveDestination(position board.Position, index ParticipantIndex) bool {
	if m.isVacantLand(position) {
		return true
	}
	territory, ok := m.TerritoryAt(position)
	if !ok {
		return false
	}
	owner, ok := m.TerritoryOwner(territory)
	return ok && owner != index
}

// 今の番の人が手放せる領地の位置（できた順）。手放す段階でだけ中身がある
func (m Match) RelinquishChoices() []board.Position {
	index, ok := m.CurrentParticipant()
	if m.Phase != PhaseRelinquishing || !ok {
		return nil
	}
	var choices []board.Position
	for _, territory := range m.territoriesOf(index) {
		choices = append(choices, territory.Position)
	}
	return choices
}

// 手放せる領地のうち、価値が最も低いものの位置。同じ価値なら先にできた領地（2026-11-08 オーナー。NPC の選び方）
func (m Match) LowestValueRelinquishChoice() (board.Position, bool) {
	var lowest *Territory
	for _, position := range m.RelinquishChoices() {
		territory, ok := m.TerritoryAt(position)
		if ok && (lowest == nil || m.TerritoryValue(territory) < m.TerritoryValue(*lowest)) {
			lowest = &territory
		}
	}
	if lowest == nil {
		return board.Position{}, false
	}
	return lowest.Position, true
}

func (m Match) territoriesOf(index ParticipantIndex) []Territory {
	var owned []Territory
	for _, territory := range m.Territories {
		if owner, ok := m.TerritoryOwner(territory); ok && owner == index {
			owned = append(owned, territory)
		}
	}
	return owned
}

// その位置の土地のレベル（上げていなければ 1）
func (m Match) LandLevel(position board.Position) int {
	for _, land := range m.LandLevels {
		if land.Position == position {
			return land.Level
		}
	}
	return 1
}

// 領地の持ち主。そこにいるクリーチャーの持ち主
func (m Match) TerritoryOwner(territory Territory) (ParticipantIndex, bool) {
	placed, ok := m.PlacedCreature(territory.Creature)
	return placed.Owner, ok
}

// 侵略先の隣にある、その参加者の領地の位置（Position.Compare の順）。隣はボードのつながりだけで数え、転送円・橋の対は数えない（2026-11-09 オーナー）
// 無いときも nil でなく空にする（戦闘の入力として記録すると、nil は "adjacent_territories": null になるため。2026-11-14 オーナー。core#175）
func (m Match) adjacentTerritoriesOf(position board.Position, owner ParticipantIndex) []board.Position {
	positions := []board.Position{}
	for _, connection := range m.Board.Connections {
		neighbor := connection.A
		switch position {
		case connection.A:
			neighbor = connection.B
		case connection.B:
		default:
			continue
		}
		if territory, ok := m.TerritoryAt(neighbor); ok {
			if territoryOwner, ok := m.TerritoryOwner(territory); ok && territoryOwner == owner {
				positions = append(positions, neighbor)
			}
		}
	}
	slices.SortFunc(positions, board.Position.Compare)
	return slices.Compact(positions)
}

func (m Match) landElement(position board.Position) board.Element {
	for _, tile := range m.Board.Tiles {
		if tile.Position == position {
			return tile.Element
		}
	}
	return ""
}

// 同じ持ち主の、同じ属性の土地の数（自分を含む）。保存せず、そのたびに数える（領地が増減するたびに持ち主の全部の領地を書き直さないため）。
// 複は連鎖しない（2026-11-08 オーナー）
func (m Match) TerritoryChainCount(territory Territory) int {
	element := m.landElement(territory.Position)
	owner, ok := m.TerritoryOwner(territory)
	if !ok || element == board.ElementMulti {
		return 1
	}
	count := 0
	for _, other := range m.territoriesOf(owner) {
		if m.landElement(other.Position) == element {
			count++
		}
	}
	return count
}

// 土地の価値（基本の値 × レベルの倍率 × 連鎖の倍率。端数は切り捨て）
func (m Match) TerritoryValue(territory Territory) int {
	return m.Rules.BaseLandValue * m.Rules.landLevelMultiplier(m.LandLevel(territory.Position)) * m.Rules.chainMultiplier(m.TerritoryChainCount(territory)) / 100
}

// 盤面の通行料。価値 × 通行料の割合（端数は切り捨て）。いるクリーチャーの通行料の倍率は掛けない（払うときに発動する。core#159）
func (m Match) TerritoryToll(territory Territory) int {
	return m.TerritoryValue(territory) * m.Rules.TollPercentage.Int() / 100
}

// いるクリーチャーの通行料の倍率。持っていなければ false
func (m Match) tollMultiplier(territory Territory) (creature.TollMultiplierPercent, bool) {
	placed, ok := m.PlacedCreature(territory.Creature)
	if !ok {
		return 0, false
	}
	return placed.Base.Abilities.TollMultiplier()
}

// 手放したときに受け取る魔力（端数は切り捨て）
func (m Match) TerritoryRelinquishMagic(territory Territory) int {
	return m.TerritoryValue(territory) * m.Rules.RelinquishPercentage.Int() / 100
}

// 席の順の総魔力（手持ちの魔力 ＋ 自分の領地の価値の合計）。勝ちと順位はこれで決める
func (m Match) TotalMagics() []int {
	totals := make([]int, len(m.Participants))
	for i, participant := range m.Participants {
		totals[i] = participant.Magic
	}
	for _, territory := range m.Territories {
		if owner, ok := m.TerritoryOwner(territory); ok && int(owner) < len(totals) {
			totals[owner] += m.TerritoryValue(territory)
		}
	}
	return totals
}

// 止まったマスが、まだ誰の領地でもない土地か
func (m Match) isVacantLand(position board.Position) bool {
	for _, tile := range m.Board.Tiles {
		if tile.Position == position {
			_, occupied := m.TerritoryAt(position)
			return tile.Kind == board.TileKindLand && !occupied
		}
	}
	return false
}
