package command

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type NPCDeckHandler struct {
	eventStore domain.EventStore[npcdeck.ID, npcdeck.Event, npcdeck.State]
	identifier domain.Identifier[npcdeck.ID]
	entityRefs domain.EntityRefResolver
	updateName npcdeck.UpdateNameFunc
	// 入れたカードを参照する（参照されている間はカードの識別子を消せない）
	updateCardReferences npcdeck.UpdateCardReferencesFunc
	cardStatuses         CardStatusesFunc
	// 写す元のプリセットデッキを、公開 ID から引いて Load する
	presetDecks          domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State]
	presetDeckIdentifier domain.Identifier[presetdeck.ID]
	getRules             GetGameRulesFunc
	now                  func() time.Time
}

func NewNPCDeckHandler(
	eventStore domain.EventStore[npcdeck.ID, npcdeck.Event, npcdeck.State],
	identifier domain.Identifier[npcdeck.ID],
	entityRefs domain.EntityRefResolver,
	updateName npcdeck.UpdateNameFunc,
	updateCardReferences npcdeck.UpdateCardReferencesFunc,
	cardStatuses CardStatusesFunc,
	presetDecks domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State],
	presetDeckIdentifier domain.Identifier[presetdeck.ID],
	getRules GetGameRulesFunc,
	now func() time.Time,
) *NPCDeckHandler {
	return &NPCDeckHandler{
		eventStore:           eventStore,
		identifier:           identifier,
		entityRefs:           entityRefs,
		updateName:           updateName,
		updateCardReferences: updateCardReferences,
		cardStatuses:         cardStatuses,
		presetDecks:          presetDecks,
		presetDeckIdentifier: presetDeckIdentifier,
		getRules:             getRules,
		now:                  now,
	}
}

// 変える操作の結果は、公開 ID と新しい revision だけにする（理由は ChangedPresetDeck と同じ）
type ChangedNPCDeck struct {
	ID       uuid.UUID
	Revision int64
}

// 空の NPC のデッキ（準備中）を作る。操作した人は context のオペレーター。入力の誤りは input.Errors、
// 使われている名前（状態によらず、すべての NPC のデッキの中で）なら ErrNPCDeckNameAlreadyInUse
func (c *NPCDeckHandler) Register(ctx context.Context, name string) (ChangedNPCDeck, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCDeck{}, ErrNotOperator
	}
	npcDeckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedNPCDeck{}, input.Errors{invalid}
	}
	return c.register(ctx, &npcdeck.RegisterCommand{Name: npcDeckName, RegisteredBy: registeredBy})
}

// プリセットデッキのカードと枚数を写して、名前 name のNPC のデッキ（準備中）を作る。写した元の公開 ID と、写したときの
// revision を記録する。元のプリセットデッキが無い（削除したものを含む）なら ErrPresetDeckNotFound。
// 名前を元から写さないのは、同じプリセットデッキから 2 つ目を作ると必ず名前の重複になり、1 つしか作れなくなるため。
// 準備中は決まり（合計の枚数等）を満たさなくてよいため、元がルールを満たすかは確かめない。ほかのエラーは Register と同じ
func (c *NPCDeckHandler) RegisterFromPresetDeck(ctx context.Context, presetDeckID uuid.UUID, name string) (ChangedNPCDeck, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCDeck{}, ErrNotOperator
	}
	deckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedNPCDeck{}, input.Errors{invalid}
	}
	preset, err := c.loadPresetDeck(ctx, presetDeckID)
	if err != nil {
		return ChangedNPCDeck{}, err
	}
	source, err := npcdeck.NewSource(preset.PublicID, preset.Version)
	if err != nil {
		return ChangedNPCDeck{}, err
	}
	return c.register(ctx, &npcdeck.RegisterCommand{Name: deckName, Source: &source, Cards: preset.Cards, RegisteredBy: registeredBy})
}

func (c *NPCDeckHandler) register(ctx context.Context, cmd *npcdeck.RegisterCommand) (ChangedNPCDeck, error) {
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCDeck, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		if err := c.updateName(ctx, id, cmd.Name); err != nil {
			return ChangedNPCDeck{}, err
		}
		cmd.ID, cmd.PublicID = id, publicID
		registered, err := c.decideAndSave(ctx, npcdeck.InitialState(), cmd)
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		if len(registered.Cards) > 0 {
			if err := c.updateCardReferences(ctx, registered.ID, slices.Collect(maps.Keys(registered.Cards))); err != nil {
				return ChangedNPCDeck{}, err
			}
		}
		return changedNPCDeck(registered), nil
	})
	if errors.Is(err, npcdeck.ErrNameAlreadyInUse) {
		return ChangedNPCDeck{}, ErrNPCDeckNameAlreadyInUse
	}
	return registered, err
}

