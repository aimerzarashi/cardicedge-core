package command

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type NPCAvatarHandler struct {
	eventStore domain.EventStore[npcavatar.ID, npcavatar.Event, npcavatar.State]
	identifier domain.Identifier[npcavatar.ID]
	updateName npcavatar.UpdateNameFunc
	now        func() time.Time
}

func NewNPCAvatarHandler(
	eventStore domain.EventStore[npcavatar.ID, npcavatar.Event, npcavatar.State],
	identifier domain.Identifier[npcavatar.ID],
	updateName npcavatar.UpdateNameFunc,
	now func() time.Time,
) *NPCAvatarHandler {
	return &NPCAvatarHandler{eventStore: eventStore, identifier: identifier, updateName: updateName, now: now}
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

// 公開中にする（使用不可からの公開に戻すのも同じ）。すでに公開中なら変えずに成功する（revision が古ければ ErrRevisionConflict）。
// ほかのエラーは Rename と同じ
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
		published, err := c.decideAndSave(ctx, current, &npcavatar.PublishCommand{PublishedBy: publishedBy})
		if err != nil {
			return ChangedNPCAvatar{}, err
		}
		return changedNPCAvatar(published), nil
	})
	return published, mapNPCAvatarError(err)
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
