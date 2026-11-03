package command

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type StarterSetHandler struct {
	eventStore domain.EventStore[starterset.ID, starterset.Event, starterset.State]
	identifier domain.Identifier[starterset.ID]
	entityRefs domain.EntityRefResolver
	updateName starterset.UpdateNameFunc
	// 配るデッキ・解放するボードを参照する（参照されている間は識別子を消せない）
	updateGrantDeckReferences starterset.UpdateGrantDeckReferencesFunc
	updateBoardReferences     starterset.UpdateBoardReferencesFunc
	grantDeckStatuses         GrantDeckStatusesFunc
	boardStatuses             BoardStatusesFunc
	// 公開中の数の持ち主（ゲーム全体のルールの集約）を、一意な key から引く
	findGameRules gamerules.FindFunc
	reserveQuota  starterset.ReserveQuotaFunc
	releaseQuota  starterset.ReleaseQuotaFunc
	now           func() time.Time
}

func NewStarterSetHandler(
	eventStore domain.EventStore[starterset.ID, starterset.Event, starterset.State],
	identifier domain.Identifier[starterset.ID],
	entityRefs domain.EntityRefResolver,
	updateName starterset.UpdateNameFunc,
	updateGrantDeckReferences starterset.UpdateGrantDeckReferencesFunc,
	updateBoardReferences starterset.UpdateBoardReferencesFunc,
	grantDeckStatuses GrantDeckStatusesFunc,
	boardStatuses BoardStatusesFunc,
	findGameRules gamerules.FindFunc,
	reserveQuota starterset.ReserveQuotaFunc,
	releaseQuota starterset.ReleaseQuotaFunc,
	now func() time.Time,
) *StarterSetHandler {
	return &StarterSetHandler{
		eventStore: eventStore, identifier: identifier, entityRefs: entityRefs, updateName: updateName,
		updateGrantDeckReferences: updateGrantDeckReferences, updateBoardReferences: updateBoardReferences,
		grantDeckStatuses: grantDeckStatuses, boardStatuses: boardStatuses,
		findGameRules: findGameRules, reserveQuota: reserveQuota, releaseQuota: releaseQuota, now: now,
	}
}

// 配布デッキの状態を返す。無い（削除したものを含む）デッキは結果に含めない
type GrantDeckStatusesFunc func(ctx context.Context, ids []grantdeck.ID) (map[grantdeck.ID]grantdeck.Status, error)

// 配布デッキの状態を、集約をイベントストアで読んで確かめる。read model（grant_decks）を読まないのは、投影が遅れている間に
// 古い状態で判断しないため（NPCDeckStatusReader と同じ）。読むだけで書かない
type GrantDeckStatusReader struct {
	eventStore domain.EventStore[grantdeck.ID, grantdeck.Event, grantdeck.State]
}

func NewGrantDeckStatusReader(eventStore domain.EventStore[grantdeck.ID, grantdeck.Event, grantdeck.State]) *GrantDeckStatusReader {
	return &GrantDeckStatusReader{eventStore: eventStore}
}

func (r *GrantDeckStatusReader) Statuses(ctx context.Context, ids []grantdeck.ID) (map[grantdeck.ID]grantdeck.Status, error) {
	statuses := make(map[grantdeck.ID]grantdeck.Status, len(ids))
	for _, id := range ids {
		state, err := r.eventStore.Load(ctx, id)
		if err != nil {
			return nil, err
		}
		if loaded, ok := state.(grantdeck.GrantDeck); ok {
			statuses[id] = loaded.Status
		}
	}
	return statuses, nil
}

// ボードの状態を返す。無い（削除したものを含む）ボードは結果に含めない
type BoardStatusesFunc func(ctx context.Context, ids []board.ID) (map[board.ID]board.Status, error)

// ボードの状態を、GrantDeckStatusReader と同じ理由でイベントストアから読む
type BoardStatusReader struct {
	eventStore domain.EventStore[board.ID, board.Event, board.State]
}

