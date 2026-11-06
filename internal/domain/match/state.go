package match

import (
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
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
