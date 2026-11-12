package match

import (
	"errors"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
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
	// 空いている土地に止まった後。クリーチャーを配置するか、配置せずにターンを終える
	PhaseDeploying Phase = "deploying"
	// 止まったマスに行動が無く、通った自分の領地があるとき、領地コマンドを実行するか何もしないかを選ぶ段階。
	// 行動があるマス（配置・侵略の段階）でも領地コマンドは受け付けるので、これは行動が無いマスのためだけの段階
	PhaseTerritoryCommanding Phase = "territory_commanding"
	// 他人の領地に止まった後。通行料を払うか、手札のクリーチャーで侵略するかを選ぶ（侵略できる手札が無くても、払うを選ぶ段階に入る。2026-11-09 オーナー）
	PhaseInvading Phase = "invading"
	// 侵略を確定した後。攻める側がアイテムを選ぶ（出せるものが無くても、出さないを選ぶ段階に入る。守る段階と同じ）
	PhaseAttacking Phase = "attacking"
	// 侵略された領地の持ち主がアイテムを選ぶ。番の人でない参加者の操作を待つ、ただ 1 つの段階
	PhaseDefending Phase = "defending"
	// 通行料を払って魔力が負になった後。0 以上になるまで自分の領地を手放す
	PhaseRelinquishing Phase = "relinquishing"
	PhaseEnding        Phase = "ending"
	PhaseEnded         Phase = "ended"
	// 進む途中だけの段階。Decide は進み終えるまで続けるので、保存された最後の状態が moving になることは無い
	PhaseMoving Phase = "moving"
	// 戦闘の途中だけの段階。moving と同じく、保存された最後の状態にはならない
	PhaseBattling Phase = "battling"
)

// 誰かの操作を待っている段階か。始まる前（段階が無い）と、版ごとに見たときの移動と戦闘の途中は待っていない
func (p Phase) AwaitsOperation() bool {
	var notStarted Phase
	return p != notStarted && p != PhaseMoving && p != PhaseBattling
}

// 戦闘のどちら側か
type BattleSide string

const (
	BattleSideAttacker BattleSide = "attacker"
	BattleSideDefender BattleSide = "defender"
)

func (s BattleSide) valid() bool {
	return s == BattleSideAttacker || s == BattleSideDefender
}

// 戦闘で発動した能力の種類（creature.Ability の種類の名前とそろえる）。戦闘の能力を足すときにここへ足す
type BattleAbility string

const (
	BattleAbilityFirstStrike BattleAbility = "first_strike"
	BattleAbilityLastStrike  BattleAbility = "last_strike"
)

func (a BattleAbility) valid() bool {
	return a == BattleAbilityFirstStrike || a == BattleAbilityLastStrike
}

func (s BattleSide) opponent() BattleSide {
	if s == BattleSideAttacker {
		return BattleSideDefender
	}
	return BattleSideAttacker
}

// 戦闘の中の AT・HP
type BattleStats struct {
	Attack    creature.Attack    `json:"attack"`
	HitPoints creature.HitPoints `json:"hit_points"`
}

// 戦闘の AT・HP を変えた効果の出どころ。種類ごとに 1 つだけ持つ（2026-11-09 オーナー。クリーチャーの能力・呪いは作るときに種類を足す）
type BattleStatSource struct {
	AdjacentTerritories *AdjacentTerritoriesSource `json:"adjacent_territories,omitempty"`
	Terrain             *TerrainSource             `json:"terrain,omitempty"`
	Item                *ItemSource                `json:"item,omitempty"`
}

// 支援。侵略先の隣にある、攻める側の領地の位置。support と呼ばないのは、能力の援護と取り違えないため
type AdjacentTerritoriesSource struct {
	Positions []board.Position `json:"positions"`
}

// 地形効果。効いた土地の属性とレベル
type TerrainSource struct {
	Element board.Element `json:"element"`
	Level   int           `json:"level"`
}

// 戦闘に出したアイテム
type ItemSource struct {
	Card card.Ref `json:"card"`
}

func NewAdjacentTerritoriesSource(positions []board.Position) BattleStatSource {
	return BattleStatSource{AdjacentTerritories: &AdjacentTerritoriesSource{Positions: positions}}
}

