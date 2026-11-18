package command

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type NPCAvatarHandler struct {
	eventStore domain.EventStore[npcavatar.ID, npcavatar.Event, npcavatar.State]
	identifier domain.Identifier[npcavatar.ID]
	entityRefs domain.EntityRefResolver
	updateName npcavatar.UpdateNameFunc
	// 使うデッキを参照する（参照されている間はデッキの識別子を消せない）
	updateDeckReferences npcavatar.UpdateDeckReferencesFunc
	deckStatuses         NPCDeckStatusesFunc
	// 使う思考パターンも同じ作りで参照する
	updateMindsetReferences npcavatar.UpdateMindsetReferencesFunc
	mindsetStatuses         NPCMindsetStatusesFunc
	now                     func() time.Time
}

func NewNPCAvatarHandler(
	eventStore domain.EventStore[npcavatar.ID, npcavatar.Event, npcavatar.State],
	identifier domain.Identifier[npcavatar.ID],
	entityRefs domain.EntityRefResolver,
	updateName npcavatar.UpdateNameFunc,
	updateDeckReferences npcavatar.UpdateDeckReferencesFunc,
	deckStatuses NPCDeckStatusesFunc,
	updateMindsetReferences npcavatar.UpdateMindsetReferencesFunc,
	mindsetStatuses NPCMindsetStatusesFunc,
	now func() time.Time,
) *NPCAvatarHandler {
	return &NPCAvatarHandler{
		eventStore: eventStore, identifier: identifier, entityRefs: entityRefs, updateName: updateName,
		updateDeckReferences: updateDeckReferences, deckStatuses: deckStatuses,
		updateMindsetReferences: updateMindsetReferences, mindsetStatuses: mindsetStatuses, now: now,
	}
}

// NPC のデッキの状態を返す。無い（削除したものを含む）デッキは結果に含めない
type NPCDeckStatusesFunc func(ctx context.Context, ids []npcdeck.ID) (map[npcdeck.ID]npcdeck.Status, error)

// NPC のデッキの状態を、集約をイベントストアで読んで確かめる。read model（npc_decks）を読まないのは、投影が遅れている間に
// 古い状態で判断しないため（CardStatusReader と同じ）。読むだけで書かない
type NPCDeckStatusReader struct {
	eventStore domain.EventStore[npcdeck.ID, npcdeck.Event, npcdeck.State]
}

func NewNPCDeckStatusReader(eventStore domain.EventStore[npcdeck.ID, npcdeck.Event, npcdeck.State]) *NPCDeckStatusReader {
	return &NPCDeckStatusReader{eventStore: eventStore}
}

func (r *NPCDeckStatusReader) Statuses(ctx context.Context, ids []npcdeck.ID) (map[npcdeck.ID]npcdeck.Status, error) {
	statuses := make(map[npcdeck.ID]npcdeck.Status, len(ids))
	for _, id := range ids {
		state, err := r.eventStore.Load(ctx, id)
		if err != nil {
			return nil, err
		}
		if loaded, ok := state.(npcdeck.NPCDeck); ok {
			statuses[id] = loaded.Status
		}
	}
	return statuses, nil
}

// NPC の思考パターンの状態を返す。無い（削除したものを含む）思考パターンは結果に含めない
type NPCMindsetStatusesFunc func(ctx context.Context, ids []npcmindset.ID) (map[npcmindset.ID]npcmindset.Status, error)

// NPC の思考パターンの状態を、NPCDeckStatusReader と同じ理由でイベントストアで読んで確かめる。読むだけで書かない
type NPCMindsetStatusReader struct {
	eventStore domain.EventStore[npcmindset.ID, npcmindset.Event, npcmindset.State]
}

func NewNPCMindsetStatusReader(eventStore domain.EventStore[npcmindset.ID, npcmindset.Event, npcmindset.State]) *NPCMindsetStatusReader {
	return &NPCMindsetStatusReader{eventStore: eventStore}
}

