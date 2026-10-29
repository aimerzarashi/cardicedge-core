package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
)

// テスト用の値ではなく、開発で触る見本。本番に近い中身（火・地と水・風の属性のスターターデッキ）にし、ファイル名で見本と分かるようにする
//
//go:embed sample_master.json
var sampleMasterJSON []byte

type master struct {
	Cards []masterCard `json:"cards"`
	// 名前はカードの名前で指す（登録して初めて公開 ID が決まるため、ファイルに ID は書けない）
	PresetDecks []masterPresetDeck `json:"preset_decks"`
	// 写して作るプリセットデッキの名前。作って公開する（名前はプリセットデッキの名前のまま）
	StarterDecks []string `json:"starter_decks"`
	// 登録してレイアウトを入れ、公開する。公開の条件を満たすボードだけを書く
	Boards []masterBoard `json:"boards"`
}

type masterCard struct {
	Type string `json:"type"`
	Name string `json:"name"`
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
}

type masterBoardDefaultRules struct {
	TargetMagic     int `json:"target_magic"`
	StartingMagic   int `json:"starting_magic"`
	LapBonus        int `json:"lap_bonus"`
	CheckpointBonus int `json:"checkpoint_bonus"`
}

type masterBoardTile struct {
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Kind      string `json:"kind"`
	Attribute string `json:"attribute"`
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
	if len(m.StarterDecks) == 0 {
		return fmt.Errorf("starter decks: at least one is required")
	}
	starterNames := map[string]bool{}
	for _, name := range m.StarterDecks {
		if !deckNames[name] {
			return fmt.Errorf("starter decks: unknown preset deck %q", name)
		}
		if starterNames[name] {
			return fmt.Errorf("starter decks: %q is duplicated", name)
		}
		starterNames[name] = true
	}
	return m.validateBoards()
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
		if _, err := board.NewDefaultRules(b.DefaultRules.TargetMagic, b.DefaultRules.StartingMagic, b.DefaultRules.LapBonus, b.DefaultRules.CheckpointBonus); err != nil {
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

func (b masterBoard) layout() (board.Layout, error) {
	size, err := board.NewSize(b.Width, b.Height)
	if err != nil {
		return board.Layout{}, err
	}
	tiles := make([]board.Tile, 0, len(b.Tiles))
	for _, t := range b.Tiles {
		kind, err := board.NewTileKind(t.Kind)
		if err != nil {
			return board.Layout{}, err
		}
		tile := board.Tile{Position: board.NewPosition(t.X, t.Y), Kind: kind}
		if t.Attribute != "" {
			tile.Attribute, err = board.NewAttribute(t.Attribute)
			if err != nil {
				return board.Layout{}, err
			}
		}
		tiles = append(tiles, tile)
	}
	connections := make([]board.Connection, 0, len(b.Connections))
	for _, c := range b.Connections {
		connections = append(connections, board.NewConnection(board.NewPosition(c.A.X, c.A.Y), board.NewPosition(c.B.X, c.B.Y)))
	}
	return board.NewLayout(size, tiles, connections)
}

func (b masterBoard) defaultRulesInput() command.BoardDefaultRulesInput {
	return command.BoardDefaultRulesInput{
		TargetMagic: b.DefaultRules.TargetMagic, StartingMagic: b.DefaultRules.StartingMagic,
		LapBonus: b.DefaultRules.LapBonus, CheckpointBonus: b.DefaultRules.CheckpointBonus,
	}
}

func (b masterBoard) layoutInput() command.BoardLayoutInput {
	tiles := make([]command.BoardTileInput, 0, len(b.Tiles))
	for _, t := range b.Tiles {
		tiles = append(tiles, command.BoardTileInput{X: t.X, Y: t.Y, Kind: t.Kind, Attribute: t.Attribute})
	}
	connections := make([]command.BoardConnectionInput, 0, len(b.Connections))
	for _, c := range b.Connections {
		connections = append(connections, command.BoardConnectionInput{
			A: command.NewBoardPosition(c.A.X, c.A.Y),
			B: command.NewBoardPosition(c.B.X, c.B.Y),
		})
	}
	return command.BoardLayoutInput{Width: b.Width, Height: b.Height, Tiles: tiles, Connections: connections}
}