func NewTerrainSource(element board.Element, level int) BattleStatSource {
	return BattleStatSource{Terrain: &TerrainSource{Element: element, Level: level}}
}

func NewItemSource(ref card.Ref) BattleStatSource {
	return BattleStatSource{Item: &ItemSource{Card: ref}}
}

type EndReason string

const (
	EndReasonTargetMagic EndReason = "target_magic"
	EndReasonRoundLimit  EndReason = "round_limit"
)

// 対戦の結果。勝者は魔力が同じならすべて（参加者の番号の昇順）
type Result struct {
	Winners []ParticipantIndex `json:"winners"`
	// 参加者の番号（席の順）ごとの順位（1 始まり）。決め方は終わり方で違う（decide.go の ranksByMagic・ranksWithWinner）
	Ranks  []int     `json:"ranks"`
	Reason EndReason `json:"reason"`
}

// デッキのカード 1 種類。始めた時点のデッキの写しで、性能も写す（始めた後にカードを変えても対戦に響かず、判断と画面が
// 同じ値を使うため。2026-11-07 オーナー。core#112）
type DeckCard struct {
	Card     card.Ref          `json:"card"`
	Quantity card.DeckQuantity `json:"quantity"`
	Spec     CardSpec          `json:"spec"`
}

// 対戦で使うカードの性能の写し（始めた時点の studio のカードの性能）。項目はカードの性能を足すごとに増やす
type CardSpec struct {
	Cost   card.Cost   `json:"cost"`
	Rarity card.Rarity `json:"rarity"`
	// クリーチャーだけが持つ。アイテム・スペルは nil で、JSON にも出さない
	Creature *CreatureSpec `json:"creature,omitempty"`
	// アイテムだけが持つ。クリーチャー・スペルは nil で、JSON にも出さない
	Item *ItemSpec `json:"item,omitempty"`
	// スペルだけが持つ。クリーチャー・アイテムは nil で、JSON にも出さない
	Spell *SpellSpec `json:"spell,omitempty"`
}

// プレイヤーにかかっている呪い。効き方は種類ごとの値のどれか 1 つ、解ける条件は Expiry（今はターン数だけ。2026-11-10 オーナー）
type Curse struct {
	FixedDice *FixedDiceCurse `json:"fixed_dice,omitempty"`
	Expiry    CurseExpiry     `json:"expiry"`
}

// 次のサイコロの目を決める呪い（remainingTurns ターンで解ける）
func NewFixedDiceCurse(value, remainingTurns int) Curse {
	return Curse{FixedDice: &FixedDiceCurse{Value: value}, Expiry: CurseExpiry{RemainingTurns: remainingTurns}}
}

// 次のサイコロの目を決める（ホーリーワード）
type FixedDiceCurse struct {
	Value int `json:"value"`
}

// 呪いが解ける条件。RemainingTurns は、かけられた人が自分のターンを終えるたびに 1 減り、0 で解ける
type CurseExpiry struct {
	RemainingTurns int `json:"remaining_turns"`
}

// スペルの性能の写し。値の型は CreatureSpec と同じ理由で spell パッケージのものをそのまま使う
type SpellSpec struct {
	Effect spell.Effect `json:"effect"`
}

// アイテムの性能の写し。値の型は CreatureSpec と同じ理由で item パッケージのものをそのまま使う
type ItemSpec struct {
	Category       item.Category       `json:"category"`
	AttackBonus    item.AttackBonus    `json:"attack_bonus"`
	HitPointsBonus item.HitPointsBonus `json:"hit_points_bonus"`
}

// クリーチャーの性能の写し。値の型は creature パッケージのものをそのまま使う（match 側に同じ決まりの型を作ると、
// 決まりが 2 か所になり、写すときに変換が要るだけで得るものが無いため）
type CreatureSpec struct {
	Elements              creature.Elements              `json:"elements"`
	Attack                creature.Attack                `json:"attack"`
	HitPoints             creature.HitPoints             `json:"hit_points"`
	ItemRestrictions      creature.ItemRestrictions      `json:"item_restrictions"`
	PlacementRestrictions creature.PlacementRestrictions `json:"placement_restrictions"`
	Abilities             creature.Abilities             `json:"abilities"`
}

