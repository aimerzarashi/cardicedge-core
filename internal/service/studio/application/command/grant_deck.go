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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type GrantDeckHandler struct {
	eventStore domain.EventStore[grantdeck.ID, grantdeck.Event, grantdeck.State]
	identifier domain.Identifier[grantdeck.ID]
	entityRefs domain.EntityRefResolver
	updateName grantdeck.UpdateNameFunc
	// 入れたカードを参照する（参照されている間はカードの識別子を消せない）
	updateCardReferences grantdeck.UpdateCardReferencesFunc
	cardStatuses         CardStatusesFunc
	// 写す元のプリセットデッキを、公開 ID から引いて Load する
	presetDecks          domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State]
	presetDeckIdentifier domain.Identifier[presetdeck.ID]
	// 公開中の数の持ち主（ゲーム全体のルールの集約）を、一意な key から引く
	findGameRules gamerules.FindFunc
	reserveQuota  grantdeck.ReserveQuotaFunc
	releaseQuota  grantdeck.ReleaseQuotaFunc
	getRules      GetGameRulesFunc
	now           func() time.Time
}

func NewGrantDeckHandler(
	eventStore domain.EventStore[grantdeck.ID, grantdeck.Event, grantdeck.State],
	identifier domain.Identifier[grantdeck.ID],
	entityRefs domain.EntityRefResolver,
	updateName grantdeck.UpdateNameFunc,
	updateCardReferences grantdeck.UpdateCardReferencesFunc,
	cardStatuses CardStatusesFunc,
	presetDecks domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State],
	presetDeckIdentifier domain.Identifier[presetdeck.ID],
	findGameRules gamerules.FindFunc,
	reserveQuota grantdeck.ReserveQuotaFunc,
	releaseQuota grantdeck.ReleaseQuotaFunc,
	getRules GetGameRulesFunc,
	now func() time.Time,
) *GrantDeckHandler {
	return &GrantDeckHandler{
		eventStore:           eventStore,
		identifier:           identifier,
		entityRefs:           entityRefs,
		updateName:           updateName,
		updateCardReferences: updateCardReferences,
		cardStatuses:         cardStatuses,
		presetDecks:          presetDecks,
		presetDeckIdentifier: presetDeckIdentifier,
		findGameRules:        findGameRules,
		reserveQuota:         reserveQuota,
		releaseQuota:         releaseQuota,
		getRules:             getRules,
		now:                  now,
	}
}

// 変える操作の結果は、公開 ID と新しい revision だけにする（理由は ChangedPresetDeck と同じ）
type ChangedGrantDeck struct {
	ID       uuid.UUID
	Revision int64
}

// 空の配布デッキ（準備中）を作る。操作した人は context のオペレーター。入力の誤りは input.Errors、
// 使われている名前（状態によらず、すべての配布デッキの中で）なら ErrGrantDeckNameAlreadyInUse
func (c *GrantDeckHandler) Register(ctx context.Context, name string) (ChangedGrantDeck, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedGrantDeck{}, ErrNotOperator
	}
	deckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedGrantDeck{}, input.Errors{invalid}
	}
	return c.register(ctx, &grantdeck.RegisterCommand{Name: deckName, RegisteredBy: registeredBy})
}

// プリセットデッキのカードと枚数を写して、名前 name の配布デッキ（準備中）を作る。写した元の公開 ID と、写したときの
// revision を記録する。元のプリセットデッキが無い（削除したものを含む）なら ErrPresetDeckNotFound。
// 名前を元から写さないのは、同じプリセットデッキから 2 つ目を作ると必ず名前の重複になり、1 つしか作れなくなるため。
// 準備中は決まり（合計の枚数等）を満たさなくてよいため、元がルールを満たすかは確かめない。ほかのエラーは Register と同じ
func (c *GrantDeckHandler) RegisterFromPresetDeck(ctx context.Context, presetDeckID uuid.UUID, name string) (ChangedGrantDeck, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedGrantDeck{}, ErrNotOperator
	}
	deckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedGrantDeck{}, input.Errors{invalid}
	}
	preset, err := c.loadPresetDeck(ctx, presetDeckID)
	if err != nil {
		return ChangedGrantDeck{}, err
	}
	source, err := grantdeck.NewSource(preset.PublicID, preset.Version)
	if err != nil {
		return ChangedGrantDeck{}, err
	}
	return c.register(ctx, &grantdeck.RegisterCommand{Name: deckName, Source: &source, Cards: preset.Cards, RegisteredBy: registeredBy})
}

