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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type StarterDeckHandler struct {
	eventStore domain.EventStore[starterdeck.ID, starterdeck.Event, starterdeck.State]
	identifier domain.Identifier[starterdeck.ID]
	entityRefs domain.EntityRefResolver
	updateName starterdeck.UpdateNameFunc
	// 入れたカードを参照する（参照されている間はカードの識別子を消せない）
	updateCardReferences starterdeck.UpdateCardReferencesFunc
	cardStatuses         CardStatusesFunc
	// 写す元のプリセットデッキを、公開 ID から引いて Load する
	presetDecks          domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State]
	presetDeckIdentifier domain.Identifier[presetdeck.ID]
	// 公開中の数の持ち主（ゲーム全体のルールの集約）を、一意な key から引く
	findGameRules gamerules.FindFunc
	reserveQuota  starterdeck.ReserveQuotaFunc
	releaseQuota  starterdeck.ReleaseQuotaFunc
	getRules      GetGameRulesFunc
	now           func() time.Time
}

func NewStarterDeckHandler(
	eventStore domain.EventStore[starterdeck.ID, starterdeck.Event, starterdeck.State],
	identifier domain.Identifier[starterdeck.ID],
	entityRefs domain.EntityRefResolver,
	updateName starterdeck.UpdateNameFunc,
	updateCardReferences starterdeck.UpdateCardReferencesFunc,
	cardStatuses CardStatusesFunc,
	presetDecks domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State],
	presetDeckIdentifier domain.Identifier[presetdeck.ID],
	findGameRules gamerules.FindFunc,
	reserveQuota starterdeck.ReserveQuotaFunc,
	releaseQuota starterdeck.ReleaseQuotaFunc,
	getRules GetGameRulesFunc,
	now func() time.Time,
) *StarterDeckHandler {
	return &StarterDeckHandler{
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
type ChangedStarterDeck struct {
	ID       uuid.UUID
	Revision int64
}

// 空のスターターデッキ（準備中）を作る。操作した人は context のオペレーター。入力の誤りは input.Errors、
// 使われている名前（状態によらず、すべてのスターターデッキの中で）なら ErrStarterDeckNameAlreadyInUse
func (c *StarterDeckHandler) Register(ctx context.Context, name string) (ChangedStarterDeck, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterDeck{}, ErrNotOperator
	}
	deckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedStarterDeck{}, input.Errors{invalid}
	}
	return c.register(ctx, &starterdeck.RegisterCommand{Name: deckName, RegisteredBy: registeredBy})
}

// プリセットデッキのカードと枚数を写して、名前 name のスターターデッキ（準備中）を作る。写した元の公開 ID と、写したときの
// revision を記録する。元のプリセットデッキが無い（削除したものを含む）なら ErrPresetDeckNotFound。
// 名前を元から写さないのは、同じプリセットデッキから 2 つ目を作ると必ず名前の重複になり、1 つしか作れなくなるため。
// 準備中は決まり（合計の枚数等）を満たさなくてよいため、元がルールを満たすかは確かめない。ほかのエラーは Register と同じ
func (c *StarterDeckHandler) RegisterFromPresetDeck(ctx context.Context, presetDeckID uuid.UUID, name string) (ChangedStarterDeck, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterDeck{}, ErrNotOperator
	}
	deckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedStarterDeck{}, input.Errors{invalid}
	}
	preset, err := c.loadPresetDeck(ctx, presetDeckID)
	if err != nil {
		return ChangedStarterDeck{}, err
	}
	source, err := starterdeck.NewSource(preset.PublicID, preset.Version)
	if err != nil {
		return ChangedStarterDeck{}, err
	}
	return c.register(ctx, &starterdeck.RegisterCommand{Name: deckName, Source: &source, Cards: preset.Cards, RegisteredBy: registeredBy})
}

func (c *StarterDeckHandler) register(ctx context.Context, cmd *starterdeck.RegisterCommand) (ChangedStarterDeck, error) {
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterDeck, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		if err := c.updateName(ctx, id, cmd.Name); err != nil {
			return ChangedStarterDeck{}, err
		}
		cmd.ID, cmd.PublicID = id, publicID
		registered, _, err := c.decideAndSave(ctx, starterdeck.InitialState(), cmd)
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		if len(registered.Cards) > 0 {
			if err := c.updateCardReferences(ctx, registered.ID, slices.Collect(maps.Keys(registered.Cards))); err != nil {
				return ChangedStarterDeck{}, err
			}
		}
		return changedStarterDeck(registered), nil
	})
	if errors.Is(err, starterdeck.ErrNameAlreadyInUse) {
		return ChangedStarterDeck{}, ErrStarterDeckNameAlreadyInUse
	}
	return registered, err
}