// revision は画面で見た NPC のデッキの版。無い・削除したなら ErrNPCDeckNotFound、画面を開いた後にほかの操作で
// 変わっていれば ErrRevisionConflict、使われている名前なら ErrNPCDeckNameAlreadyInUse、準備中でなければ ErrNPCDeckNotDraft
func (c *NPCDeckHandler) Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (ChangedNPCDeck, error) {
	renamedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCDeck{}, ErrNotOperator
	}
	npcDeckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedNPCDeck{}, input.Errors{invalid}
	}
	renamed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCDeck, error) {
		npcDeck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		if npcDeck.Status != npcdeck.StatusDraft {
			return ChangedNPCDeck{}, npcdeck.ErrNotDraft
		}
		if err := c.updateName(ctx, npcDeck.ID, npcDeckName); err != nil {
			return ChangedNPCDeck{}, err
		}
		renamed, err := c.decideAndSave(ctx, npcDeck, &npcdeck.RenameCommand{Name: npcDeckName, RenamedBy: renamedBy})
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		return changedNPCDeck(renamed), nil
	})
	if errors.Is(err, npcdeck.ErrNameAlreadyInUse) {
		return ChangedNPCDeck{}, ErrNPCDeckNameAlreadyInUse
	}
	return renamed, mapNPCDeckError(err)
}

// 入力の誤りの決まりは PresetDeckHandler.SetCardCounts と同じ（新しく入れる／増やすカードは公開中だけ等）。
// 準備中でなければ、入力の誤りより先に ErrNPCDeckNotDraft。ほかのエラーは Rename と同じ
func (c *NPCDeckHandler) SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []CardCountInput) (ChangedNPCDeck, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCDeck{}, ErrNotOperator
	}
	rules, err := c.getRules(ctx)
	if err != nil {
		return ChangedNPCDeck{}, err
	}
	requested, err := parseCardCounts(ctx, c.entityRefs, rules.Deck, cards)
	if err != nil {
		return ChangedNPCDeck{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCDeck, error) {
		npcDeck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		if npcDeck.Status != npcdeck.StatusDraft {
			return ChangedNPCDeck{}, npcdeck.ErrNotDraft
		}
		if err := requireAvailable(ctx, c.cardStatuses, npcDeck.Cards, requested); err != nil {
			return ChangedNPCDeck{}, err
		}
		changed, err := c.decideAndSave(ctx, npcDeck, &npcdeck.SetCardCountsCommand{Counts: requested.counts, Rules: rules.Deck, ChangedBy: changedBy})
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		if err := c.updateCardReferences(ctx, changed.ID, slices.Collect(maps.Keys(changed.Cards))); err != nil {
			return ChangedNPCDeck{}, err
		}
		return changedNPCDeck(changed), nil
	})
	if errors.Is(err, npcdeck.ErrTotalCountTooLarge) {
		return ChangedNPCDeck{}, input.Errors{&input.TotalTooLarge{Field: "cards", Max: rules.Deck.TotalCardCount}}
	}
	return changed, mapNPCDeckError(err)
}

// 公開中にする（使用不可からの公開に戻すのも同じ）。公開するとき確かめるのは、合計の枚数がちょうどルールの値
// （input.TotalMismatch）・入っているカードがすべて公開中（input.Unavailable。項目名は cards）。公開中の数の上限は持たない。名前は登録・変更のときに全体で一意にしてあるため、ここでは確かめない。
// すでに公開中なら、何も確かめず変えずに成功する（revision が古ければ ErrRevisionConflict）。ほかのエラーは Rename と同じ
func (c *NPCDeckHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedNPCDeck, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCDeck{}, ErrNotOperator
	}
	rules, err := c.getRules(ctx)
	if err != nil {
		return ChangedNPCDeck{}, err
	}
	published, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCDeck, error) {
		npcDeck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		_, changing, err := npcDeck.Status.Publish()
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		if !changing {
			return changedNPCDeck(npcDeck), nil
		}
		if err := c.checkPublishable(ctx, npcDeck, rules.Deck); err != nil {
			return ChangedNPCDeck{}, err
		}
		published, err := c.decideAndSave(ctx, npcDeck, &npcdeck.PublishCommand{Rules: rules.Deck, PublishedBy: publishedBy})
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		return changedNPCDeck(published), nil
	})
	return published, mapNPCDeckError(err)
}