func (c *GrantDeckHandler) register(ctx context.Context, cmd *grantdeck.RegisterCommand) (ChangedGrantDeck, error) {
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedGrantDeck, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		if err := c.updateName(ctx, id, cmd.Name); err != nil {
			return ChangedGrantDeck{}, err
		}
		cmd.ID, cmd.PublicID = id, publicID
		registered, _, err := c.decideAndSave(ctx, grantdeck.InitialState(), cmd)
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		if len(registered.Cards) > 0 {
			if err := c.updateCardReferences(ctx, registered.ID, slices.Collect(maps.Keys(registered.Cards))); err != nil {
				return ChangedGrantDeck{}, err
			}
		}
		return changedGrantDeck(registered), nil
	})
	if errors.Is(err, grantdeck.ErrNameAlreadyInUse) {
		return ChangedGrantDeck{}, ErrGrantDeckNameAlreadyInUse
	}
	return registered, err
}

// revision は画面で見た配布デッキの版。無い・削除したなら ErrGrantDeckNotFound、画面を開いた後にほかの操作で
// 変わっていれば ErrRevisionConflict、使われている名前なら ErrGrantDeckNameAlreadyInUse、準備中でなければ ErrGrantDeckNotDraft
func (c *GrantDeckHandler) Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (ChangedGrantDeck, error) {
	renamedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedGrantDeck{}, ErrNotOperator
	}
	deckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedGrantDeck{}, input.Errors{invalid}
	}
	renamed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedGrantDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		if deck.Status != grantdeck.StatusDraft {
			return ChangedGrantDeck{}, grantdeck.ErrNotDraft
		}
		if err := c.updateName(ctx, deck.ID, deckName); err != nil {
			return ChangedGrantDeck{}, err
		}
		renamed, _, err := c.decideAndSave(ctx, deck, &grantdeck.RenameCommand{Name: deckName, RenamedBy: renamedBy})
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		return changedGrantDeck(renamed), nil
	})
	if errors.Is(err, grantdeck.ErrNameAlreadyInUse) {
		return ChangedGrantDeck{}, ErrGrantDeckNameAlreadyInUse
	}
	return renamed, mapGrantDeckError(err)
}

// 入力の誤りの決まりは PresetDeckHandler.SetCardCounts と同じ（新しく入れる／増やすカードは公開中だけ等）。
// 準備中でなければ、入力の誤りより先に ErrGrantDeckNotDraft。ほかのエラーは Rename と同じ
func (c *GrantDeckHandler) SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []CardCountInput) (ChangedGrantDeck, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedGrantDeck{}, ErrNotOperator
	}
	rules, err := c.getRules(ctx)
	if err != nil {
		return ChangedGrantDeck{}, err
	}
	requested, err := parseCardCounts(ctx, c.entityRefs, rules.Deck, cards)
	if err != nil {
		return ChangedGrantDeck{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedGrantDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		if deck.Status != grantdeck.StatusDraft {
			return ChangedGrantDeck{}, grantdeck.ErrNotDraft
		}
		if err := requireAvailable(ctx, c.cardStatuses, deck.Cards, requested); err != nil {
			return ChangedGrantDeck{}, err
		}
		changed, _, err := c.decideAndSave(ctx, deck, &grantdeck.SetCardCountsCommand{Counts: requested.counts, Rules: rules.Deck, ChangedBy: changedBy})
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		if err := c.updateCardReferences(ctx, changed.ID, slices.Collect(maps.Keys(changed.Cards))); err != nil {
			return ChangedGrantDeck{}, err
		}
		return changedGrantDeck(changed), nil
	})
	if errors.Is(err, grantdeck.ErrTotalCountTooLarge) {
		return ChangedGrantDeck{}, input.Errors{&input.TotalTooLarge{Field: "cards", Max: rules.Deck.TotalCardCount}}
	}
	return changed, mapGrantDeckError(err)
}

