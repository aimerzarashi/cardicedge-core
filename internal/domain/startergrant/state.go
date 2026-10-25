package startergrant

import "gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"

type State interface {
	isStarterGrantState()
}

type NotStartedStarterGrant struct{}

func (NotStartedStarterGrant) isStarterGrantState() {}

// 配る中身（開始のときに写したもの）と、どこまで済んだかを持つ。済んだ段は、子の集約ができたことの記録
type InProgressStarterGrant struct {
	ID                 ID
	Version            int64
	Player             player.ID
	Cards              []CardCount
	Decks              []Deck
	CollectionRecorded bool
	// 添字は DeckNumber - 1。長さは配るデッキの数（Decks と同じ）
	DeckRecorded []bool
}

func (InProgressStarterGrant) isStarterGrantState() {}

func (s InProgressStarterGrant) allRecorded() bool {
	if !s.CollectionRecorded {
		return false
	}
	for _, recorded := range s.DeckRecorded {
		if !recorded {
			return false
		}
	}
	return true
}

type CompletedStarterGrant struct {
	ID      ID
	Version int64
	Player  player.ID
}

func (CompletedStarterGrant) isStarterGrantState() {}

func InitialState() State {
	return NotStartedStarterGrant{}
}
