package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
)

// テスト用の値ではなく、開発で触る見本。本番に近い中身（火・地と水・風の属性の配布デッキ）にし、ファイル名で見本と分かるようにする
//
//go:embed sample_master.json
var sampleMasterJSON []byte

type master struct {
	Cards []masterCard `json:"cards"`
	// 名前はカードの名前で指す（登録して初めて公開 ID が決まるため、ファイルに ID は書けない）
	PresetDecks []masterPresetDeck `json:"preset_decks"`
	// 写して作るプリセットデッキの名前。作って公開する（名前はプリセットデッキの名前のまま）
	GrantDecks []string `json:"grant_decks"`
	// 登録してレイアウトを入れ、公開する。公開の条件を満たすボードだけを書く
	Boards []masterBoard `json:"boards"`
	// 登録して使う NPC のデッキを割り当て、公開する NPC のアバター
	NPCAvatars []masterNPCAvatar `json:"npc_avatars"`
	// プリセットデッキから、元と同じ名前を指定して写して、公開する NPC のデッキ。プリセットデッキの名前で書く
	NPCDecks []string `json:"npc_decks"`
	// 登録して配るデッキ（grant_decks に書いた名前）と解放するボード（boards に書いた名前）を指定し、公開するスターターセット。
	// 公開中にできるのは同時に 1 つだけなので、公開するのは 1 つだけ書く
	StarterSets []masterStarterSet `json:"starter_sets"`
}

type masterStarterSet struct {
	Name         string   `json:"name"`
	GrantDecks   []string `json:"grant_decks"`
	UnlockBoards []string `json:"unlock_boards"`
	// npc_avatars に書いた名前。解放する NPC のアバター
	UnlockNPCAvatars []string `json:"unlock_npc_avatars"`
}

// 公開するには公開中のデッキが 1 つ以上要るため、デッキは 1 つ以上書く。NPC のデッキの名前（npc_decks に書いたもの）で指す
type masterNPCAvatar struct {
	Name  string   `json:"name"`
	Decks []string `json:"decks"`
}

// 性能は準備中の間に入れてから公開する（公開した後は変えられないため）。属性・攻撃力・体力はクリーチャーだけが持つ
// （攻撃力は 0 が正しい値なので、書き忘れと見分けるためにポインタで受ける）
type masterCard struct {
	Type                  string   `json:"type"`
	Name                  string   `json:"name"`
	Cost                  int      `json:"cost"`
	Rarity                string   `json:"rarity"`
	Elements              []string `json:"elements"`
	Attack                *int     `json:"attack"`
	HitPoints             *int     `json:"hit_points"`
	ItemRestrictions      []string `json:"item_restrictions"`
	PlacementRestrictions []string `json:"placement_restrictions"`
	Abilities             []string `json:"abilities"`
	// アイテムだけが持つ（増減は 0 が正しい値なので、攻撃力と同じくポインタで受ける）
	Category       string `json:"category"`
	AttackBonus    *int   `json:"attack_bonus"`
	HitPointsBonus *int   `json:"hit_points_bonus"`
	// スペルだけが持つ。書かなければ効果なし（効果の無いスペルを公開できない決まりは、効果の種類がそろうまで効かせない。core#149）。
	// 形は studio の read model と同じく、種類の名前の下に値を入れる（{"draw_cards":{"count":1}}）
	Effect *masterSpellEffect `json:"effect"`
}

type masterSpellEffect struct {
	DrawCards *struct {
		Count int `json:"count"`
	} `json:"draw_cards"`
	GainMagic *struct {
		PerLap int `json:"per_lap"`
	} `json:"gain_magic"`
	RollDice *struct {
		Count int `json:"count"`
	} `json:"roll_dice"`
	FixedDiceCurse *struct {
		Value int `json:"value"`
		Turns int `json:"turns"`
	} `json:"fixed_dice_curse"`
	Damage *struct {
		Amount int `json:"amount"`
	} `json:"damage"`
}

// 種類がどれも無い効果は書き間違い（効果なしは effect を書かない）
func (c masterCard) validateSpellSpec() error {
	if c.Effect == nil {
		return nil
	}
	if c.Type != "spell" {
		return fmt.Errorf("%s cannot have effect", c.Type)
	}
	kinds := 0
	var err error
	if c.Effect.DrawCards != nil {
		kinds++
		_, err = spell.NewDrawCount(c.Effect.DrawCards.Count)
	}
	if c.Effect.GainMagic != nil {
		kinds++
		_, err = spell.NewMagicPerLap(c.Effect.GainMagic.PerLap)
	}
	if c.Effect.RollDice != nil {
		kinds++
		_, err = spell.NewDiceCount(c.Effect.RollDice.Count)
	}
	if curse := c.Effect.FixedDiceCurse; curse != nil {
		kinds++
		if _, err = spell.NewDiceFace(curse.Value); err == nil {
			_, err = spell.NewCurseTurns(curse.Turns)
		}
	}
	if c.Effect.Damage != nil {
		kinds++
		_, err = spell.NewDamageAmount(c.Effect.Damage.Amount)
	}
	if kinds != 1 {
		return errors.New("spell effect needs exactly one kind")
	}
	return err
}

