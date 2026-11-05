package query

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// 公開 ID に当たる対戦が、そのプレイヤーの対戦として無い（無い・参加していない対戦。存在を明かさないため区別しない）
var ErrMatchNotFound = errors.New("query: match not found")

type MatchParticipantKind string

const (
	MatchParticipantKindPlayer MatchParticipantKind = "player"
	MatchParticipantKindNPC    MatchParticipantKind = "npc"
)

// 対戦の段階。今の番の人が次にする操作を表す
type MatchPhase string

const (
	MatchPhaseDrawing      MatchPhase = "drawing"
	MatchPhaseDiscarding   MatchPhase = "discarding"
	MatchPhaseRolling      MatchPhase = "rolling"
	MatchPhaseChoosingPath MatchPhase = "choosing_path"
	MatchPhaseEnding       MatchPhase = "ending"
	MatchPhaseEnded        MatchPhase = "ended"
)

type MatchEndReason string

const (
	MatchEndReasonTargetMagic MatchEndReason = "target_magic"
	MatchEndReasonRoundLimit  MatchEndReason = "round_limit"
)

// 対戦の結果。勝者は参加者の番号（Match.Participants の何番目か）
type MatchOutcome struct {
	Winners []int
	Reason  MatchEndReason
}

// 手札のカード 1 枚。名前と種類は写した cards から引く
type MatchCard struct {
	ID   uuid.UUID
	Name string
	Type CardType
}

// 対戦の盤面の写し。始めた時点のボードの形（対戦が自分の写しを持つので、ボードのマスタが変わっても変わらない）
type MatchBoard struct {
	Width       int
	Height      int
	Tiles       []BoardTile
	Connections []BoardConnection
	Pairs       []BoardPair
}

// 対戦のルール。始めた時点の写し（全体のルール MatchRules とは別の値）
type MatchPlayRules struct {
	TargetMagic       int
	StartingMagic     int
	LapBonus          int
	CheckpointBonus   int
	RoundLimit        int
	DiceMin           int
	DiceMax           int
	MaxHandCards      int
	StartingHandCards int
}

// 参加者 1 人。山札は並びも中身も見せず、枚数だけを持つ。TurnOrder は順番の何番目か（0 始まり）
type MatchParticipant struct {
	Kind          MatchParticipantKind
	Player        *uuid.UUID
	Avatar        RoomNamed
	Position      BoardPosition
	Magic         int
	Hand          []MatchCard
	DrawPileCount int
	TurnOrder     int
	// 拠点を出てから印を付けたチェックポイントの位置
	PassedCheckpoints []BoardPosition
}

// 対戦。read model を作らず、集約の状態から組み立てる。Participants は席の順
type Match struct {
	ID                 uuid.UUID
	Revision           int64
	Round              int
	Rules              MatchPlayRules
	Board              MatchBoard
	Participants       []MatchParticipant
	CurrentParticipant *int
	Phase              MatchPhase
	// このターンに振った目。振る前は nil
	Dice           *int
	RemainingSteps int
	// 今の番の人が選べる行き先。分かれ道で選ぶ段階のときだけ中身がある
	PathChoices []BoardPosition
	// 終わるまでは nil
	Outcome *MatchOutcome
}

// 名前を引く元（公開 ID）。アバター・NPC のアバターの名前と、カードの名前・種類を、それぞれの read model から 1 回で読む
type MatchNames struct {
	Avatars    map[uuid.UUID]string
	NPCAvatars map[uuid.UUID]string
	Cards      map[uuid.UUID]MatchCard
}

type LoadMatchNamesFunc func(ctx context.Context, avatarIDs, npcAvatarIDs, cardIDs []uuid.UUID) (MatchNames, error)

// 対戦を、read model ではなくイベントストアから読み直した集約の状態から返す（対戦に read model を作らない。組み立ての決まりを 1 か所にするため）。
// 参加者のプレイヤーだけが読める
type MatchHandler struct {
	eventStore domain.EventStore[match.ID, match.Event, match.State]
	matches    domain.Identifier[match.ID]
	players    domain.Identifier[player.ID]
	entityRefs domain.EntityRefResolver
	loadNames  LoadMatchNamesFunc
}

func NewMatchHandler(
	eventStore domain.EventStore[match.ID, match.Event, match.State],
	matches domain.Identifier[match.ID],
	players domain.Identifier[player.ID],
	entityRefs domain.EntityRefResolver,
	loadNames LoadMatchNamesFunc,
) *MatchHandler {
	return &MatchHandler{eventStore: eventStore, matches: matches, players: players, entityRefs: entityRefs, loadNames: loadNames}
}

