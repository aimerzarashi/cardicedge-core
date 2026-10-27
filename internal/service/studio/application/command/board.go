package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type BoardHandler struct {
	eventStore domain.EventStore[board.ID, board.Event, board.State]
	identifier domain.Identifier[board.ID]
	updateName board.UpdateNameFunc
	getRules   GetStudioRulesFunc
	now        func() time.Time
}

func NewBoardHandler(
	eventStore domain.EventStore[board.ID, board.Event, board.State],
	identifier domain.Identifier[board.ID],
	updateName board.UpdateNameFunc,
	getRules GetStudioRulesFunc,
	now func() time.Time,
) *BoardHandler {
	return &BoardHandler{eventStore: eventStore, identifier: identifier, updateName: updateName, getRules: getRules, now: now}
}

// 変える操作の結果は、公開 ID と新しい revision だけにする（理由は ChangedPresetDeck と同じ）
type ChangedBoard struct {
	ID       uuid.UUID
	Revision int64
}

// x が列、y が行（0 始まり）。マスの座標・つながりの端の入力と、公開できないマスの出力の両方に使う
type BoardPosition struct {
	X int
	Y int
}

func NewBoardPosition(x, y int) BoardPosition {
	return BoardPosition{X: x, Y: y}
}

// kind は land・base・checkpoint（GraphQL では enum）
type BoardTileInput struct {
	X    int
	Y    int
	Kind string
}

type BoardConnectionInput struct {
	A BoardPosition
	B BoardPosition
}

// 変えた後のレイアウトの全体。前との差分は Decide が作る
type BoardLayoutInput struct {
	Width       int
	Height      int
	Tiles       []BoardTileInput
	Connections []BoardConnectionInput
}

// 空のボード（準備中。四角形でマスもつながりも無い。大きさは 8×8 を studio_rules の範囲に収めた値）を作る。操作した人は context のオペレーター。
// 入力の誤りは input.Errors、使われている名前（状態によらず、すべてのボードの中で）なら ErrBoardNameAlreadyInUse
func (c *BoardHandler) Register(ctx context.Context, name string) (ChangedBoard, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedBoard{}, ErrNotOperator
	}
	boardName, invalid := newBoardName("name", name)
	if invalid != nil {
		return ChangedBoard{}, input.Errors{invalid}
	}
	rules, err := c.getRules(ctx)
	if err != nil {
		return ChangedBoard{}, err
	}
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedBoard, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return ChangedBoard{}, err
		}
		if err := c.updateName(ctx, id, boardName); err != nil {
			return ChangedBoard{}, err
		}
		registered, _, err := c.decideAndSave(ctx, board.InitialState(), &board.RegisterCommand{ID: id, PublicID: publicID, Name: boardName, Rules: rules.Board, RegisteredBy: registeredBy})
		if err != nil {
			return ChangedBoard{}, err
		}
		return changedBoard(registered), nil
	})
	if errors.Is(err, board.ErrNameAlreadyInUse) {
		return ChangedBoard{}, ErrBoardNameAlreadyInUse
	}
	return registered, err
}

// revision は画面で見たボードの版。無い・削除したなら ErrBoardNotFound、画面を開いた後にほかの操作で変わっていれば
// ErrRevisionConflict、使われている名前なら ErrBoardNameAlreadyInUse、準備中でなければ ErrBoardNotDraft
func (c *BoardHandler) Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (ChangedBoard, error) {
	renamedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedBoard{}, ErrNotOperator
	}
	boardName, invalid := newBoardName("name", name)
	if invalid != nil {
		return ChangedBoard{}, input.Errors{invalid}
	}
	renamed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedBoard, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedBoard{}, err
		}
		if current.Status != board.StatusDraft {
			return ChangedBoard{}, board.ErrNotDraft
		}
		if err := c.updateName(ctx, current.ID, boardName); err != nil {
			return ChangedBoard{}, err
		}
		renamed, _, err := c.decideAndSave(ctx, current, &board.RenameCommand{Name: boardName, RenamedBy: renamedBy})
		if err != nil {
			return ChangedBoard{}, err
		}
		return changedBoard(renamed), nil
	})
	if errors.Is(err, board.ErrNameAlreadyInUse) {
		return ChangedBoard{}, ErrBoardNameAlreadyInUse
	}
	return renamed, mapBoardError(err)
}