// 見本の効果を、コマンドの入力にする（書いていなければ効果なし）
func (c masterCard) spellEffect() command.SpellEffect {
	var effect command.SpellEffect
	if c.Effect == nil {
		return effect
	}
	if c.Effect.DrawCards != nil {
		effect.DrawCards = &command.SpellDrawCards{Count: c.Effect.DrawCards.Count}
	}
	if c.Effect.GainMagic != nil {
		effect.GainMagic = &command.SpellGainMagic{PerLap: c.Effect.GainMagic.PerLap}
	}
	if c.Effect.RollDice != nil {
		effect.RollDice = &command.SpellRollDice{Count: c.Effect.RollDice.Count}
	}
	if curse := c.Effect.FixedDiceCurse; curse != nil {
		effect.FixedDiceCurse = &command.SpellFixedDiceCurse{Value: curse.Value, Turns: curse.Turns}
	}
	if c.Effect.Damage != nil {
		effect.Damage = &command.SpellDamage{Amount: c.Effect.Damage.Amount}
	}
	return effect
}

type masterPresetDeck struct {
	Name  string            `json:"name"`
	Cards []masterDeckEntry `json:"cards"`
}

type masterDeckEntry struct {
	Card  string `json:"card"`
	Count int    `json:"count"`
}

type masterBoard struct {
	Name         string                  `json:"name"`
	Width        int                     `json:"width"`
	Height       int                     `json:"height"`
	DefaultRules masterBoardDefaultRules `json:"default_rules"`
	Tiles        []masterBoardTile       `json:"tiles"`
	Connections  []masterBoardConnection `json:"connections"`
	Pairs        []masterBoardConnection `json:"pairs"`
}

type masterBoardDefaultRules struct {
	TargetMagic     int `json:"target_magic"`
	StartingMagic   int `json:"starting_magic"`
	LapBonus        int `json:"lap_bonus"`
	CheckpointBonus int `json:"checkpoint_bonus"`
	RoundLimit      int `json:"round_limit"`
	DiceMin         int `json:"dice_min"`
	DiceMax         int `json:"dice_max"`
	BaseLandValue   int `json:"base_land_value"`
}

type masterBoardTile struct {
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Kind    string `json:"kind"`
	Element string `json:"element"`
}

type masterBoardPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type masterBoardConnection struct {
	A masterBoardPosition `json:"a"`
	B masterBoardPosition `json:"b"`
}

const (
	nameMaxLength  = 50
	cardMaxCount   = 4
	deckTotalCount = 40
)

// クリーチャーは属性・攻撃力・体力・アイテム制限・配置制限・能力（無ければ []）を必ず書き、アイテム・スペルは書かない（書いても入れる先が無く、黙って捨てることになるため）
func (c masterCard) validateCreatureSpec() error {
	if c.Type != "creature" {
		if len(c.Elements) > 0 || c.Attack != nil || c.HitPoints != nil || c.ItemRestrictions != nil || c.PlacementRestrictions != nil || c.Abilities != nil {
			return fmt.Errorf("%s cannot have elements, attack, hit_points, item_restrictions, placement_restrictions or abilities", c.Type)
		}
		return nil
	}
	if _, err := creature.NewElements(c.Elements); err != nil {
		return err
	}
	if c.Attack == nil || c.HitPoints == nil || c.ItemRestrictions == nil || c.PlacementRestrictions == nil || c.Abilities == nil {
		return errors.New("creature needs attack, hit_points, item_restrictions, placement_restrictions and abilities")
	}
	if _, err := creature.NewItemRestrictions(c.ItemRestrictions); err != nil {
		return err
	}
	if _, err := creature.NewPlacementRestrictions(c.PlacementRestrictions); err != nil {
		return err
	}
	for _, ability := range c.Abilities {
		if ability != "defensive" {
			return fmt.Errorf("unknown ability %q", ability)
		}
	}
	if _, err := creature.NewAttack(*c.Attack); err != nil {
		return err
	}
	_, err := creature.NewHitPoints(*c.HitPoints)
	return err
}