func NewBoardStatusReader(eventStore domain.EventStore[board.ID, board.Event, board.State]) *BoardStatusReader {
	return &BoardStatusReader{eventStore: eventStore}
}

func (r *BoardStatusReader) Statuses(ctx context.Context, ids []board.ID) (map[board.ID]board.Status, error) {
	statuses := make(map[board.ID]board.Status, len(ids))
	for _, id := range ids {
		state, err := r.eventStore.Load(ctx, id)
		if err != nil {
			return nil, err
		}
		if loaded, ok := state.(board.Board); ok {
			statuses[id] = loaded.Status
		}
	}
	return statuses, nil
}

// 変える操作の結果は、公開 ID と新しい revision だけにする（理由は ChangedPresetDeck と同じ）
type ChangedStarterSet struct {
	ID       uuid.UUID
	Revision int64
}

// 準備中のスターターセットを作る。操作した人は context のオペレーター。配るデッキと解放するボードは登録の後に指定する。
// 入力の誤りは input.Errors、使われている名前（状態によらず、すべてのスターターセットの中で）なら ErrStarterSetNameAlreadyInUse
func (c *StarterSetHandler) Register(ctx context.Context, name string) (ChangedStarterSet, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterSet{}, ErrNotOperator
	}
	setName, invalid := newStarterSetName("name", name)
	if invalid != nil {
		return ChangedStarterSet{}, input.Errors{invalid}
	}
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterSet, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return ChangedStarterSet{}, err
		}
		if err := c.updateName(ctx, id, setName); err != nil {
			return ChangedStarterSet{}, err
		}
		registered, _, err := c.decideAndSave(ctx, starterset.InitialState(), &starterset.RegisterCommand{ID: id, PublicID: publicID, Name: setName, RegisteredBy: registeredBy})
		if err != nil {
			return ChangedStarterSet{}, err
		}
		return changedStarterSet(registered), nil
	})
	if errors.Is(err, starterset.ErrNameAlreadyInUse) {
		return ChangedStarterSet{}, ErrStarterSetNameAlreadyInUse
	}
	return registered, err
}

// revision は画面で見たスターターセットの版。無い・削除したなら ErrStarterSetNotFound、画面を開いた後にほかの操作で変わっていれば
// ErrRevisionConflict、使われている名前なら ErrStarterSetNameAlreadyInUse、準備中でなければ ErrStarterSetNotDraft
func (c *StarterSetHandler) Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (ChangedStarterSet, error) {
	renamedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterSet{}, ErrNotOperator
	}
	setName, invalid := newStarterSetName("name", name)
	if invalid != nil {
		return ChangedStarterSet{}, input.Errors{invalid}
	}
	renamed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterSet, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterSet{}, err
		}
		if current.Status != starterset.StatusDraft {
			return ChangedStarterSet{}, starterset.ErrNotDraft
		}
		if err := c.updateName(ctx, current.ID, setName); err != nil {
			return ChangedStarterSet{}, err
		}
		renamed, _, err := c.decideAndSave(ctx, current, &starterset.RenameCommand{Name: setName, RenamedBy: renamedBy})
		if err != nil {
			return ChangedStarterSet{}, err
		}
		return changedStarterSet(renamed), nil
	})
	if errors.Is(err, starterset.ErrNameAlreadyInUse) {
		return ChangedStarterSet{}, ErrStarterSetNameAlreadyInUse
	}
	return renamed, mapStarterSetError(err)
}