// 参加していない対戦も、無いのと同じ ErrMatchNotFound にする
func (q *MatchHandler) Get(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (Match, error) {
	id, err := q.matches.Resolve(ctx, matchID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return Match{}, ErrMatchNotFound
	}
	if err != nil {
		return Match{}, err
	}
	viewer, err := q.players.Resolve(ctx, playerID)
	if err != nil {
		return Match{}, err
	}
	state, err := q.eventStore.Load(ctx, id)
	if err != nil {
		return Match{}, err
	}
	current, ok := state.(match.Match)
	if !ok || !current.HasPlayer(viewer) {
		return Match{}, ErrMatchNotFound
	}
	return q.build(ctx, matchID, current)
}

func (q *MatchHandler) build(ctx context.Context, publicID uuid.UUID, current match.Match) (Match, error) {
	var refs []domain.EntityRef
	for _, participant := range current.Participants {
		switch {
		case participant.Participant.Player != nil:
			p := participant.Participant.Player
			refs = append(refs, domain.NewEntityRef(player.Types().EntityType, p.Player.Int64()), domain.NewEntityRef(avatar.Types().EntityType, p.Avatar.Int64()))
		case participant.Participant.NPC != nil:
			refs = append(refs, domain.NewEntityRef(npcavatar.Types().EntityType, participant.Participant.NPC.Avatar.Int64()))
		}
		for _, hand := range participant.Hand {
			refs = append(refs, domain.NewEntityRef(hand.EntityType, hand.ID))
		}
	}
	publicIDs, err := q.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return Match{}, err
	}
	publicIDOf := func(entityType domain.EntityType, id int64) (uuid.UUID, error) {
		value, ok := publicIDs[domain.NewEntityRef(entityType, id)]
		if !ok {
			return uuid.Nil, fmt.Errorf("query: %s %d of match has no public id", entityType, id)
		}
		return value, nil
	}
	var avatarIDs, npcAvatarIDs, cardIDs []uuid.UUID
	participants := make([]MatchParticipant, len(current.Participants))
	for i, participant := range current.Participants {
		converted := MatchParticipant{
			Position: BoardPosition{X: participant.Position.X.Int(), Y: participant.Position.Y.Int()},
			Magic:    participant.Magic, DrawPileCount: len(participant.DrawPile), TurnOrder: turnOrderOf(current.TurnOrder, i),
			PassedCheckpoints: fromBoardPositions(participant.PassedCheckpoints),
		}
		switch {
		case participant.Participant.Player != nil:
			p := participant.Participant.Player
			converted.Kind = MatchParticipantKindPlayer
			playerPublicID, err := publicIDOf(player.Types().EntityType, p.Player.Int64())
			if err != nil {
				return Match{}, err
			}
			converted.Player = &playerPublicID
			if converted.Avatar.ID, err = publicIDOf(avatar.Types().EntityType, p.Avatar.Int64()); err != nil {
				return Match{}, err
			}
			avatarIDs = append(avatarIDs, converted.Avatar.ID)
		case participant.Participant.NPC != nil:
			var err error
			converted.Kind = MatchParticipantKindNPC
			if converted.Avatar.ID, err = publicIDOf(npcavatar.Types().EntityType, participant.Participant.NPC.Avatar.Int64()); err != nil {
				return Match{}, err
			}
			npcAvatarIDs = append(npcAvatarIDs, converted.Avatar.ID)
		default:
			return Match{}, fmt.Errorf("query: participant %d of match has neither player nor npc", i)
		}
		converted.Hand = make([]MatchCard, len(participant.Hand))
		for j, hand := range participant.Hand {
			cardID, err := publicIDOf(hand.EntityType, hand.ID)
			if err != nil {
				return Match{}, err
			}
			converted.Hand[j].ID = cardID
			cardIDs = append(cardIDs, cardID)
		}
		participants[i] = converted
	}
	names, err := q.loadNames(ctx, avatarIDs, npcAvatarIDs, cardIDs)
	if err != nil {
		return Match{}, err
	}
	for i := range participants {
		participant := &participants[i]
		source := names.Avatars
		if participant.Kind == MatchParticipantKindNPC {
			source = names.NPCAvatars
		}
		name, ok := source[participant.Avatar.ID]
		if !ok {
			return Match{}, fmt.Errorf("query: avatar %s of match is not found", participant.Avatar.ID)
		}
		participant.Avatar.Name = name
		for j := range participant.Hand {
			found, ok := names.Cards[participant.Hand[j].ID]
			if !ok {
				return Match{}, fmt.Errorf("query: card %s of match is not found", participant.Hand[j].ID)
			}
			participant.Hand[j] = found
		}
	}
	phase, err := fromMatchPhase(current.Phase)
	if err != nil {
		return Match{}, err
	}
	result := Match{
		ID: publicID, Revision: current.Version, Round: current.Round, Rules: fromMatchPlayRules(current.Rules),
		Board: fromMatchBoard(current.Board), Participants: participants,
		Phase: phase, RemainingSteps: current.RemainingSteps, PathChoices: []BoardPosition{},
	}
	if turn, ok := current.CurrentParticipant(); ok {
		index := turn.Int()
		result.CurrentParticipant = &index
	}
	if current.Dice > 0 {
		dice := current.Dice
		result.Dice = &dice
	}
	if phase == MatchPhaseChoosingPath {
		result.PathChoices = fromBoardPositions(current.PathChoices())
	}
	if current.Result != nil {
		outcome, err := fromMatchResult(*current.Result)
		if err != nil {
			return Match{}, err
		}
		result.Outcome = &outcome
	}
	return result, nil
}

