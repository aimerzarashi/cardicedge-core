package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"unicode/utf8"
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
	return nil
}
