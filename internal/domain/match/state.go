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
	// 直前にいたマス。向き無しのつながりで戻らないための記憶で、次のターンの最初の一歩でも使うのでターンを終えても残す。最初の一歩の前は無い
	Previous *board.Position
	// 拠点を出てから印を付けたチェックポイント
	PassedCheckpoints []board.Position
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
type DeployedCreature struct {
	ID    DeployedCreatureID
	Card  card.Ref
	Owner ParticipantIndex
	// 配置したときの性能の写し（対戦を始めたときの写しから写す。この 1 体だけを変える効果がほかの同じカードに響かないように）
	Base CreatureSpec
	// 今の体力
	HitPoints creature.HitPoints
}

// 領地。持ち主は、そこにいるクリーチャーの持ち主
type Territory struct {
	Position board.Position
	Creature DeployedCreatureID
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
	// 出したときの性能の写し
	Base CreatureSpec
	// 戦闘の前の体力（戦闘の中の値は Battle が持つ）
	HitPoints creature.HitPoints
	// 出したアイテム。出すまでと、出さなかったときは nil
	AttackerItem *card.Ref
	DefenderItem *card.Ref
}

// 戦闘の中の片方のクリーチャー
type BattleCreature struct {
	// 効果と攻撃を当てた今の値
	Stats BattleStats
	// 効果を当てる前の体力
	InitialHitPoints creature.HitPoints
	// 効果で増減した体力の合計
	HitPointsBonus int
}

// 戦闘の後に領地に残る体力。増えた分は先に減って残りは消え、下がった分は戦闘が終わると戻るので、残る傷は増えた分を超えたダメージだけ
func (c BattleCreature) territoryHitPoints() creature.HitPoints {
	damage := c.InitialHitPoints.Int() + c.HitPointsBonus - c.Stats.HitPoints.Int()
	return c.InitialHitPoints.Modified(-max(damage-max(c.HitPointsBonus, 0), 0))
}

// 戦闘が始まってから終わるまでの両方のクリーチャー
type Battle struct {
	Attacker BattleCreature
	Defender BattleCreature
}

func (b Battle) creature(side BattleSide) BattleCreature {
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

// どちらかの体力が 0 になっていて、戦闘が終わるか。AT は 0 になっても終わらない
func (b Battle) decided() bool {
	return b.Attacker.Stats.HitPoints.Int() == 0 || b.Defender.Stats.HitPoints.Int() == 0
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
	DeployedCreatures []DeployedCreature
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
	// 最後に振った配置したクリーチャーの ID。盤面から外れたクリーチャーの ID を使い回さないため、数を数え直さずにここで持つ
	LastDeployedCreatureID DeployedCreatureID
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
func (m Match) DeployedCreature(id DeployedCreatureID) (DeployedCreature, bool) {
	for _, deployed := range m.DeployedCreatures {
		if deployed.ID == id {
			return deployed, true
		}
	}
	return DeployedCreature{}, false
}

// 今の番の人が配置できる手札の番号（手札の順）。配置の段階でだけ中身がある。クリーチャーで、コストが今の魔力以下のもの
func (m Match) DeployChoices() []int {
	if m.Phase != PhaseDeploying {
		return nil
	}
	return m.payableCreatures()
}

// 今の番の人が侵略に出せる手札の番号（手札の順）。侵略を選ぶ段階でだけ中身がある。決まりは配置と同じで、これから変わりうるので関数は分ける
func (m Match) InvadeChoices() []int {
	if m.Phase != PhaseInvading {
		return nil
	}
	return m.payableCreatures()
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
	participant := m.Participants[index]
	specs := m.CardSpecs()
	var choices []int
	for i, ref := range participant.Hand {
		spec, ok := specs[ref]
		if ok && ref.EntityType == card.ItemEntityType && spec.Cost.Int() <= participant.Magic {
			choices = append(choices, i)
		}
	}
	return choices
}

// 今操作を待っている参加者。守る段階だけは領地の持ち主で、ほかは今の番の人
func (m Match) AwaitingParticipant() (ParticipantIndex, bool) {
	if m.Phase == PhaseDefending {
		territory, ok := m.invadedTerritory()
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
	return m.Phase == PhaseDeploying || m.Phase == PhaseInvading || m.Phase == PhaseTerritoryCommanding
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
		for _, handIndex := range m.payableCreatures() {
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
	// 払う魔力
	Magic int
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
		magic := m.landElementChangeMagic(current)
		if magic > m.Participants[index].Magic {
			continue
		}
		for _, element := range changeableLandElements {
			if element != current {
				choices = append(choices, ElementChangeChoice{Position: territory.Position, Element: element, Magic: magic})
			}
		}
	}
	return choices
}

// 基本の値 × 倍率。複属性の土地からは安い倍率（2026-11-10 オーナー）
func (m Match) landElementChangeMagic(current board.Element) int {
	if current == board.ElementMulti {
		return m.Rules.BaseLandValue * m.Rules.MultiLandElementChangeMultiplier.Int()
	}
	return m.Rules.BaseLandValue * m.Rules.LandElementChangeMultiplier.Int()
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
	deployed, ok := m.DeployedCreature(territory.Creature)
	return deployed.Owner, ok
}

// 侵略先の隣にある、その参加者の領地の位置（Position.Compare の順）。隣はボードのつながりだけで数え、転送円・橋の対は数えない（2026-11-09 オーナー）
func (m Match) adjacentTerritoriesOf(position board.Position, owner ParticipantIndex) []board.Position {
	var positions []board.Position
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
func (m Match) chainCount(territory Territory) int {
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
	return m.Rules.BaseLandValue * m.Rules.landLevelMultiplier(m.LandLevel(territory.Position)) * m.Rules.chainMultiplier(m.chainCount(territory)) / 100
}

// 他人が止まったときに払う魔力（端数は切り捨て）
func (m Match) TerritoryToll(territory Territory) int {
	return m.TerritoryValue(territory) * m.Rules.TollPercentage.Int() / 100
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