func (r *NPCMindsetStatusReader) Statuses(ctx context.Context, ids []npcmindset.ID) (map[npcmindset.ID]npcmindset.Status, error) {
	statuses := make(map[npcmindset.ID]npcmindset.Status, len(ids))
	for _, id := range ids {
		state, err := r.eventStore.Load(ctx, id)
		if err != nil {
			return nil, err
		}
		if loaded, ok := state.(npcmindset.NPCMindset); ok {
			statuses[id] = loaded.Status
		}
	}
	return statuses, nil
}

// 変える操作の結果は、公開 ID と新しい revision だけにする（理由は ChangedPresetDeck と同じ）
type ChangedNPCAvatar struct {
	ID       uuid.UUID
	Revision int64
}

// 準備中の NPC のアバターを作る。操作した人は context のオペレーター。
// 入力の誤りは input.Errors、使われている名前（状態によらず、すべての NPC のアバターの中で）なら ErrNPCAvatarNameAlreadyInUse
func (c *NPCAvatarHandler) Register(ctx context.Context, name string) (ChangedNPCAvatar, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCAvatar{}, ErrNotOperator
	}
	npcAvatarName, invalid := newNPCAvatarName("name", name)
	if invalid != nil {
		return ChangedNPCAvatar{}, input.Errors{invalid}
	}
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCAvatar, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		if err := c.updateName(ctx, id, npcAvatarName); err != nil {
			return ChangedNPCAvatar{}, err
		}
		registered, err := c.decideAndSave(ctx, npcavatar.InitialState(), &npcavatar.RegisterCommand{ID: id, PublicID: publicID, Name: npcAvatarName, RegisteredBy: registeredBy})
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		return changedNPCAvatar(registered), nil
	})
	if errors.Is(err, npcavatar.ErrNameAlreadyInUse) {
		return ChangedNPCAvatar{}, ErrNPCAvatarNameAlreadyInUse
	}
	return registered, err
}

// revision は画面で見た NPC のアバターの版。無い・削除したなら ErrNPCAvatarNotFound、画面を開いた後にほかの操作で変わっていれば
// ErrRevisionConflict、使われている名前なら ErrNPCAvatarNameAlreadyInUse、準備中でなければ ErrNPCAvatarNotDraft
func (c *NPCAvatarHandler) Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (ChangedNPCAvatar, error) {
	renamedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCAvatar{}, ErrNotOperator
	}
	npcAvatarName, invalid := newNPCAvatarName("name", name)
	if invalid != nil {
		return ChangedNPCAvatar{}, input.Errors{invalid}
	}
	renamed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCAvatar, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		if current.Status != npcavatar.StatusDraft {
			return ChangedNPCAvatar{}, npcavatar.ErrNotDraft
		}
		if err := c.updateName(ctx, current.ID, npcAvatarName); err != nil {
			return ChangedNPCAvatar{}, err
		}
		renamed, err := c.decideAndSave(ctx, current, &npcavatar.RenameCommand{Name: npcAvatarName, RenamedBy: renamedBy})
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		return changedNPCAvatar(renamed), nil
	})
	if errors.Is(err, npcavatar.ErrNameAlreadyInUse) {
		return ChangedNPCAvatar{}, ErrNPCAvatarNameAlreadyInUse
	}
	return renamed, mapNPCAvatarError(err)
}

// 使うデッキの一覧を deckIDs（NPC のデッキの公開 ID）に置き換える。使えるのは公開中のデッキだけで、一覧は 0 件でもよい（準備中のあいだ）。
// 入力の誤りは、まとめて input.Errors で返す。項目名は deckIds.<番号>。同じデッキの 2 つ目以降は input.Duplicated、
// 無い・削除した・NPC のデッキでないものは input.NotFound、新しく入れるもので公開中でないものは input.Unavailable。
// 件数の上限は持たない（カードの枚数のような、決まりから導ける値が無く、新しい決まりをここで作らないため）。
// 準備中でなければ、公開中かの確かめより先に ErrNPCAvatarNotDraft。同じデッキの集まりなら（順番だけ違っても）変えずに成功する。
// ほかのエラーは Rename と同じ
func (c *NPCAvatarHandler) SetDecks(ctx context.Context, id uuid.UUID, revision int64, deckIDs []uuid.UUID) (ChangedNPCAvatar, error) {
	assignedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCAvatar{}, ErrNotOperator
	}
	decks, err := c.parseDecks(ctx, deckIDs)
	if err != nil {
		return ChangedNPCAvatar{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCAvatar, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		if current.Status != npcavatar.StatusDraft {
			return ChangedNPCAvatar{}, npcavatar.ErrNotDraft
		}
		if err := c.requirePublished(ctx, current.Decks, decks); err != nil {
			return ChangedNPCAvatar{}, err
		}
		changed, err := c.decideAndSave(ctx, current, &npcavatar.SetDecksCommand{Decks: decks, ChangedBy: assignedBy})
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		if err := c.updateDeckReferences(ctx, changed.ID, changed.Decks); err != nil {
			return ChangedNPCAvatar{}, err
		}
		return changedNPCAvatar(changed), nil
	})
	return changed, mapNPCAvatarError(err)
}