// アイテム制限。性能の無いアイテムは分類が無いので制限に当たらない
func (s CreatureSpec) canUseItem(candidate CardSpec) bool {
	return candidate.Item == nil || !s.ItemRestrictions.Forbids(candidate.Item.Category.String())
}

// 配置制限。配置・侵略・交換・移動の行き先と、土地の属性の変更の後の属性のどれにも、この 1 つで答える
func (s CreatureSpec) canBeActiveOn(element board.Element) bool {
	return !s.PlacementRestrictions.Forbids(element.String())
}

// 防御型（手札からの侵略にも、移動して侵略にも効く）
func (s CreatureSpec) canInvade() bool {
	return !s.Abilities.IsDefensive()
}

// 防御型（空き地への移動にも効く）
func (s CreatureSpec) canMove() bool {
	return !s.Abilities.IsDefensive()
}

// 攻撃の順に効く能力（先制か後手。持っていなければ無し）
func (s CreatureSpec) strikeAbility() (BattleAbility, bool) {
	switch {
	case s.Abilities.HasFirstStrike():
		return BattleAbilityFirstStrike, true
	case s.Abilities.HasLastStrike():
		return BattleAbilityLastStrike, true
	}
	return "", false
}

// 攻撃する順の強さ（大きい方が先）。先制と後手は同時に持てない（creature.NewAbilities が守る）
func (s CreatureSpec) strikePriority() int {
	switch {
	case s.Abilities.HasFirstStrike():
		return 1
	case s.Abilities.HasLastStrike():
		return -1
	}
	return 0
}

// 配置したクリーチャーを指す、対戦の中の通し番号（1 始まり）。位置で指さないのは、領地コマンドで移動・交換しても同じ 1 体を指すため
type DeployedCreatureID int

func NewDeployedCreatureID(value int) (DeployedCreatureID, error) {
	if value <= 0 {
		return 0, errors.New("deployed creature id must be positive")
	}
	return DeployedCreatureID(value), nil
}

func (id DeployedCreatureID) Int() int {
	return int(id)
}

// 戦利品のパックを指す名前（キー）。パックのマスタ（次のマイルストーン）ができるまでは、決まった名前で表す
type PackKey string

const DefaultPackKey PackKey = "default"

// 空のキーは作れない（選べるパックの中に無いものと同じ扱いにするため、呼ぶ側は作れなかったら選べないパックとして断る）
func NewPackKey(value string) (PackKey, error) {
	if value == "" {
		return "", errors.New("pack key must not be empty")
	}
	return PackKey(value), nil
}

// 選べる戦利品のパック。始めた時点の写し（パックの引き方は Decide がキーから決める）
type RewardPack struct {
	Key  PackKey `json:"key"`
	Name string  `json:"name"`
}