// 公開中にする（使用不可からの公開に戻すのも同じ）。公開するとき確かめるのは、合計の枚数がちょうどルールの値
// （input.TotalMismatch）・入っているカードがすべて公開中（input.Unavailable。項目名は cards）・公開中の数の上限
// （ErrGrantDeckLimitReached。上限の値は LimitReachedError）。名前は登録・変更のときに全体で一意にしてあるため、ここでは確かめない。
// すでに公開中なら、何も確かめず変えずに成功する（revision が古ければ ErrRevisionConflict）。ほかのエラーは Rename と同じ
func (c *GrantDeckHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedGrantDeck, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedGrantDeck{}, ErrNotOperator
	}
	rules, err := c.getRules(ctx)
	if err != nil {
		return ChangedGrantDeck{}, err
	}
	published, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedGrantDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		_, changing, err := deck.Status.Publish()
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		if !changing {
			return changedGrantDeck(deck), nil
		}
		if err := c.checkPublishable(ctx, deck, rules.Deck); err != nil {
			return ChangedGrantDeck{}, err
		}
		if err := c.reservePublished(ctx, rules.StarterDeck.MaxPublished); err != nil {
			return ChangedGrantDeck{}, err
		}
		published, _, err := c.decideAndSave(ctx, deck, &grantdeck.PublishCommand{Rules: rules.Deck, PublishedBy: publishedBy})
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		return changedGrantDeck(published), nil
	})
	if errors.Is(err, grantdeck.ErrLimitReached) {
		return ChangedGrantDeck{}, &LimitReachedError{reason: ErrGrantDeckLimitReached, Max: rules.StarterDeck.MaxPublished}
	}
	return published, mapGrantDeckError(err)
}

// 合計とカードの誤りは、まとめて input.Errors で返す。カードの状態を読むのと保存の間の競合を許す理由は requireAvailable と同じ
func (c *GrantDeckHandler) checkPublishable(ctx context.Context, deck grantdeck.GrantDeck, rules gamerules.Deck) error {
	invalid := input.Errors{}
	if deck.TotalCount() != rules.TotalCardCount {
		invalid = append(invalid, &input.TotalMismatch{Field: "cards", Required: rules.TotalCardCount})
	}
	if len(deck.Cards) > 0 {
		refs := slices.SortedFunc(maps.Keys(deck.Cards), func(a, b presetdeck.CardRef) int {
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

// 公開中の数を 1 つ確保する。持ち主は game_rules の内部 ID、上限はルールの値
func (c *GrantDeckHandler) reservePublished(ctx context.Context, maxPublished int) error {
	gameRulesID, found, err := c.findGameRules(ctx)
	if err != nil {
		return err
	}
	if !found {
		return ErrGameRulesNotCreated
	}
	limit, err := domain.NewQuotaLimit(maxPublished)
	if err != nil {
		return err
	}
	return c.reserveQuota(ctx, grantdeck.PublishedQuota(gameRulesID), limit)
}

// 使用不可にする。公開中の数を 1 つ戻す（もう一度公開できるように）。すでに使用不可なら変えずに成功する。
// 準備中なら ErrInvalidGrantDeckStatusTransition。ほかのエラーは Rename と同じ
func (c *GrantDeckHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedGrantDeck, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedGrantDeck{}, ErrNotOperator
	}
	retired, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedGrantDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		retired, changed, err := c.decideAndSave(ctx, deck, &grantdeck.RetireCommand{RetiredBy: retiredBy})
		if err != nil {
			return ChangedGrantDeck{}, err
		}
		if changed {
			gameRulesID, found, err := c.findGameRules(ctx)
			if err != nil {
				return ChangedGrantDeck{}, err
			}
			if !found {
				return ChangedGrantDeck{}, ErrGameRulesNotCreated
			}
			if err := c.releaseQuota(ctx, grantdeck.PublishedQuota(gameRulesID)); err != nil {
				return ChangedGrantDeck{}, err
			}
		}
		return changedGrantDeck(retired), nil
	})
	return retired, mapGrantDeckError(err)
}

// 準備中のものだけ消せる。削除のイベントと一緒に識別子を消し、名前を使えるようにする。準備中でなければ ErrGrantDeckNotDraft。
// ほかのエラーは Rename と同じ
func (c *GrantDeckHandler) Delete(ctx context.Context, id uuid.UUID, revision int64) error {
	deletedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ErrNotOperator
	}
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return struct{}{}, err
		}
		if _, _, err := c.decideAndSave(ctx, deck, &grantdeck.DeleteCommand{DeletedBy: deletedBy}); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.identifier.Delete(ctx, deck.ID)
	})
	return mapGrantDeckError(err)
}