// 公開 ID を内部 ID にする。誤りは最初の 1 つで止めず、まとめて返す（SetCardCounts と同じ）
func (c *NPCAvatarHandler) parseDecks(ctx context.Context, deckIDs []uuid.UUID) ([]npcdeck.ID, error) {
	refs, err := c.entityRefs.ResolveAll(ctx, deckIDs)
	if err != nil {
		return nil, err
	}
	invalid := input.Errors{}
	decks := make([]npcdeck.ID, 0, len(deckIDs))
	seen := make(map[uuid.UUID]bool, len(deckIDs))
	for i, deckID := range deckIDs {
		field := fmt.Sprintf("deckIds.%d", i)
		if seen[deckID] {
			invalid = append(invalid, &input.Duplicated{Field: field})
		}
		seen[deckID] = true
		// 行が無い・NPC のデッキでない、はどちらも、指したデッキが無いとして扱う
		ref, found := refs[deckID]
		if !found || ref.EntityType() != npcdeck.Types().EntityType {
			invalid = append(invalid, &input.NotFound{Field: field})
			continue
		}
		deck, err := npcdeck.NewID(ref.ID())
		if err != nil {
			return nil, err
		}
		decks = append(decks, deck)
	}
	if len(invalid) > 0 {
		return nil, invalid
	}
	return decks, nil
}

// 新しく入れるデッキ（今の一覧に無いもの）が公開中か確かめる。位置は requested の並び（deckIds の番号）と同じ。
// 今ある（その後に使用不可にした）デッキは確かめない（外さずに、ほかのデッキだけを変えられるように。公開のときに確かめるため）。
// デッキの状態を読むのと保存の間に使用不可にされる競合は許す（公開のときにもう一度確かめるため）
func (c *NPCAvatarHandler) requirePublished(ctx context.Context, current, requested []npcdeck.ID) error {
	statuses, err := c.deckStatuses(ctx, requested)
	if err != nil {
		return err
	}
	invalid := input.Errors{}
	for i, deck := range requested {
		if statuses[deck] != npcdeck.StatusPublished && !slices.Contains(current, deck) {
			invalid = append(invalid, &input.Unavailable{Field: fmt.Sprintf("deckIds.%d", i)})
		}
	}
	if len(invalid) > 0 {
		return invalid
	}
	return nil
}

// 使う思考パターンの一覧を mindsetIDs（NPC の思考パターンの公開 ID）に置き換える。SetDecks と同じ作りと誤りで、項目名は mindsetIds.<番号>
// （同じものの 2 つ目以降は input.Duplicated、無い・削除した・NPC の思考パターンでないものは input.NotFound、新しく入れるもので
// 公開中でないものは input.Unavailable）。0 件でもよい（アバターの公開に割り当ては要らない）。準備中でなければ ErrNPCAvatarNotDraft。
// 同じ集まりなら（順番だけ違っても）変えずに成功する。ほかのエラーは Rename と同じ
func (c *NPCAvatarHandler) SetMindsets(ctx context.Context, id uuid.UUID, revision int64, mindsetIDs []uuid.UUID) (ChangedNPCAvatar, error) {
	assignedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCAvatar{}, ErrNotOperator
	}
	mindsets, err := c.parseMindsets(ctx, mindsetIDs)
	if err != nil {
		return ChangedNPCAvatar{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCAvatar, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		if current.Status != npcavatar.StatusDraft {
			return ChangedNPCAvatar{}, npcavatar.ErrNotDraft
		}
		if err := c.requirePublishedMindsets(ctx, current.Mindsets, mindsets); err != nil {
			return ChangedNPCAvatar{}, err
		}
		changed, err := c.decideAndSave(ctx, current, &npcavatar.SetMindsetsCommand{Mindsets: mindsets, ChangedBy: assignedBy})
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		if err := c.updateMindsetReferences(ctx, changed.ID, changed.Mindsets); err != nil {
			return ChangedNPCAvatar{}, err
		}
		return changedNPCAvatar(changed), nil
	})
	return changed, mapNPCAvatarError(err)
}

