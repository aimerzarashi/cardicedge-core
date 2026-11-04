package command

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type PresetDeckHandler struct {
	eventStore domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State]
	identifier domain.Identifier[presetdeck.ID]
	entityRefs domain.EntityRefResolver
	updateName presetdeck.UpdateNameFunc
	// 入れたカードを参照する（参照されている間はカードの識別子を消せない）
	updateCardReferences presetdeck.UpdateCardReferencesFunc
	cardStatuses         CardStatusesFunc
	getRules             GetGameRulesFunc
	now                  func() time.Time
}

func NewPresetDeckHandler(
	eventStore domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State],
	identifier domain.Identifier[presetdeck.ID],
	entityRefs domain.EntityRefResolver,
	updateName presetdeck.UpdateNameFunc,
	updateCardReferences presetdeck.UpdateCardReferencesFunc,
	cardStatuses CardStatusesFunc,
	getRules GetGameRulesFunc,
	now func() time.Time,
) *PresetDeckHandler {
	return &PresetDeckHandler{
		eventStore:           eventStore,
		identifier:           identifier,
		entityRefs:           entityRefs,
		updateName:           updateName,
		updateCardReferences: updateCardReferences,
		cardStatuses:         cardStatuses,
		getRules:             getRules,
		now:                  now,
	}
}

// 変える操作の結果は、公開 ID と新しい revision だけにする（内容は返さない）。画面は詳細を読み直し、詳細の revision を持つ
// （表示している内容と revision を同じ時点のものにそろえるため。core#24）。変えた後の全体を返す案は、カードの公開 ID を
// 引き直す処理と、問い合わせと同じ形の結果の型をコマンドにも持つことになるため採らなかった
type ChangedPresetDeck struct {
	ID       uuid.UUID
	Revision int64
}

// 操作した人は context のオペレーター。入力の誤りは input.Errors、使われている名前なら ErrPresetDeckNameAlreadyInUse
func (c *PresetDeckHandler) Register(ctx context.Context, name string) (ChangedPresetDeck, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedPresetDeck{}, ErrNotOperator
	}
	deckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedPresetDeck{}, input.Errors{invalid}
	}
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedPresetDeck, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return ChangedPresetDeck{}, err
		}
		if err := c.updateName(ctx, id, deckName); err != nil {
			return ChangedPresetDeck{}, err
		}
		cmd := &presetdeck.RegisterCommand{ID: id, PublicID: publicID, Name: deckName, RegisteredBy: registeredBy}
		registered, err := c.decideAndSave(ctx, presetdeck.InitialState(), cmd)
		if err != nil {
			return ChangedPresetDeck{}, err
		}
		return changedPresetDeck(registered), nil
	})
	if errors.Is(err, presetdeck.ErrNameAlreadyInUse) {
		return ChangedPresetDeck{}, ErrPresetDeckNameAlreadyInUse
	}
	return registered, err
}

// revision は画面で見たプリセットデッキの版。無い・削除したなら ErrPresetDeckNotFound、
// 画面を開いた後にほかの操作で変わっていれば ErrRevisionConflict、使われている名前なら ErrPresetDeckNameAlreadyInUse
func (c *PresetDeckHandler) Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (ChangedPresetDeck, error) {
	renamedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedPresetDeck{}, ErrNotOperator
	}
	deckName, invalid := newPresetDeckName("name", name)
	if invalid != nil {
		return ChangedPresetDeck{}, input.Errors{invalid}
	}
	renamed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedPresetDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedPresetDeck{}, err
		}
		if err := c.updateName(ctx, deck.ID, deckName); err != nil {
			return ChangedPresetDeck{}, err
		}
		renamed, err := c.decideAndSave(ctx, deck, &presetdeck.RenameCommand{Name: deckName, RenamedBy: renamedBy})
		if err != nil {
			return ChangedPresetDeck{}, err
		}
		return changedPresetDeck(renamed), nil
	})
	if errors.Is(err, presetdeck.ErrNameAlreadyInUse) {
		return ChangedPresetDeck{}, ErrPresetDeckNameAlreadyInUse
	}
	return renamed, revisionConflict(err)
}