// 無い・削除したなら ErrPresetDeckNotFound
func (c *GrantDeckHandler) loadPresetDeck(ctx context.Context, publicID uuid.UUID) (presetdeck.PresetDeck, error) {
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
	deck, ok := state.(presetdeck.PresetDeck)
	if !ok {
		return presetdeck.PresetDeck{}, ErrPresetDeckNotFound
	}
	return deck, nil
}

// revision を Load の直後に比べる理由は PresetDeckHandler.load と同じ
func (c *GrantDeckHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (grantdeck.GrantDeck, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return grantdeck.GrantDeck{}, ErrGrantDeckNotFound
	}
	if err != nil {
		return grantdeck.GrantDeck{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return grantdeck.GrantDeck{}, err
	}
	deck, ok := state.(grantdeck.GrantDeck)
	if !ok {
		return grantdeck.GrantDeck{}, ErrGrantDeckNotFound
	}
	if deck.Version != revision {
		return grantdeck.GrantDeck{}, ErrRevisionConflict
	}
	return deck, nil
}

// 変わった（イベントを作った）かも返す。状態を変えない操作（同じ名前・公開中にもう一度公開する等）は版が進まない
func (c *GrantDeckHandler) decideAndSave(ctx context.Context, state grantdeck.State, cmd grantdeck.Command) (grantdeck.GrantDeck, bool, error) {
	events, err := grantdeck.Decide(state, cmd, c.now())
	if err != nil {
		return grantdeck.GrantDeck{}, false, err
	}
	if err := c.eventStore.Save(ctx, events); err != nil {
		return grantdeck.GrantDeck{}, false, err
	}
	for _, event := range events {
		if state, err = grantdeck.Evolve(state, event); err != nil {
			return grantdeck.GrantDeck{}, false, err
		}
	}
	deck, _ := state.(grantdeck.GrantDeck)
	return deck, len(events) > 0, nil
}

func changedGrantDeck(deck grantdeck.GrantDeck) ChangedGrantDeck {
	return ChangedGrantDeck{ID: deck.PublicID, Revision: deck.Version}
}

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変える
func mapGrantDeckError(err error) error {
	switch {
	case errors.Is(err, grantdeck.ErrNotDraft):
		return ErrGrantDeckNotDraft
	case errors.Is(err, grantdeck.ErrInvalidStatusTransition):
		return ErrInvalidGrantDeckStatusTransition
	}
	return revisionConflict(err)
}