// 合計とカードの誤りは、まとめて input.Errors で返す。カードの状態を読むのと保存の間の競合を許す理由は requireAvailable と同じ
func (c *NPCDeckHandler) checkPublishable(ctx context.Context, npcDeck npcdeck.NPCDeck, rules gamerules.Deck) error {
	invalid := input.Errors{}
	if npcDeck.TotalCount() != rules.TotalCardCount {
		invalid = append(invalid, &input.TotalMismatch{Field: "cards", Required: rules.TotalCardCount})
	}
	if len(npcDeck.Cards) > 0 {
		refs := slices.SortedFunc(maps.Keys(npcDeck.Cards), func(a, b presetdeck.CardRef) int {
			return cmp.Or(cmp.Compare(a.EntityType(), b.EntityType()), cmp.Compare(a.ID(), b.ID()))
		})
		statuses, err := c.cardStatuses(ctx, refs)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			if statuses[ref] != card.StatusPublished {
				invalid = append(invalid, &input.Unavailable{Field: "cards"})
				break
			}
		}
	}
	if len(invalid) > 0 {
		return invalid
	}
	return nil
}

// 使用不可にする。すでに使用不可なら変えずに成功する。
// 準備中なら ErrInvalidNPCDeckStatusTransition。ほかのエラーは Rename と同じ
func (c *NPCDeckHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedNPCDeck, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedNPCDeck{}, ErrNotOperator
	}
	retired, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedNPCDeck, error) {
		npcDeck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		retired, err := c.decideAndSave(ctx, npcDeck, &npcdeck.RetireCommand{RetiredBy: retiredBy})
		if err != nil {
			return ChangedNPCDeck{}, err
		}
		return changedNPCDeck(retired), nil
	})
	return retired, mapNPCDeckError(err)
}

// 準備中のものだけ消せる。削除のイベントと一緒に識別子を消し、名前を使えるようにする。準備中でなければ ErrNPCDeckNotDraft。
// ほかのエラーは Rename と同じ
func (c *NPCDeckHandler) Delete(ctx context.Context, id uuid.UUID, revision int64) error {
	deletedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ErrNotOperator
	}
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		npcDeck, err := c.load(ctx, id, revision)
		if err != nil {
			return struct{}{}, err
		}
		if _, err := c.decideAndSave(ctx, npcDeck, &npcdeck.DeleteCommand{DeletedBy: deletedBy}); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.identifier.Delete(ctx, npcDeck.ID)
	})
	return mapNPCDeckError(err)
}

// 無い・削除したなら ErrPresetDeckNotFound
func (c *NPCDeckHandler) loadPresetDeck(ctx context.Context, publicID uuid.UUID) (presetdeck.PresetDeck, error) {
	id, err := c.presetDeckIdentifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return presetdeck.PresetDeck{}, ErrPresetDeckNotFound
	}
	if err != nil {
		return presetdeck.PresetDeck{}, err
	}
	state, err := c.presetDecks.Load(ctx, id)
	if err != nil {
		return presetdeck.PresetDeck{}, err
	}
	presetDeck, ok := state.(presetdeck.PresetDeck)
	if !ok {
		return presetdeck.PresetDeck{}, ErrPresetDeckNotFound
	}
	return presetDeck, nil
}

// revision を Load の直後に比べる理由は PresetDeckHandler.load と同じ
func (c *NPCDeckHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (npcdeck.NPCDeck, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return npcdeck.NPCDeck{}, ErrNPCDeckNotFound
	}
	if err != nil {
		return npcdeck.NPCDeck{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return npcdeck.NPCDeck{}, err
	}
	npcDeck, ok := state.(npcdeck.NPCDeck)
	if !ok {
		return npcdeck.NPCDeck{}, ErrNPCDeckNotFound
	}
	if npcDeck.Version != revision {
		return npcdeck.NPCDeck{}, ErrRevisionConflict
	}
	return npcDeck, nil
}

// 状態を変えない操作（同じ名前・公開中にもう一度公開する等）は版が進まない
func (c *NPCDeckHandler) decideAndSave(ctx context.Context, state npcdeck.State, cmd npcdeck.Command) (npcdeck.NPCDeck, error) {
	events, err := npcdeck.Decide(state, cmd, c.now())
	if err != nil {
		return npcdeck.NPCDeck{}, err
	}
	if err := c.eventStore.Save(ctx, events); err != nil {
		return npcdeck.NPCDeck{}, err
	}
	for _, event := range events {
		if state, err = npcdeck.Evolve(state, event); err != nil {
			return npcdeck.NPCDeck{}, err
		}
	}
	npcDeck, _ := state.(npcdeck.NPCDeck)
	return npcDeck, nil
}

func changedNPCDeck(npcDeck npcdeck.NPCDeck) ChangedNPCDeck {
	return ChangedNPCDeck{ID: npcDeck.PublicID, Revision: npcDeck.Version}
}

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変える
func mapNPCDeckError(err error) error {
	switch {
	case errors.Is(err, npcdeck.ErrNotDraft):
		return ErrNPCDeckNotDraft
	case errors.Is(err, npcdeck.ErrInvalidStatusTransition):
		return ErrInvalidNPCDeckStatusTransition
	}
	return revisionConflict(err)
}