// 配るデッキの一覧を grantDeckIDs（配布デッキの公開 ID）に置き換える。使えるのは公開中のデッキだけで、一覧は 0 件でもよい（準備中のあいだ）。
// 入力の誤りは、まとめて input.Errors で返す。項目名は grantDeckIds.<番号>。同じデッキの 2 つ目以降は input.Duplicated、
// 無い・削除した・配布デッキでないものは input.NotFound、新しく入れるもので公開中でないものは input.Unavailable。
// 件数の上限は持たない（決まりから導ける値が無く、新しい決まりをここで作らないため）。
// 準備中でなければ、公開中かの確かめより先に ErrStarterSetNotDraft。同じデッキの集まりなら（順番だけ違っても）変えずに成功する。
// ほかのエラーは Rename と同じ
func (c *StarterSetHandler) SetGrantDecks(ctx context.Context, id uuid.UUID, revision int64, grantDeckIDs []uuid.UUID) (ChangedStarterSet, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterSet{}, ErrNotOperator
	}
	grantDecks, err := parseReferences(ctx, c.entityRefs, grantDeckIDs, "grantDeckIds", grantdeck.Types().EntityType, grantdeck.NewID)
	if err != nil {
		return ChangedStarterSet{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterSet, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterSet{}, err
		}
		if current.Status != starterset.StatusDraft {
			return ChangedStarterSet{}, starterset.ErrNotDraft
		}
		if err := requirePublished(ctx, c.grantDeckStatuses, current.GrantDecks, grantDecks, "grantDeckIds", grantdeck.StatusPublished); err != nil {
			return ChangedStarterSet{}, err
		}
		changed, _, err := c.decideAndSave(ctx, current, &starterset.SetGrantDecksCommand{GrantDecks: grantDecks, ChangedBy: changedBy})
		if err != nil {
			return ChangedStarterSet{}, err
		}
		if err := c.updateGrantDeckReferences(ctx, changed.ID, changed.GrantDecks); err != nil {
			return ChangedStarterSet{}, err
		}
		return changedStarterSet(changed), nil
	})
	return changed, mapStarterSetError(err)
}

// 解放するボードの一覧を boardIDs（ボードの公開 ID）に置き換える。誤りの決まりは SetGrantDecks と同じ（項目名は unlockBoardIds.<番号>。
// ボードでないものは input.NotFound）
func (c *StarterSetHandler) SetUnlockBoards(ctx context.Context, id uuid.UUID, revision int64, boardIDs []uuid.UUID) (ChangedStarterSet, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterSet{}, ErrNotOperator
	}
	boards, err := parseReferences(ctx, c.entityRefs, boardIDs, "unlockBoardIds", board.Types().EntityType, board.NewID)
	if err != nil {
		return ChangedStarterSet{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterSet, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterSet{}, err
		}
		if current.Status != starterset.StatusDraft {
			return ChangedStarterSet{}, starterset.ErrNotDraft
		}
		if err := requirePublished(ctx, c.boardStatuses, current.UnlockBoards, boards, "unlockBoardIds", board.StatusPublished); err != nil {
			return ChangedStarterSet{}, err
		}
		changed, _, err := c.decideAndSave(ctx, current, &starterset.SetUnlockBoardsCommand{UnlockBoards: boards, ChangedBy: changedBy})
		if err != nil {
			return ChangedStarterSet{}, err
		}
		if err := c.updateBoardReferences(ctx, changed.ID, changed.UnlockBoards); err != nil {
			return ChangedStarterSet{}, err
		}
		return changedStarterSet(changed), nil
	})
	return changed, mapStarterSetError(err)
}