// 公開 ID を内部 ID にする。誤りは最初の 1 つで止めず、まとめて返す（parseDecks と同じ）
func (c *NPCAvatarHandler) parseMindsets(ctx context.Context, mindsetIDs []uuid.UUID) ([]npcmindset.ID, error) {
	refs, err := c.entityRefs.ResolveAll(ctx, mindsetIDs)
	if err != nil {
		return nil, err
	}
	invalid := input.Errors{}
	mindsets := make([]npcmindset.ID, 0, len(mindsetIDs))
	seen := make(map[uuid.UUID]bool, len(mindsetIDs))
	for i, mindsetID := range mindsetIDs {
		field := fmt.Sprintf("mindsetIds.%d", i)
		if seen[mindsetID] {
			invalid = append(invalid, &input.Duplicated{Field: field})
		}
		seen[mindsetID] = true
		ref, found := refs[mindsetID]
		if !found || ref.EntityType() != npcmindset.Types().EntityType {
			invalid = append(invalid, &input.NotFound{Field: field})
			continue
		}
		mindset, err := npcmindset.NewID(ref.ID())
		if err != nil {
			return nil, err
		}
		mindsets = append(mindsets, mindset)
	}
	if len(invalid) > 0 {
		return nil, invalid
	}
	return mindsets, nil
}

// 新しく入れる思考パターン（今の一覧に無いもの）が公開中か確かめる。確かめない範囲と競合を許す理由は requirePublished と同じ
func (c *NPCAvatarHandler) requirePublishedMindsets(ctx context.Context, current, requested []npcmindset.ID) error {
	statuses, err := c.mindsetStatuses(ctx, requested)
	if err != nil {
		return err
	}
	invalid := input.Errors{}
	for i, mindset := range requested {
		if statuses[mindset] != npcmindset.StatusPublished && !slices.Contains(current, mindset) {
			invalid = append(invalid, &input.Unavailable{Field: fmt.Sprintf("mindsetIds.%d", i)})
		}
	}
	if len(invalid) > 0 {
		return invalid
	}
	return nil
}

// 公開中にする（使用不可からの公開に戻すのも同じ）。公開するとき確かめるのは、使うデッキが 1 つ以上あること
// （無ければ input.Required。項目名は decks）と、そのうち 1 つ以上が今も公開中であること（無ければ input.Unavailable）。
// 公開中でないデッキが混ざっていてもよい（使用不可にしたデッキは外さなくてよく、公開中のデッキが残っていればよいため）。
// すでに公開中なら、何も確かめず変えずに成功する（revision が古ければ ErrRevisionConflict）。ほかのエラーは Rename と同じ
func (c *NPCAvatarHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedNPCAvatar, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCAvatar{}, ErrNotOperator
	}
	published, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCAvatar, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		_, changing, err := current.Status.Publish()
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		if !changing {
			return changedNPCAvatar(current), nil
		}
		if err := c.checkPublishable(ctx, current); err != nil {
			return ChangedNPCAvatar{}, err
		}
		published, err := c.decideAndSave(ctx, current, &npcavatar.PublishCommand{PublishedBy: publishedBy})
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		return changedNPCAvatar(published), nil
	})
	return published, mapNPCAvatarError(err)
}