// 変えた後のレイアウトの全体を受け取る。入力の誤りは input.Errors
// （項目は width・height・tiles.N.x・tiles.N.y・tiles.N.kind・tiles.N・connections.N.a・connections.N.b・connections.N）。
// 幅・高さが studio_rules の範囲外なら、格子の範囲が決まらないため、マス・つながりは調べない。ほかのエラーは Rename と同じ
func (c *BoardHandler) ChangeLayout(ctx context.Context, id uuid.UUID, revision int64, layoutInput BoardLayoutInput) (ChangedBoard, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedBoard{}, ErrNotOperator
	}
	rules, err := c.getRules(ctx)
	if err != nil {
		return ChangedBoard{}, err
	}
	layout, err := newLayout(rules.Board, layoutInput)
	if err != nil {
		return ChangedBoard{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedBoard, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedBoard{}, err
		}
		changed, _, err := c.decideAndSave(ctx, current, &board.ChangeLayoutCommand{Layout: layout, Rules: rules.Board, ChangedBy: changedBy})
		if err != nil {
			return ChangedBoard{}, err
		}
		return changedBoard(changed), nil
	})
	return changed, mapBoardError(err)
}

// 公開中にする（使用不可からの公開に戻すのも同じ）。公開の条件を満たさなければ *BoardNotPublishableError。
// すでに公開中なら、何も確かめず変えずに成功する（revision が古ければ ErrRevisionConflict）。ほかのエラーは Rename と同じ
func (c *BoardHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedBoard, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedBoard{}, ErrNotOperator
	}
	published, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedBoard, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedBoard{}, err
		}
		published, _, err := c.decideAndSave(ctx, current, &board.PublishCommand{PublishedBy: publishedBy})
		if err != nil {
			return ChangedBoard{}, err
		}
		return changedBoard(published), nil
	})
	return published, mapBoardError(err)
}

// 使用不可にする。すでに使用不可なら変えずに成功する。準備中なら ErrInvalidBoardStatusTransition。ほかのエラーは Rename と同じ
func (c *BoardHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedBoard, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedBoard{}, ErrNotOperator
	}
	retired, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedBoard, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedBoard{}, err
		}
		retired, _, err := c.decideAndSave(ctx, current, &board.RetireCommand{RetiredBy: retiredBy})
		if err != nil {
			return ChangedBoard{}, err
		}
		return changedBoard(retired), nil
	})
	return retired, mapBoardError(err)
}

// 準備中のものだけ消せる。削除のイベントと一緒に識別子を消し、名前を使えるようにする。準備中でなければ ErrBoardNotDraft。
// ほかのエラーは Rename と同じ
func (c *BoardHandler) Delete(ctx context.Context, id uuid.UUID, revision int64) error {
	deletedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ErrNotOperator
	}
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return struct{}{}, err
		}
		if _, _, err := c.decideAndSave(ctx, current, &board.DeleteCommand{DeletedBy: deletedBy}); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.identifier.Delete(ctx, current.ID)
	})
	return mapBoardError(err)
}