// アイテムは分類と増減を必ず書き、クリーチャー・スペルは書かない（validateCreatureSpec と同じ理由）
func (c masterCard) validateItemSpec() error {
	if c.Type != "item" {
		if c.Category != "" || c.AttackBonus != nil || c.HitPointsBonus != nil {
			return fmt.Errorf("%s cannot have category, attack_bonus or hit_points_bonus", c.Type)
		}
		return nil
	}
	if c.AttackBonus == nil || c.HitPointsBonus == nil {
		return errors.New("item needs attack_bonus and hit_points_bonus")
	}
	_, err := item.NewCategory(c.Category)
	return err
}

func loadMaster(raw []byte) (master, error) {
	var m master
	if err := json.Unmarshal(raw, &m); err != nil {
		return master{}, err
	}
	return m, m.validate()
}

// 登録の入口（コマンド）も確かめるが、途中まで入れてから誤りに気づかないよう、始める前に見本のファイルの誤りを見つける
func (m master) validate() error {
	cardNames := map[string]bool{}
	for _, c := range m.Cards {
		switch c.Type {
		case "creature", "item", "spell":
		default:
			return fmt.Errorf("card %q: unknown type %q", c.Name, c.Type)
		}
		if n := utf8.RuneCountInString(c.Name); n == 0 || n > nameMaxLength {
			return fmt.Errorf("card %q: name length %d is out of range", c.Name, n)
		}
		if cardNames[c.Name] {
			return fmt.Errorf("card %q: duplicated", c.Name)
		}
		cardNames[c.Name] = true
		if _, err := card.NewCost(c.Cost); err != nil {
			return fmt.Errorf("card %q: %w", c.Name, err)
		}
		if _, err := card.NewRarity(c.Rarity); err != nil {
			return fmt.Errorf("card %q: %w", c.Name, err)
		}
		if err := c.validateItemSpec(); err != nil {
			return fmt.Errorf("card %q: %w", c.Name, err)
		}
		if err := c.validateCreatureSpec(); err != nil {
			return fmt.Errorf("card %q: %w", c.Name, err)
		}
		if err := c.validateSpellSpec(); err != nil {
			return fmt.Errorf("card %q: %w", c.Name, err)
		}
	}
	deckNames := map[string]bool{}
	for _, d := range m.PresetDecks {
		if n := utf8.RuneCountInString(d.Name); n == 0 || n > nameMaxLength {
			return fmt.Errorf("preset deck %q: name length %d is out of range", d.Name, n)
		}
		if deckNames[d.Name] {
			return fmt.Errorf("preset deck %q: duplicated", d.Name)
		}
		deckNames[d.Name] = true
		total := 0
		seen := map[string]bool{}
		for _, e := range d.Cards {
			if !cardNames[e.Card] {
				return fmt.Errorf("preset deck %q: unknown card %q", d.Name, e.Card)
			}
			if seen[e.Card] {
				return fmt.Errorf("preset deck %q: card %q is duplicated", d.Name, e.Card)
			}
			seen[e.Card] = true
			if e.Count < 1 || e.Count > cardMaxCount {
				return fmt.Errorf("preset deck %q: card %q count %d is out of range", d.Name, e.Card, e.Count)
			}
			total += e.Count
		}
		if total != deckTotalCount {
			return fmt.Errorf("preset deck %q: total %d is not %d", d.Name, total, deckTotalCount)
		}
	}
	if len(m.GrantDecks) == 0 {
		return fmt.Errorf("grant decks: at least one is required")
	}
	starterNames := map[string]bool{}
	for _, name := range m.GrantDecks {
		if !deckNames[name] {
			return fmt.Errorf("grant decks: unknown preset deck %q", name)
		}
		if starterNames[name] {
			return fmt.Errorf("grant decks: %q is duplicated", name)
		}
		starterNames[name] = true
	}
	if err := m.validateBoards(); err != nil {
		return err
	}
	npcDeckNames, err := m.validateNPCDecks(deckNames)
	if err != nil {
		return err
	}
	if err := m.validateNPCAvatars(npcDeckNames); err != nil {
		return err
	}
	return m.validateStarterSets(starterNames)
}

