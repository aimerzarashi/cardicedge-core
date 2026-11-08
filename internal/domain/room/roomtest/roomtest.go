package roomtest

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar/avatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck/decktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules/gamerulestest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
)

// テストでも値の型は生成関数で作る（factory_enforcer）

func ID(t testing.TB, value int64) room.ID {
	t.Helper()
	id, err := room.NewID(value)
	require.NoError(t, err)
	return id
}

func Name(t testing.TB, value string) room.Name {
	t.Helper()
	name, err := room.NewName(value)
	require.NoError(t, err)
	return name
}

func SeatIndex(t testing.TB, value int) room.SeatIndex {
	t.Helper()
	index, err := room.NewSeatIndex(value)
	require.NoError(t, err)
	return index
}

func SeatKind(t testing.TB, value string) room.SeatKind {
	t.Helper()
	kind, err := room.NewSeatKind(value)
	require.NoError(t, err)
	return kind
}

// 全体のルールの人数の範囲
func PlayersRange(t testing.TB, minPlayers, maxPlayers int) (gamerules.MinPlayers, gamerules.MaxPlayers) {
	t.Helper()
	match := gamerulestest.Match(t, minPlayers, maxPlayers, 1, 1)
	return match.MinPlayers, match.MaxPlayers
}

// 人数の範囲は 2〜4 人（全体のルールの既定の値）
func SeatKinds(t testing.TB, kinds ...room.SeatKind) room.SeatKinds {
	t.Helper()
	minPlayers, maxPlayers := PlayersRange(t, 2, 4)
	seats, err := room.NewSeatKinds(kinds, minPlayers, maxPlayers)
	require.NoError(t, err)
	return seats
}

func PlayerChoice(t testing.TB, avatarID, deckID int64) room.SeatChoice {
	t.Helper()
	return room.NewPlayerSeatChoice(avatartest.ID(t, avatarID), decktest.ID(t, deckID))
}

func NPCChoice(t testing.TB, npcAvatarID, npcDeckID int64) room.SeatChoice {
	t.Helper()
	return room.NewNPCSeatChoice(npcavatartest.ID(t, npcAvatarID), npcdecktest.ID(t, npcDeckID))
}

// ボードの既定のルールの登録の直後の値
func MatchRules(t testing.TB, boardID board.ID) room.MatchRules {
	t.Helper()
	rules := board.InitialDefaultRules()
	matchRules, err := room.NewMatchRules(boardID, rules.TargetMagic, rules.StartingMagic, rules.LapBonus, rules.CheckpointBonus, rules.RoundLimit, rules.DiceMin, rules.DiceMax, rules.BaseLandValue)
	require.NoError(t, err)
	return matchRules
}