// revision を Load の直後に比べる理由は PresetDeckHandler.load と同じ
func (c *BoardHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (board.Board, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return board.Board{}, ErrBoardNotFound
	}
	if err != nil {
		return board.Board{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return board.Board{}, err
	}
	current, ok := state.(board.Board)
	if !ok {
		return board.Board{}, ErrBoardNotFound
	}
	if current.Version != revision {
		return board.Board{}, ErrRevisionConflict
	}
	return current, nil
}

// 変わった（イベントを作った）かも返す。状態を変えない操作（同じ名前・同じレイアウト・公開中にもう一度公開する等）は版が進まない
func (c *BoardHandler) decideAndSave(ctx context.Context, state board.State, cmd board.Command) (board.Board, bool, error) {
	events, err := board.Decide(state, cmd, c.now())
	if err != nil {
		return board.Board{}, false, err
	}
	if err := c.eventStore.Save(ctx, events); err != nil {
		return board.Board{}, false, err
	}
	for _, event := range events {
		if state, err = board.Evolve(state, event); err != nil {
			return board.Board{}, false, err
		}
	}
	current, _ := state.(board.Board)
	return current, len(events) > 0, nil
}

func changedBoard(current board.Board) ChangedBoard {
	return ChangedBoard{ID: current.PublicID, Revision: current.Version}
}

func newBoardName(field, value string) (board.Name, input.Error) {
	name, err := board.NewName(value)
	switch {
	case errors.Is(err, board.ErrNameEmpty):
		return "", &input.Required{Field: field}
	case errors.Is(err, board.ErrNameTooLong):
		return "", &input.TooLong{Field: field, Max: board.MaxNameLength}
	case err != nil:
		return "", &input.InvalidFormat{Field: field}
	}
	return name, nil
}

// 幅・高さ → マスの種類 → ValidateLayout の順に確かめ、誤りはまとめて input.Errors で返す。レイアウトの決まりは ValidateLayout の
// 1 か所にあり、ここはその違反を入力の項目のパスに写すだけ。幅・高さの範囲は studio_rules（rules）の値
func newLayout(rules studiorules.Board, layoutInput BoardLayoutInput) (board.Layout, error) {
	invalid := input.Errors{}
	if !rules.Contains(layoutInput.Width) {
		invalid = append(invalid, &input.OutOfRange{Field: "width", Min: rules.MinSize, Max: rules.MaxSize})
	}
	if !rules.Contains(layoutInput.Height) {
		invalid = append(invalid, &input.OutOfRange{Field: "height", Min: rules.MinSize, Max: rules.MaxSize})
	}
	if len(invalid) > 0 {
		return board.Layout{}, invalid
	}
	// 範囲の下限は 1 以上（studiorules.New）なので、範囲の中なら失敗しない。失敗するならコードの誤りなので、内部のエラーのまま返す
	size, err := board.NewSize(layoutInput.Width, layoutInput.Height)
	if err != nil {
		return board.Layout{}, err
	}

	tiles := make([]board.Tile, len(layoutInput.Tiles))
	for i, tile := range layoutInput.Tiles {
		kind, err := board.NewTileKind(tile.Kind)
		if err != nil {
			invalid = append(invalid, &input.InvalidFormat{Field: fmt.Sprintf("tiles.%d.kind", i)})
			// 種類の誤りとは別に、座標の誤りも示せるよう、仮の種類で検証を続ける
			kind = board.TileKindLand
		}
		tiles[i] = board.Tile{Position: board.NewPosition(tile.X, tile.Y), Kind: kind}
	}
	connections := make([]board.Connection, len(layoutInput.Connections))
	for i, connection := range layoutInput.Connections {
		connections[i] = board.NewConnection(board.NewPosition(connection.A.X, connection.A.Y), board.NewPosition(connection.B.X, connection.B.Y))
	}

	for _, violation := range board.ValidateLayout(size, tiles, connections) {
		invalid = append(invalid, layoutViolationToInput(violation, size, layoutInput))
	}
	if len(invalid) > 0 {
		return board.Layout{}, invalid
	}
	// ValidateLayout が通ったのに失敗するならコードの誤りなので、入力の誤りにせず内部のエラーのまま返す
	return board.NewLayout(size, tiles, connections)
}

func layoutViolationToInput(violation board.LayoutViolation, size board.Size, layoutInput BoardLayoutInput) input.Error {
	switch violation.Subject {
	case board.SubjectTile:
		field := fmt.Sprintf("tiles.%d", violation.Index)
		switch violation.Reason {
		case board.ReasonOutOfRange:
			if violation.Part == board.PartX {
				return &input.OutOfRange{Field: field + ".x", Min: 0, Max: size.Width() - 1}
			}
			return &input.OutOfRange{Field: field + ".y", Min: 0, Max: size.Height() - 1}
		case board.ReasonDuplicatedTile:
			return &input.Duplicated{Field: field}
		}
	case board.SubjectConnection:
		field := fmt.Sprintf("connections.%d", violation.Index)
		switch violation.Reason {
		case board.ReasonTileNotFound:
			return &input.NotFound{Field: field + "." + string(inputEnd(violation.Part, layoutInput.Connections[violation.Index]))}
		case board.ReasonNotAdjacent:
			return &input.NotAdjacent{Field: field}
		case board.ReasonDuplicatedConnection:
			return &input.Duplicated{Field: field}
		}
	}
	// 理由を domain に足したのに、ここに写し方を足し忘れた（黙って別の誤りにしない）
	panic(fmt.Sprintf("unknown layout violation: %+v", violation))
}

// ValidateLayout の a・b は並べ方の小さい方が a にそろった後のもの。入力の a・b が入れ替わっていれば、入力の側に戻す
func inputEnd(part board.Part, connection BoardConnectionInput) board.Part {
	a := board.NewPosition(connection.A.X, connection.A.Y)
	b := board.NewPosition(connection.B.X, connection.B.Y)
	if a.Compare(b) <= 0 {
		return part
	}
	if part == board.PartA {
		return board.PartB
	}
	return board.PartA
}

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変える
func mapBoardError(err error) error {
	var notPublishable *board.NotPublishableError
	switch {
	case errors.As(err, &notPublishable):
		unreachable := make([]BoardPosition, 0, len(notPublishable.Readiness.UnreachableTiles))
		for _, pos := range notPublishable.Readiness.UnreachableTiles {
			unreachable = append(unreachable, BoardPosition{X: pos.X, Y: pos.Y})
		}
		return &BoardNotPublishableError{
			BaseCount: notPublishable.Readiness.BaseCount, CheckpointCount: notPublishable.Readiness.CheckpointCount, UnreachableTiles: unreachable,
		}
	case errors.Is(err, board.ErrNotDraft):
		return ErrBoardNotDraft
	case errors.Is(err, board.ErrInvalidStatusTransition):
		return ErrInvalidBoardStatusTransition
	}
	return revisionConflict(err)
}