// revision は画面で見たスターターデッキの版。無い・削除したなら ErrStarterDeckNotFound、画面を開いた後にほかの操作で
// 変わっていれば ErrRevisionConflict、使われている名前なら ErrStarterDeckNameAlreadyInUse、準備中でなければ ErrStarterDeckNotDraft
func (c *StarterDeckHandler) Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (ChangedStarterDeck, error) {
	renamedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterDeck{}, ErrNotOperator
	}
	deckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedStarterDeck{}, input.Errors{invalid}
	}
	renamed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		if deck.Status != starterdeck.StatusDraft {
			return ChangedStarterDeck{}, starterdeck.ErrNotDraft
		}
		if err := c.updateName(ctx, deck.ID, deckName); err != nil {
			return ChangedStarterDeck{}, err
		}
		renamed, _, err := c.decideAndSave(ctx, deck, &starterdeck.RenameCommand{Name: deckName, RenamedBy: renamedBy})
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		return changedStarterDeck(renamed), nil
	})
	if errors.Is(err, starterdeck.ErrNameAlreadyInUse) {
		return ChangedStarterDeck{}, ErrStarterDeckNameAlreadyInUse
	}
	return renamed, mapStarterDeckError(err)
}

// 入力の誤りの決まりは PresetDeckHandler.SetCardCounts と同じ（新しく入れる／増やすカードは公開中だけ等）。
// 準備中でなければ、入力の誤りより先に ErrStarterDeckNotDraft。ほかのエラーは Rename と同じ
func (c *StarterDeckHandler) SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []CardCountInput) (ChangedStarterDeck, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterDeck{}, ErrNotOperator
	}
	rules, err := c.getRules(ctx)
	if err != nil {
		return ChangedStarterDeck{}, err
	}
	requested, err := parseCardCounts(ctx, c.entityRefs, rules.Deck, cards)
	if err != nil {
		return ChangedStarterDeck{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		if deck.Status != starterdeck.StatusDraft {
			return ChangedStarterDeck{}, starterdeck.ErrNotDraft
		}
		if err := requireAvailable(ctx, c.cardStatuses, deck.Cards, requested); err != nil {
			return ChangedStarterDeck{}, err
		}
		changed, _, err := c.decideAndSave(ctx, deck, &starterdeck.SetCardCountsCommand{Counts: requested.counts, Rules: rules.Deck, ChangedBy: changedBy})
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		if err := c.updateCardReferences(ctx, changed.ID, slices.Collect(maps.Keys(changed.Cards))); err != nil {
			return ChangedStarterDeck{}, err
		}
		return changedStarterDeck(changed), nil
	})
	if errors.Is(err, starterdeck.ErrTotalCountTooLarge) {
		return ChangedStarterDeck{}, input.Errors{&input.TotalTooLarge{Field: "cards", Max: rules.Deck.TotalCardCount}}
	}
	return changed, mapStarterDeckError(err)
}

// 公開中にする（使用不可からの公開に戻すのも同じ）。公開するとき確かめるのは、合計の枚数がちょうどルールの値
// （input.TotalMismatch）・入っているカードがすべて公開中（input.Unavailable。項目名は cards）・公開中の数の上限
// （ErrStarterDeckLimitReached。上限の値は LimitReachedError）。名前は登録・変更のときに全体で一意にしてあるため、ここでは確かめない。
// すでに公開中なら、何も確かめず変えずに成功する（revision が古ければ ErrRevisionConflict）。ほかのエラーは Rename と同じ
func (c *StarterDeckHandler) Publish(ctx context.Context, id uuid.UUID, revision int64) (ChangedStarterDeck, error) {
	publishedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterDeck{}, ErrNotOperator
	}
	rules, err := c.getRules(ctx)
	if err != nil {
		return ChangedStarterDeck{}, err
	}
	published, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		_, changing, err := deck.Status.Publish()
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		if !changing {
			return changedStarterDeck(deck), nil
		}
		if err := c.checkPublishable(ctx, deck, rules.Deck); err != nil {
			return ChangedStarterDeck{}, err
		}
		if err := c.reservePublished(ctx, rules.StarterDeck.MaxPublished); err != nil {
			return ChangedStarterDeck{}, err
		}
		published, _, err := c.decideAndSave(ctx, deck, &starterdeck.PublishCommand{Rules: rules.Deck, PublishedBy: publishedBy})
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		return changedStarterDeck(published), nil
	})
	if errors.Is(err, starterdeck.ErrLimitReached) {
		return ChangedStarterDeck{}, &LimitReachedError{reason: ErrStarterDeckLimitReached, Max: rules.StarterDeck.MaxPublished}
	}
	return published, mapStarterDeckError(err)
}