// デッキの状態を読むのと保存の間の競合を許す理由は requirePublished と同じ
func (c *NPCAvatarHandler) checkPublishable(ctx context.Context, current npcavatar.NPCAvatar) error {
	if len(current.Decks) == 0 {
		return input.Errors{&input.Required{Field: "decks"}}
	}
	statuses, err := c.deckStatuses(ctx, current.Decks)
	if err != nil {
		return err
	}
	for _, deck := range current.Decks {
		if statuses[deck] == npcdeck.StatusPublished {
			return nil
		}
	}
	return input.Errors{&input.Unavailable{Field: "decks"}}
}

// 使用不可にする。すでに使用不可なら変えずに成功する。準備中なら ErrInvalidNPCAvatarStatusTransition。ほかのエラーは Rename と同じ
func (c *NPCAvatarHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedNPCAvatar, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCAvatar{}, ErrNotOperator
	}
	retired, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCAvatar, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		retired, err := c.decideAndSave(ctx, current, &npcavatar.RetireCommand{RetiredBy: retiredBy})
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		return changedNPCAvatar(retired), nil
	})
	return retired, mapNPCAvatarError(err)
}

// 準備中のものだけ消せる。削除のイベントと一緒に識別子を消し、名前を使えるようにする。準備中でなければ ErrNPCAvatarNotDraft。
// ほかのエラーは Rename と同じ
func (c *NPCAvatarHandler) Delete(ctx context.Context, id uuid.UUID, revision int64) error {
	deletedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ErrNotOperator
	}
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		current, err := c.load(ctx, id, revision)
		if err != nil {
			return struct{}{}, err
		}
		if _, err := c.decideAndSave(ctx, current, &npcavatar.DeleteCommand{DeletedBy: deletedBy}); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.identifier.Delete(ctx, current.ID)
	})
	return mapNPCAvatarError(err)
}

// revision を Load の直後に比べる理由は PresetDeckHandler.load と同じ
func (c *NPCAvatarHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (npcavatar.NPCAvatar, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return npcavatar.NPCAvatar{}, ErrNPCAvatarNotFound
	}
	if err != nil {
		return npcavatar.NPCAvatar{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return npcavatar.NPCAvatar{}, err
	}
	current, ok := state.(npcavatar.NPCAvatar)
	if !ok {
		return npcavatar.NPCAvatar{}, ErrNPCAvatarNotFound
	}
	if current.Version != revision {
		return npcavatar.NPCAvatar{}, ErrRevisionConflict
	}
	return current, nil
}

// 状態を変えない操作（同じ名前・公開中にもう一度公開する等）は版が進まない
func (c *NPCAvatarHandler) decideAndSave(ctx context.Context, state npcavatar.State, cmd npcavatar.Command) (npcavatar.NPCAvatar, error) {
	events, err := npcavatar.Decide(state, cmd, c.now())
	if err != nil {
		return npcavatar.NPCAvatar{}, err
	}
	if err := c.eventStore.Save(ctx, events); err != nil {
		return npcavatar.NPCAvatar{}, err
	}
	for _, event := range events {
		if state, err = npcavatar.Evolve(state, event); err != nil {
			return npcavatar.NPCAvatar{}, err
		}
	}
	current, _ := state.(npcavatar.NPCAvatar)
	return current, nil
}

func changedNPCAvatar(current npcavatar.NPCAvatar) ChangedNPCAvatar {
	return ChangedNPCAvatar{ID: current.PublicID, Revision: current.Version}
}

func newNPCAvatarName(field, value string) (npcavatar.Name, input.Error) {
	name, err := npcavatar.NewName(value)
	switch {
	case errors.Is(err, npcavatar.ErrNameEmpty):
		return "", &input.Required{Field: field}
	case errors.Is(err, npcavatar.ErrNameTooLong):
		return "", &input.TooLong{Field: field, Max: npcavatar.MaxNameLength}
	case err != nil:
		return "", &input.InvalidFormat{Field: field}
	}
	return name, nil
}

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変える
func mapNPCAvatarError(err error) error {
	switch {
	case errors.Is(err, npcavatar.ErrNotDraft):
		return ErrNPCAvatarNotDraft
	case errors.Is(err, npcavatar.ErrInvalidStatusTransition):
		return ErrInvalidNPCAvatarStatusTransition
	}
	return revisionConflict(err)
}