// 名前の決まりは domain の starterset.NewName で確かめる。公開中の数の上限は、コマンドが守る決まり（quota）なので、
// 見本のファイルでも 1 つだけにする（2 つ書くと、流すたびに 2 つ目の公開で止まるため）
func (m master) validateStarterSets(grantDeckNames map[string]bool) error {
	if len(m.StarterSets) > starterset.MaxPublished {
		return fmt.Errorf("starter sets: at most %d can be published", starterset.MaxPublished)
	}
	boardNames := map[string]bool{}
	for _, b := range m.Boards {
		boardNames[b.Name] = true
	}
	npcAvatarNames := map[string]bool{}
	for _, a := range m.NPCAvatars {
		npcAvatarNames[a.Name] = true
	}
	names := map[string]bool{}
	for _, set := range m.StarterSets {
		validName, err := starterset.NewName(set.Name)
		if err != nil {
			return fmt.Errorf("starter set %q: %w", set.Name, err)
		}
		key := domain.NormalizeName(validName.String())
		if names[key] {
			return fmt.Errorf("starter set %q: duplicated", set.Name)
		}
		names[key] = true
		if len(set.GrantDecks) == 0 {
			return fmt.Errorf("starter set %q: at least one grant deck is required", set.Name)
		}
		if err := checkReferences(set.GrantDecks, grantDeckNames); err != nil {
			return fmt.Errorf("starter set %q: grant decks: %w", set.Name, err)
		}
		if err := checkReferences(set.UnlockBoards, boardNames); err != nil {
			return fmt.Errorf("starter set %q: unlock boards: %w", set.Name, err)
		}
		if err := checkReferences(set.UnlockNPCAvatars, npcAvatarNames); err != nil {
			return fmt.Errorf("starter set %q: unlock npc avatars: %w", set.Name, err)
		}
	}
	return nil
}

// names の名前が、known にあって重ならないこと
func checkReferences(names []string, known map[string]bool) error {
	seen := map[string]bool{}
	for _, name := range names {
		if !known[name] {
			return fmt.Errorf("unknown %q", name)
		}
		if seen[name] {
			return fmt.Errorf("%q is duplicated", name)
		}
		seen[name] = true
	}
	return nil
}

// レイアウトの決まりと公開の条件は、コマンドと同じ domain の関数で確かめる（ここに書き写さない）。公開できないボードを見本に
// 書くと、流すたびに公開で止まるため、始める前に見つける
func (m master) validateBoards() error {
	names := map[string]bool{}
	for _, b := range m.Boards {
		if n := utf8.RuneCountInString(b.Name); n == 0 || n > nameMaxLength {
			return fmt.Errorf("board %q: name length %d is out of range", b.Name, n)
		}
		if names[b.Name] {
			return fmt.Errorf("board %q: duplicated", b.Name)
		}
		names[b.Name] = true
		// 決まりは domain の関数で確かめる。書き忘れ（0）もここで見つかる
		if _, err := board.NewDefaultRules(b.DefaultRules.TargetMagic, b.DefaultRules.StartingMagic, b.DefaultRules.LapBonus, b.DefaultRules.CheckpointBonus,
			b.DefaultRules.RoundLimit, b.DefaultRules.DiceMin, b.DefaultRules.DiceMax, b.DefaultRules.BaseLandValue); err != nil {
			return fmt.Errorf("board %q: %w", b.Name, err)
		}
		layout, err := b.layout()
		if err != nil {
			return fmt.Errorf("board %q: %w", b.Name, err)
		}
		if readiness := layout.Readiness(); !readiness.Publishable() {
			return fmt.Errorf("board %q: not publishable: %+v", b.Name, readiness)
		}
	}
	return nil
}

// 名前の決まりは domain の npcavatar.NewName で確かめる（ここに長さを書き写さない）。重なりは登録の入口と同じ比べ方で見つける
func (m master) validateNPCAvatars(npcDeckNames map[string]bool) error {
	names := map[string]bool{}
	for _, a := range m.NPCAvatars {
		validName, err := npcavatar.NewName(a.Name)
		if err != nil {
			return fmt.Errorf("npc avatar %q: %w", a.Name, err)
		}
		key := domain.NormalizeName(validName.String())
		if names[key] {
			return fmt.Errorf("npc avatar %q: duplicated", a.Name)
		}
		names[key] = true
		if len(a.Decks) == 0 {
			return fmt.Errorf("npc avatar %q: at least one deck is required", a.Name)
		}
		decks := map[string]bool{}
		for _, deck := range a.Decks {
			if !npcDeckNames[deck] {
				return fmt.Errorf("npc avatar %q: unknown npc deck %q", a.Name, deck)
			}
			if decks[deck] {
				return fmt.Errorf("npc avatar %q: deck %q is duplicated", a.Name, deck)
			}
			decks[deck] = true
		}
	}
	return nil
}