// 公開 ID を内部 ID にする。誤りは最初の 1 つで止めず、まとめて返す（NPCAvatarHandler.parseDecks と同じ）
func parseReferences[I domain.ID](ctx context.Context, entityRefs domain.EntityRefResolver, publicIDs []uuid.UUID, field string, entityType domain.EntityType, newID domain.NewIDFunc[I]) ([]I, error) {
	refs, err := entityRefs.ResolveAll(ctx, publicIDs)
	if err != nil {
		return nil, err
	}
	invalid := input.Errors{}
	ids := make([]I, 0, len(publicIDs))
	seen := make(map[uuid.UUID]bool, len(publicIDs))
	for i, publicID := range publicIDs {
		path := fmt.Sprintf("%s.%d", field, i)
		if seen[publicID] {
			invalid = append(invalid, &input.Duplicated{Field: path})
		}
		seen[publicID] = true
		// 行が無い・種類が違う、はどちらも、指したものが無いとして扱う
		ref, found := refs[publicID]
		if !found || ref.EntityType() != entityType {
			invalid = append(invalid, &input.NotFound{Field: path})
			continue
		}
		id, err := newID(ref.ID())
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if len(invalid) > 0 {
		return nil, invalid
	}
	return ids, nil
}

// 新しく入れるもの（今の一覧に無いもの）が公開中か確かめる。位置は requested の並び（公開 ID の一覧の番号）と同じ。
// 今ある（その後に使用不可にした）ものは確かめない（外さずに、ほかのものだけを変えられるように）。
// 状態を読むのと保存の間に使用不可にされる競合は許す（公開中のものは準備中へ戻らず、使用不可は新しく入れないための印のため）
func requirePublished[I comparable, S comparable](ctx context.Context, statuses func(context.Context, []I) (map[I]S, error), current, requested []I, field string, published S) error {
	found, err := statuses(ctx, requested)
	if err != nil {
		return err
	}
	invalid := input.Errors{}
	for i, id := range requested {
		if found[id] != published && !slices.Contains(current, id) {
			invalid = append(invalid, &input.Unavailable{Field: fmt.Sprintf("%s.%d", field, i)})
		}
	}
	if len(invalid) > 0 {
		return invalid
	}
	return nil
}

// 公開中にする（使用不可からの公開に戻すのも同じ）。公開するとき確かめるのは、配るデッキが 1 つ以上あること
// （無ければ input.Required。項目名は grantDecks）と、公開中の数の上限（ErrStarterSetLimitReached。上限の値は LimitReachedError）。
// 解放するボードは 0 件でもよい。すでに公開中なら、何も確かめず変えずに成功する（revision が古ければ ErrRevisionConflict）。
// ほかのエラーは Rename と同じ
func (c *StarterSetHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedStarterSet, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterSet{}, ErrNotOperator
	}
	published, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterSet, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterSet{}, err
		}
		_, changing, err := current.Status.Publish()
		if err != nil {
			return ChangedStarterSet{}, err
		}
		if !changing {
			return changedStarterSet(current), nil
		}
		if len(current.GrantDecks) == 0 {
			return ChangedStarterSet{}, input.Errors{&input.Required{Field: "grantDecks"}}
		}
		if err := c.reservePublished(ctx); err != nil {
			return ChangedStarterSet{}, err
		}
		published, _, err := c.decideAndSave(ctx, current, &starterset.PublishCommand{PublishedBy: publishedBy})
		if err != nil {
			return ChangedStarterSet{}, err
		}
		return changedStarterSet(published), nil
	})
	if errors.Is(err, starterset.ErrLimitReached) {
		return ChangedStarterSet{}, &LimitReachedError{reason: ErrStarterSetLimitReached, Max: starterset.MaxPublished}
	}
	return published, mapStarterSetError(err)
}

// 公開中の数を 1 つ確保する。持ち主は game_rules の内部 ID、上限は starterset.MaxPublished
func (c *StarterSetHandler) reservePublished(ctx context.Context) error {
	gameRulesID, found, err := c.findGameRules(ctx)
	if err != nil {
		return err
	}
	if !found {
		return ErrGameRulesNotCreated
	}
	limit, err := domain.NewQuotaLimit(starterset.MaxPublished)
	if err != nil {
		return err
	}
	return c.reserveQuota(ctx, starterset.PublishedQuota(gameRulesID), limit)
}