// 今選べるパックは既定の 1 つだけ（公開中のカード全体から等しい出やすさで引く）
func DefaultRewardPacks() []RewardPack {
	return []RewardPack{{Key: DefaultPackKey, Name: "通常パック"}}
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

// 対戦のルール。部屋の対戦ルール（ボードの既定のルールと同じ 8 つの値）と、全体のルールの手札の決まりの写し（始めた後にルールを変えても対戦に響かないため）。
// 8 つの値は board.DefaultRules で受け取る（目の範囲が 2 種類以上であることなどを、確かめずに作れないように）
type Rules struct {
	TargetMagic       int                         `json:"target_magic"`
	StartingMagic     int                         `json:"starting_magic"`
	LapBonus          int                         `json:"lap_bonus"`
	CheckpointBonus   int                         `json:"checkpoint_bonus"`
	RoundLimit        int                         `json:"round_limit"`
	DiceMin           int                         `json:"dice_min"`
	DiceMax           int                         `json:"dice_max"`
	BaseLandValue     int                         `json:"base_land_value"`
	MaxHandCards      gamerules.MaxHandCards      `json:"max_hand_cards"`
	StartingHandCards gamerules.StartingHandCards `json:"starting_hand_cards"`
	// 順位（1 位から）ごとの戦利品の枚数。全体のルールの写しで、数え方は rewardCards の 1 か所
	RewardCardsByRank gamerules.RewardCardsByRank `json:"reward_cards_by_rank"`
	// 戦利品に選べるパック
	RewardPacks []RewardPack `json:"reward_packs"`
	// 土地の価値の決まり。全体のルールの写し
	LandLevelMultipliers gamerules.LandLevelMultipliers `json:"land_level_multipliers"`
	ChainMultipliers     gamerules.ChainMultipliers     `json:"chain_multipliers"`
	TollPercentage       gamerules.TollPercentage       `json:"toll_percentage"`
	RelinquishPercentage gamerules.RelinquishPercentage `json:"relinquish_percentage"`
	// 戦闘の効果の決まり。全体のルールの写し
	TerrainBonusPerLevel   gamerules.TerrainBonusPerLevel   `json:"terrain_bonus_per_level"`
	AdjacentTerritoryBonus gamerules.AdjacentTerritoryBonus `json:"adjacent_territory_bonus"`
	// 土地の属性を変える費用の決まり。全体のルールの写し
	LandElementChangeMultiplier      gamerules.LandElementChangeMultiplier `json:"land_element_change_multiplier"`
	MultiLandElementChangeMultiplier gamerules.LandElementChangeMultiplier `json:"multi_land_element_change_multiplier"`
}

func NewRules(values board.DefaultRules, maxHandCards gamerules.MaxHandCards, startingHandCards gamerules.StartingHandCards, rewardCardsByRank gamerules.RewardCardsByRank, rewardPacks []RewardPack, landLevelMultipliers gamerules.LandLevelMultipliers, chainMultipliers gamerules.ChainMultipliers, tollPercentage gamerules.TollPercentage, relinquishPercentage gamerules.RelinquishPercentage, terrainBonusPerLevel gamerules.TerrainBonusPerLevel, adjacentTerritoryBonus gamerules.AdjacentTerritoryBonus, landElementChangeMultiplier, multiLandElementChangeMultiplier gamerules.LandElementChangeMultiplier) Rules {
	return Rules{
		TargetMagic: values.TargetMagic, StartingMagic: values.StartingMagic, LapBonus: values.LapBonus,
		CheckpointBonus: values.CheckpointBonus, RoundLimit: values.RoundLimit, DiceMin: values.DiceMin, DiceMax: values.DiceMax, BaseLandValue: values.BaseLandValue,
		MaxHandCards: maxHandCards, StartingHandCards: startingHandCards,
		RewardCardsByRank: rewardCardsByRank, RewardPacks: rewardPacks,
		LandLevelMultipliers: landLevelMultipliers, ChainMultipliers: chainMultipliers, TollPercentage: tollPercentage, RelinquishPercentage: relinquishPercentage,
		TerrainBonusPerLevel: terrainBonusPerLevel, AdjacentTerritoryBonus: adjacentTerritoryBonus,
		LandElementChangeMultiplier: landElementChangeMultiplier, MultiLandElementChangeMultiplier: multiLandElementChangeMultiplier,
	}
}

// 順位（1 位から）の戦利品の枚数。順位が一覧より下なら最後の値を使う（4 人より多い対戦でも、順位ごとの値を人数分そろえずに済むため）
func (r Rules) rewardCards(rank int) int {
	return r.RewardCardsByRank[min(max(rank, 1), len(r.RewardCardsByRank))-1].Int()
}

// レベルが一覧より上なら最後の値を使う（レベルの上げ下げは core#115。値の数を変えても、対戦中のレベルが範囲の外にならないように）
func (r Rules) landLevelMultiplier(level int) int {
	return r.LandLevelMultipliers[min(max(level, 1), len(r.LandLevelMultipliers))-1]
}

// 連鎖の数が一覧より多ければ最後の値を使う（2026-11-08 オーナー）
func (r Rules) chainMultiplier(count int) int {
	return r.ChainMultipliers[min(max(count, 1), len(r.ChainMultipliers))-1]
}

func (r Rules) hasRewardPack(key PackKey) bool {
	return slices.ContainsFunc(r.RewardPacks, func(pack RewardPack) bool { return pack.Key == key })
}
