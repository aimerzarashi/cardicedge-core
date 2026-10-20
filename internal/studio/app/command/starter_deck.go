package command

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/input"
)

type StarterDeckCommands struct {
	eventStore domain.EventStore[starterdeck.ID, starterdeck.Event, starterdeck.State]
	identifier domain.Identifier[starterdeck.ID]
	entityRefs domain.EntityRefResolver
	updateName starterdeck.UpdateNameFunc
	// 入れたカードを参照する（参照されている間はカードの識別子を消せない）
	updateCardReferences starterdeck.UpdateCardReferencesFunc
	now                  func() time.Time
}

func NewStarterDeckCommands(
	eventStore domain.EventStore[starterdeck.ID, starterdeck.Event, starterdeck.State],
	identifier domain.Identifier[starterdeck.ID],
	entityRefs domain.EntityRefResolver,
	updateName starterdeck.UpdateNameFunc,
	updateCardReferences starterdeck.UpdateCardReferencesFunc,
	now func() time.Time,
) *StarterDeckCommands {
	return &StarterDeckCommands{
		eventStore:           eventStore,
		identifier:           identifier,
		entityRefs:           entityRefs,
		updateName:           updateName,
		updateCardReferences: updateCardReferences,
		now:                  now,
	}
}

// 変える操作の結果は、公開 ID と新しい revision だけにする（内容は返さない）。画面は詳細を読み直し、詳細の revision を持つ
// （表示している内容と revision を同じ時点のものにそろえるため。core#24）。変えた後の全体を返す案は、カードの公開 ID を
// 引き直す処理と、問い合わせと同じ形の結果の型をコマンドにも持つことになるため採らなかった
type ChangedStarterDeck struct {
	ID       uuid.UUID
	Revision int64
}

// 操作した人は context のオペレーター。入力の誤りは input.Errors、使われている名前なら ErrStarterDeckNameAlreadyInUse
func (c *StarterDeckCommands) Register(ctx context.Context, name string) (ChangedStarterDeck, error) {
	registeredBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterDeck{}, ErrNotOperator
	}
	deckName, invalid := newStarterDeckName("name", name)
	if invalid != nil {
		return ChangedStarterDeck{}, input.Errors{invalid}
	}
	registered, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterDeck, error) {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		if err := c.updateName(ctx, id, deckName); err != nil {
			return ChangedStarterDeck{}, err
		}
		cmd := &starterdeck.RegisterCommand{ID: id, PublicID: publicID, Name: deckName, RegisteredBy: registeredBy}
		registered, err := c.decideAndSave(ctx, starterdeck.InitialState(), cmd)
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		return changedStarterDeck(registered), nil
	})
	if errors.Is(err, starterdeck.ErrNameAlreadyInUse) {
		return ChangedStarterDeck{}, ErrStarterDeckNameAlreadyInUse
	}
	return registered, err
}

// revision は画面で見たスターターデッキの版。無い・削除したなら ErrStarterDeckNotFound、
// 画面を開いた後にほかの操作で変わっていれば ErrRevisionConflict、使われている名前なら ErrStarterDeckNameAlreadyInUse
func (c *StarterDeckCommands) Rename(ctx context.Context, id uuid.UUID, revision int64, name string) (ChangedStarterDeck, error) {
	renamedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterDeck{}, ErrNotOperator
	}
	deckName, invalid := newStarterDeckName("name", name)
	if invalid != nil {
		return ChangedStarterDeck{}, input.Errors{invalid}
	}
	renamed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		if err := c.updateName(ctx, deck.ID, deckName); err != nil {
			return ChangedStarterDeck{}, err
		}
		renamed, err := c.decideAndSave(ctx, deck, &starterdeck.RenameCommand{Name: deckName, RenamedBy: renamedBy})
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		return changedStarterDeck(renamed), nil
	})
	if errors.Is(err, starterdeck.ErrNameAlreadyInUse) {
		return ChangedStarterDeck{}, ErrStarterDeckNameAlreadyInUse
	}
	return renamed, revisionConflict(err)
}

// 変えるカードだけの、その枚数にする入力（変えないカードは含めなくてよい）
type CardCountInput struct {
	CardID uuid.UUID
	Count  int
}

// 枚数を 0 にするとデッキから外す。一覧のどれか 1 つでも誤りがあれば何も変えない。入力の誤りは、一覧が長すぎる
// （MaxTotalCount 件を超える）・カードが無い（Creature・Item・Spell のどれでもない）・枚数が範囲の外・同じカードが 2 回出てきた
// （2 回目に付ける）・変えた後の合計が MaxTotalCount を超える。空の一覧は誤りにしない（変わらない操作として扱う）。
// ほかのエラーは Rename と同じ
func (c *StarterDeckCommands) SetCardCounts(ctx context.Context, id uuid.UUID, revision int64, cards []CardCountInput) (ChangedStarterDeck, error) {
	changedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterDeck{}, ErrNotOperator
	}
	// カードを引くのは DB を読むだけなので、入力の誤りをまとめて返せるよう、トランザクションの前に確かめる
	counts, err := c.cardCounts(ctx, cards)
	if err != nil {
		return ChangedStarterDeck{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterDeck, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		changed, err := c.decideAndSave(ctx, deck, &starterdeck.SetCardCountsCommand{Counts: counts, ChangedBy: changedBy})
		if err != nil {
			return ChangedStarterDeck{}, err
		}
		// 入れているカードを消せないよう、変えた後のカードを参照する（0 にしたカードは参照から外れる）
		if err := c.updateCardReferences(ctx, changed.ID, slices.Collect(maps.Keys(changed.Cards))); err != nil {
			return ChangedStarterDeck{}, err
		}
		return changedStarterDeck(changed), nil
	})
	if errors.Is(err, starterdeck.ErrTotalCountTooLarge) {
		return ChangedStarterDeck{}, input.Errors{&input.TotalTooLarge{Field: "cards", Max: starterdeck.MaxTotalCount}}
	}
	return changed, revisionConflict(err)
}