// 合計とカードの誤りは、まとめて input.Errors で返す。カードの状態を読むのと保存の間の競合を許す理由は requireAvailable と同じ
func (c *StarterDeckHandler) checkPublishable(ctx context.Context, deck starterdeck.StarterDeck, rules gamerules.Deck) error {
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
func (c *StarterDeckHandler) reservePublished(ctx context.Context, maxPublished int) error {
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
	return c.reserveQuota(ctx, starterdeck.PublishedQuota(gameRulesID), limit)
}

// 使用不可にする。公開中の数を 1 つ戻す（もう一度公開できるように）。すでに使用不可なら変えずに成功する。
// 準備中なら ErrInvalidStarterDeckStatusTransition。ほかのエラーは Rename と同じ
func (c *StarterDeckHandler) Retire(ctx context.Context, id uuid.UUID, revision int64) (ChangedStarterDeck, error) {
	retiredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterDeck{}, ErrNotOperator
	}
	retired, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		retired, changed, err := c.decideAndSave(ctx, deck, &starterdeck.RetireCommand{RetiredBy: retiredBy})
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		if changed {
			gameRulesID, found, err := c.findGameRules(ctx)
			if err != nil {
				return ChangedStarterDeck{}, err
			}
			if !found {
				return ChangedStarterDeck{}, ErrGameRulesNotCreated
			}
			if err := c.releaseQuota(ctx, starterdeck.PublishedQuota(gameRulesID)); err != nil {
				return ChangedStarterDeck{}, err
			}
		}
		return changedStarterDeck(retired), nil
	})
	return retired, mapStarterDeckError(err)
}

// 準備中のものだけ消せる。削除のイベントと一緒に識別子を消し、名前を使えるようにする。準備中でなければ ErrStarterDeckNotDraft。
// ほかのエラーは Rename と同じ
func (c *StarterDeckHandler) Delete(ctx context.Context, id uuid.UUID, revision int64) error {
	deletedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ErrNotOperator
	}
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return struct{}{}, err
		}
		if _, _, err := c.decideAndSave(ctx, deck, &starterdeck.DeleteCommand{DeletedBy: deletedBy}); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.identifier.Delete(ctx, deck.ID)
	})
	return mapStarterDeckError(err)
}

// 無い・削除したなら ErrPresetDeckNotFound
func (c *StarterDeckHandler) loadPresetDeck(ctx context.Context, publicID uuid.UUID) (presetdeck.PresetDeck, error) {
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
func (c *StarterDeckHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (starterdeck.StarterDeck, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return starterdeck.StarterDeck{}, ErrStarterDeckNotFound
	}
	if err != nil {
		return starterdeck.StarterDeck{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return starterdeck.StarterDeck{}, err
	}
	deck, ok := state.(starterdeck.StarterDeck)
	if !ok {
		return starterdeck.StarterDeck{}, ErrStarterDeckNotFound
	}
	if deck.Version != revision {
		return starterdeck.StarterDeck{}, ErrRevisionConflict
	}
	return deck, nil
}

// 変わった（イベントを作った）かも返す。状態を変えない操作（同じ名前・公開中にもう一度公開する等）は版が進まない
func (c *StarterDeckHandler) decideAndSave(ctx context.Context, state starterdeck.State, cmd starterdeck.Command) (starterdeck.StarterDeck, bool, error) {
	events, err := starterdeck.Decide(state, cmd, c.now())
	if err != nil {
		return starterdeck.StarterDeck{}, false, err
	}
	if err := c.eventStore.Save(ctx, events); err != nil {
		return starterdeck.StarterDeck{}, false, err
	}
	for _, event := range events {
		if state, err = starterdeck.Evolve(state, event); err != nil {
			return starterdeck.StarterDeck{}, false, err
		}
	}
	deck, _ := state.(starterdeck.StarterDeck)
	return deck, len(events) > 0, nil
}

func changedStarterDeck(deck starterdeck.StarterDeck) ChangedStarterDeck {
	return ChangedStarterDeck{ID: deck.PublicID, Revision: deck.Version}
}

// presentation 層にドメインのエラーを見せないため、application 層のエラーに変える
func mapStarterDeckError(err error) error {
	switch {
	case errors.Is(err, starterdeck.ErrNotDraft):
		return ErrStarterDeckNotDraft
	case errors.Is(err, starterdeck.ErrInvalidStatusTransition):
		return ErrInvalidStarterDeckStatusTransition
	}
	return revisionConflict(err)
}