// moving は Decide の途中だけの段階で、保存した状態には出ない。出たらコードの誤り
func fromMatchPhase(phase match.Phase) (MatchPhase, error) {
	switch phase {
	case match.PhaseDrawing:
		return MatchPhaseDrawing, nil
	case match.PhaseDiscarding:
		return MatchPhaseDiscarding, nil
	case match.PhaseRolling:
		return MatchPhaseRolling, nil
	case match.PhaseChoosingPath:
		return MatchPhaseChoosingPath, nil
	case match.PhaseEnding:
		return MatchPhaseEnding, nil
	case match.PhaseEnded:
		return MatchPhaseEnded, nil
	default:
		return "", fmt.Errorf("query: unexpected match phase: %q", phase)
	}
}

func fromMatchResult(result match.Result) (MatchOutcome, error) {
	var reason MatchEndReason
	switch result.Reason {
	case match.EndReasonTargetMagic:
		reason = MatchEndReasonTargetMagic
	case match.EndReasonRoundLimit:
		reason = MatchEndReasonRoundLimit
	default:
		return MatchOutcome{}, fmt.Errorf("query: unknown match end reason: %q", result.Reason)
	}
	winners := make([]int, len(result.Winners))
	for i, winner := range result.Winners {
		winners[i] = winner.Int()
	}
	return MatchOutcome{Winners: winners, Reason: reason}, nil
}

func fromBoardPositions(positions []board.Position) []BoardPosition {
	converted := make([]BoardPosition, len(positions))
	for i, position := range positions {
		converted[i] = NewBoardPosition(position.X.Int(), position.Y.Int())
	}
	return converted
}

func fromMatchPlayRules(rules match.Rules) MatchPlayRules {
	return MatchPlayRules{
		TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus, CheckpointBonus: rules.CheckpointBonus,
		RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax,
		MaxHandCards: rules.MaxHandCards.Int(), StartingHandCards: rules.StartingHandCards.Int(),
	}
}

func fromMatchBoard(source match.Board) MatchBoard {
	position := func(p board.Position) BoardPosition {
		return NewBoardPosition(p.X.Int(), p.Y.Int())
	}
	result := MatchBoard{Width: source.Size.Width.Int(), Height: source.Size.Height.Int()}
	result.Tiles = make([]BoardTile, len(source.Tiles))
	for i, tile := range source.Tiles {
		result.Tiles[i] = BoardTile{X: tile.Position.X.Int(), Y: tile.Position.Y.Int(), Kind: BoardTileKind(tile.Kind), Attribute: BoardTileAttribute(tile.Attribute)}
	}
	result.Connections = make([]BoardConnection, len(source.Connections))
	for i, connection := range source.Connections {
		result.Connections[i] = BoardConnection{A: position(connection.A), B: position(connection.B)}
	}
	result.Pairs = make([]BoardPair, len(source.Pairs))
	for i, pair := range source.Pairs {
		result.Pairs[i] = BoardPair{A: position(pair.A), B: position(pair.B)}
	}
	return result
}

// 順番の何番目か。ParticipantIndex は席の番号なので、番号 i を持つものの位置を探す
func turnOrderOf(order []match.ParticipantIndex, seat int) int {
	return slices.IndexFunc(order, func(index match.ParticipantIndex) bool {
		return index.Int() == seat
	})
}