// 誤りは最初の 1 つで止めず、まとめて input.Errors で返す。項目名は cards.<番号>.cardId のように一覧の位置を含める。
// 一覧が長すぎるときは、1 件ずつの確かめをせずに返す（存在しない ID を大量に送られても、その数だけ処理しないため）
func (c *StarterDeckCommands) cardCounts(ctx context.Context, cards []CardCountInput) (map[starterdeck.CardRef]starterdeck.CardCount, error) {
	if len(cards) > starterdeck.MaxTotalCount {
		return nil, input.Errors{&input.OutOfRange{Field: "cards", Min: 0, Max: starterdeck.MaxTotalCount}}
	}
	ids := make([]uuid.UUID, 0, len(cards))
	for _, card := range cards {
		ids = append(ids, card.CardID)
	}
	refs, err := c.entityRefs.ResolveAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	invalid := input.Errors{}
	counts := make(map[starterdeck.CardRef]starterdeck.CardCount, len(cards))
	seen := make(map[uuid.UUID]bool, len(cards))
	for i, card := range cards {
		cardField := fmt.Sprintf("cards.%d.cardId", i)
		if seen[card.CardID] {
			invalid = append(invalid, &input.Duplicated{Field: cardField})
		}
		seen[card.CardID] = true
		// 行が無い・カードの種類でない、はどちらも、指したカードが無いとして扱う
		ref, found := refs[card.CardID]
		cardRef, err := starterdeck.NewCardRef(ref.EntityType(), ref.ID())
		if !found || errors.Is(err, starterdeck.ErrNotCard) {
			invalid = append(invalid, &input.NotFound{Field: cardField})
		} else if err != nil {
			return nil, err
		}
		count, err := starterdeck.NewCardCount(card.Count)
		if errors.Is(err, starterdeck.ErrCardCountOutOfRange) {
			invalid = append(invalid, &input.OutOfRange{Field: fmt.Sprintf("cards.%d.count", i), Min: 0, Max: starterdeck.MaxCardCount})
		} else if err != nil {
			return nil, err
		}
		counts[cardRef] = count
	}
	if len(invalid) > 0 {
		return nil, invalid
	}
	return counts, nil
}

// 削除のイベントと一緒に識別子を消し、名前をほかのスターターデッキが使えるようにする（core#24）。エラーは Rename と同じ
func (c *StarterDeckCommands) Delete(ctx context.Context, id uuid.UUID, revision int64) error {
	deletedBy, ok := authentication.Operator(ctx)
	if !ok {
		return ErrNotOperator
	}
	_, err := rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		deck, err := c.load(ctx, id, revision)
		if err != nil {
			return struct{}{}, err
		}
		events, err := starterdeck.Decide(deck, &starterdeck.DeleteCommand{DeletedBy: deletedBy}, c.now())
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
func (c *StarterDeckCommands) load(ctx context.Context, publicID uuid.UUID, revision int64) (starterdeck.StarterDeck, error) {
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

func (c *StarterDeckCommands) decideAndSave(ctx context.Context, state starterdeck.State, cmd starterdeck.Command) (starterdeck.StarterDeck, error) {
	events, err := starterdeck.Decide(state, cmd, c.now())
	if err != nil {
		return starterdeck.StarterDeck{}, err
	}
	if err := c.eventStore.Save(ctx, events); err != nil {
		return starterdeck.StarterDeck{}, err
	}
	for _, event := range events {
		if state, err = starterdeck.Evolve(state, event); err != nil {
			return starterdeck.StarterDeck{}, err
		}
	}
	return state.(starterdeck.StarterDeck), nil
}

func changedStarterDeck(deck starterdeck.StarterDeck) ChangedStarterDeck {
	return ChangedStarterDeck{ID: deck.PublicID, Revision: deck.Version}
}

// 読み込みから保存までに先を越された（eventstore の版の衝突）のも、利用者から見れば画面が古いのと同じなので、同じエラーにする
func revisionConflict(err error) error {
	if errors.Is(err, domain.ErrVersionConflict) {
		return ErrRevisionConflict
	}
	return err
}

// 入力の誤りでなければ input.Error は nil
func newStarterDeckName(field, value string) (starterdeck.Name, input.Error) {
	name, err := starterdeck.NewName(value)
	switch {
	case errors.Is(err, starterdeck.ErrNameEmpty):
		return "", &input.Required{Field: field}
	case errors.Is(err, starterdeck.ErrNameTooLong):
		return "", &input.TooLong{Field: field, Max: starterdeck.MaxNameLength}
	case err != nil:
		return "", &input.InvalidFormat{Field: field}
	}
	return name, nil
}