// 使用不可にする。公開中の数を 1 つ戻す（入れ替えられるように）。すでに使用不可なら変えずに成功する。
// 準備中なら ErrInvalidStarterSetStatusTransition。ほかのエラーは Rename と同じ
func (c *StarterSetHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedStarterSet, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterSet{}, ErrNotOperator
	}
	retired, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterSet, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterSet{}, err
		}
		retired, changed, err := c.decideAndSave(ctx, current, &starterset.RetireCommand{RetiredBy: retiredBy})
		if err != nil {
			return ChangedStarterSet{}, err
		}
		if changed {
			gameRulesID, found, err := c.findGameRules(ctx)
			if err != nil {
				return ChangedStarterSet{}, err
			}
			if !found {
				return ChangedStarterSet{}, ErrGameRulesNotCreated
			}
			if err := c.releaseQuota(ctx, starterset.PublishedQuota(gameRulesID)); err != nil {
				return ChangedStarterSet{}, err
			}
		}
		return changedStarterSet(retired), nil
	})
	return retired, mapStarterSetError(err)
}

// 準備中のものだけ消せる。削除のイベントと一緒に識別子を消し、名前を使えるようにする。準備中でなければ ErrStarterSetNotDraft。
// ほかのエラーは Rename と同じ
func (c *StarterSetHandler) Delete(ctx context.Context, id uuid.UUID, revision int64) error {
	deletedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ErrNotOperator
	}
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return struct{}{}, err
		}
		if _, _, err := c.decideAndSave(ctx, current, &starterset.DeleteCommand{DeletedBy: deletedBy}); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.identifier.Delete(ctx, current.ID)
	})
	return mapStarterSetError(err)
}

// revision を Load の直後に比べる理由は PresetDeckHandler.load と同じ
func (c *StarterSetHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (starterset.StarterSet, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return starterset.StarterSet{}, ErrStarterSetNotFound
	}
	if err != nil {
		return starterset.StarterSet{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return starterset.StarterSet{}, err
	}
	current, ok := state.(starterset.StarterSet)
	if !ok {
		return starterset.StarterSet{}, ErrStarterSetNotFound
	}
	if current.Version != revision {
		return starterset.StarterSet{}, ErrRevisionConflict
	}
	return current, nil
}

// 変わった（イベントを作った）かも返す。状態を変えない操作（同じ名前・公開中にもう一度公開する等）は版が進まない
func (c *StarterSetHandler) decideAndSave(ctx context.Context, state starterset.State, cmd starterset.Command) (starterset.StarterSet, bool, error) {
	events, err := starterset.Decide(state, cmd, c.now())
	if err != nil {
		return starterset.StarterSet{}, false, err
	}
	if err := c.eventStore.Save(ctx, events); err != nil {
		return starterset.StarterSet{}, false, err
	}
	for _, event := range events {
		if state, err = starterset.Evolve(state, event); err != nil {
			return starterset.StarterSet{}, false, err
		}
	}
	current, _ := state.(starterset.StarterSet)
	return current, len(events) > 0, nil
}

func changedStarterSet(current starterset.StarterSet) ChangedStarterSet {
	return ChangedStarterSet{ID: current.PublicID, Revision: current.Version}
}

func newStarterSetName(field, value string) (starterset.Name, input.Error) {
	name, err := starterset.NewName(value)
	switch {
	case errors.Is(err, starterset.ErrNameEmpty):
		return "", &input.Required{Field: field}
	case errors.Is(err, starterset.ErrNameTooLong):
		return "", &input.TooLong{Field: field, Max: starterset.MaxNameLength}
	case err != nil:
		return "", &input.InvalidFormat{Field: field}
	}
	return name, nil
}

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変える
func mapStarterSetError(err error) error {
	switch {
	case errors.Is(err, starterset.ErrNotDraft):
		return ErrStarterSetNotDraft
	case errors.Is(err, starterset.ErrInvalidStatusTransition):
		return ErrInvalidStarterSetStatusTransition
	}
	return revisionConflict(err)
}