// 確かめた NPC のデッキの名前の集まりを返す（NPC のアバターが指すため）
func (m master) validateNPCDecks(presetDeckNames map[string]bool) (map[string]bool, error) {
	names := map[string]bool{}
	for _, name := range m.NPCDecks {
		if !presetDeckNames[name] {
			return nil, fmt.Errorf("npc decks: unknown preset deck %q", name)
		}
		if names[name] {
			return nil, fmt.Errorf("npc decks: %q is duplicated", name)
		}
		names[name] = true
	}
	return names, nil
}

// 幅・高さの範囲は、ファイルの確かめでは studio_rules の既定の値を使う（実際の範囲は適用時にコマンドが studio_rules から読んで確かめる。
// ここは適用の前に、ファイルの誤りを早く見つけるための確かめ）
func (b masterBoard) layout() (board.Layout, error) {
	within := studiorules.Defaults().Board
	width, err := board.NewWidth(b.Width, within)
	if err != nil {
		return board.Layout{}, err
	}
	height, err := board.NewHeight(b.Height, within)
	if err != nil {
		return board.Layout{}, err
	}
	size := board.Size{Width: width, Height: height}
	tiles := make([]board.Tile, 0, len(b.Tiles))
	for _, t := range b.Tiles {
		kind, err := board.NewTileKind(t.Kind)
		if err != nil {
			return board.Layout{}, err
		}
		position, err := newPosition(t.X, t.Y)
		if err != nil {
			return board.Layout{}, err
		}
		tile := board.Tile{Position: position, Kind: kind}
		if t.Element != "" {
			tile.Element, err = board.NewElement(t.Element)
			if err != nil {
				return board.Layout{}, err
			}
		}
		tiles = append(tiles, tile)
	}
	connections := make([]board.Connection, 0, len(b.Connections))
	for _, c := range b.Connections {
		a, err := newPosition(c.A.X, c.A.Y)
		if err != nil {
			return board.Layout{}, err
		}
		b, err := newPosition(c.B.X, c.B.Y)
		if err != nil {
			return board.Layout{}, err
		}
		connections = append(connections, board.NewConnection(a, b))
	}
	pairs := make([]board.Pair, 0, len(b.Pairs))
	for _, p := range b.Pairs {
		a, err := newPosition(p.A.X, p.A.Y)
		if err != nil {
			return board.Layout{}, err
		}
		b, err := newPosition(p.B.X, p.B.Y)
		if err != nil {
			return board.Layout{}, err
		}
		pairs = append(pairs, board.NewPair(a, b))
	}
	return board.NewLayout(size, tiles, connections, pairs)
}

func newPosition(x, y int) (board.Position, error) {
	px, err := board.NewX(x)
	if err != nil {
		return board.Position{}, err
	}
	py, err := board.NewY(y)
	if err != nil {
		return board.Position{}, err
	}
	return board.Position{X: px, Y: py}, nil
}

func (b masterBoard) defaultRulesInput() command.BoardDefaultRulesInput {
	return command.BoardDefaultRulesInput{
		TargetMagic: b.DefaultRules.TargetMagic, StartingMagic: b.DefaultRules.StartingMagic,
		LapBonus: b.DefaultRules.LapBonus, CheckpointBonus: b.DefaultRules.CheckpointBonus,
		RoundLimit: b.DefaultRules.RoundLimit, DiceMin: b.DefaultRules.DiceMin, DiceMax: b.DefaultRules.DiceMax, BaseLandValue: b.DefaultRules.BaseLandValue,
	}
}

func (b masterBoard) layoutInput() command.BoardLayoutInput {
	tiles := make([]command.BoardTileInput, 0, len(b.Tiles))
	for _, t := range b.Tiles {
		tiles = append(tiles, command.BoardTileInput{X: t.X, Y: t.Y, Kind: t.Kind, Element: t.Element})
	}
	connections := make([]command.BoardConnectionInput, 0, len(b.Connections))
	for _, c := range b.Connections {
		connections = append(connections, command.BoardConnectionInput{
			A: command.NewBoardPosition(c.A.X, c.A.Y),
			B: command.NewBoardPosition(c.B.X, c.B.Y),
		})
	}
	pairs := make([]command.BoardPairInput, 0, len(b.Pairs))
	for _, p := range b.Pairs {
		pairs = append(pairs, command.BoardPairInput{
			A: command.NewBoardPosition(p.A.X, p.A.Y),
			B: command.NewBoardPosition(p.B.X, p.B.Y),
		})
	}
	return command.BoardLayoutInput{Width: b.Width, Height: b.Height, Tiles: tiles, Connections: connections, Pairs: pairs}
}