// 枚数を 0 にするとデッキから外す。一覧のどれか 1 つでも誤りがあれば何も変えない。入力の誤りは、一覧が長すぎる
// （ルールの合計の枚数を超える件数）・カードが無い（Creature・Item・Spell のどれでもない）・枚数が範囲の外・同じカードが 2 回出てきた
// （2 回目に付ける）・変えた後の合計がルールの合計の枚数を超える・新しく入れる／枚数を増やすカードが公開中でない
// （input.Unavailable。すでに入っていて枚数を増やさないカードは、使用不可になっていても残せる）。
// 空の一覧は誤りにしない（変わらない操作として扱う）。ほかのエラーは Rename と同じ
func (c *PresetDeckHandler) SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []CardCountInput) (ChangedPresetDeck, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedPresetDeck{}, ErrNotOperator
	}
	rules, err := c.getRules(ctx)
	if err != nil {
		return ChangedPresetDeck{}, err
	}
	// カードを引くのは DB を読むだけなので、入力の誤りをまとめて返せるよう、トランザクションの前に確かめる
	requested, err := parseCardCounts(ctx, c.entityRefs, rules.Deck, cards)
	if err != nil {
		return ChangedPresetDeck{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedPresetDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedPresetDeck{}, err
		}
		if err := requireAvailable(ctx, c.cardStatuses, deck.Cards, requested); err != nil {
			return ChangedPresetDeck{}, err
		}
		changed, err := c.decideAndSave(ctx, deck, &presetdeck.SetCardCountsCommand{Set: requested.set, Remove: requested.remove, Rules: rules.Deck, ChangedBy: changedBy})
		if err != nil {
			return ChangedPresetDeck{}, err
		}
		// 入れているカードを消せないよう、変えた後のカードを参照する（0 にしたカードは参照から外れる）
		if err := c.updateCardReferences(ctx, changed.ID, slices.Collect(maps.Keys(changed.Cards))); err != nil {
			return ChangedPresetDeck{}, err
		}
		return changedPresetDeck(changed), nil
	})
	if errors.Is(err, presetdeck.ErrTotalCountTooLarge) {
		return ChangedPresetDeck{}, input.Errors{&input.TotalTooLarge{Field: "cards", Max: rules.Deck.TotalCardCount}}
	}
	return changed, revisionConflict(err)
}

// 削除のイベントと一緒に識別子を消し、名前をほかのプリセットデッキが使えるようにする（core#24）。エラーは Rename と同じ
func (c *PresetDeckHandler) Delete(ctx context.Context, id uuid.UUID, revision int64) error {
	deletedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ErrNotOperator
	}
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return struct{}{}, err
		}
		events, err := presetdeck.Decide(deck, &presetdeck.DeleteCommand{DeletedBy: deletedBy}, c.now())
		if err != nil {
			return struct{}{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.identifier.Delete(ctx, deck.ID)
	})
	return revisionConflict(err)
}

// revision は Load の直後に比べる。保存のときの版の衝突（eventstore）が守るのは、この Load から保存までの間だけで、
// 画面を開いてから操作するまでに変わったことは見つけられないため。変わらない操作（同じ名前など）でも、
// 古い画面を見て操作したことを知らせるため、比べてから進める
func (c *PresetDeckHandler) load(ctx context.Context, publicID uuid.UUID, revision int64) (presetdeck.PresetDeck, error) {
	id, err := c.identifier.Resolve(ctx, publicID)
	if errors.Is(err, domain.ErrEntityNotFound) {
		return presetdeck.PresetDeck{}, ErrPresetDeckNotFound
	}
	if err != nil {
		return presetdeck.PresetDeck{}, err
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return presetdeck.PresetDeck{}, err
	}
	deck, ok := state.(presetdeck.PresetDeck)
	if !ok {
		return presetdeck.PresetDeck{}, ErrPresetDeckNotFound
	}
	if deck.Version != revision {
		return presetdeck.PresetDeck{}, ErrRevisionConflict
	}
	return deck, nil
}

func (c *PresetDeckHandler) decideAndSave(ctx context.Context, state presetdeck.State, cmd presetdeck.Command) (presetdeck.PresetDeck, error) {
	events, err := presetdeck.Decide(state, cmd, c.now())
	if err != nil {
		return presetdeck.PresetDeck{}, err
	}
	if err := c.eventStore.Save(ctx, events); err != nil {
		return presetdeck.PresetDeck{}, err
	}
	for _, event := range events {
		if state, err = presetdeck.Evolve(state, event); err != nil {
			return presetdeck.PresetDeck{}, err
		}
	}
	return state.(presetdeck.PresetDeck), nil
}

func changedPresetDeck(deck presetdeck.PresetDeck) ChangedPresetDeck {
	return ChangedPresetDeck{ID: deck.PublicID, Revision: deck.Version}
}

// 読み込みから保存までに先を越された（eventstore の版の衝突）のも、利用者から見れば画面が古いのと同じなので、同じエラーにする
func revisionConflict(err error) error {
	if errors.Is(err, domain.ErrVersionConflict) {
		return ErrRevisionConflict
	}
	return err
}

// 入力の誤りでなければ input.Error は nil
func newPresetDeckName(field, value string) (presetdeck.Name, input.Error) {
	name, err := presetdeck.NewName(value)
	switch {
	case errors.Is(err, presetdeck.ErrNameEmpty):
		return "", &input.Required{Field: field}
	case errors.Is(err, presetdeck.ErrNameTooLong):
		return "", &input.TooLong{Field: field, Max: presetdeck.MaxNameLength}
	case err != nil:
		return "", &input.InvalidFormat{Field: field}
	}
	return name, nil
}
