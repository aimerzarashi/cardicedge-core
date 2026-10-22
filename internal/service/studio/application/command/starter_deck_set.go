package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

type StarterDeckSetHandler struct {
	eventStore domain.EventStore[starterdeckset.ID, starterdeckset.Event, starterdeckset.State]
	identifier domain.Identifier[starterdeckset.ID]
	find       starterdeckset.FindFunc
	create     starterdeckset.CreateFunc
	// 写したカードを参照する（参照されている間はカードの識別子を消せない）
	updateCardReferences starterdeckset.UpdateCardReferencesFunc
	// 写す元のプリセットデッキを、公開 ID から引いて Load する
	presetDecks          domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State]
	presetDeckIdentifier domain.Identifier[presetdeck.ID]
	now                  func() time.Time
}

func NewStarterDeckSetHandler(
	eventStore domain.EventStore[starterdeckset.ID, starterdeckset.Event, starterdeckset.State],
	identifier domain.Identifier[starterdeckset.ID],
	find starterdeckset.FindFunc,
	create starterdeckset.CreateFunc,
	updateCardReferences starterdeckset.UpdateCardReferencesFunc,
	presetDecks domain.EventStore[presetdeck.ID, presetdeck.Event, presetdeck.State],
	presetDeckIdentifier domain.Identifier[presetdeck.ID],
	now func() time.Time,
) *StarterDeckSetHandler {
	return &StarterDeckSetHandler{
		eventStore:           eventStore,
		identifier:           identifier,
		find:                 find,
		create:               create,
		updateCardReferences: updateCardReferences,
		presetDecks:          presetDecks,
		presetDeckIdentifier: presetDeckIdentifier,
		now:                  now,
	}
}

// スターターデッキセットはゲーム全体で 1 つで、画面は ID で指さないため、公開 ID は返さない（core#25）
type ChangedStarterDeckSet struct {
	Revision int64
}

// 選んだプリセットデッキの中身を写して、今のスターターデッキと丸ごと入れ替える（初めての設定・選び直し・写し直しのどれも）。
// revision は画面で見たスターターデッキセットの版（未設定なら 0）。入力の誤りは、数が RequiredDeckCount でない・
// 同じプリセットデッキが 2 回出てきた（2 回目に付ける）・プリセットデッキが無い（削除したものを含む）・合計がちょうど
// presetdeck.MaxTotalCount でない。画面を開いた後にほかの操作で変わっていれば（初めての設定がほかの人と重なったときも）
// ErrRevisionConflict
func (c *StarterDeckSetHandler) Set(ctx context.Context, revision int64, presetDeckIDs []uuid.UUID) (ChangedStarterDeckSet, error) {
	setBy, ok := authentication.Operator(ctx)
	if !ok {
		return ChangedStarterDeckSet{}, ErrNotOperator
	}
	// 写す元を引いて写すのは DB を読むだけなので、入力の誤りをまとめて返せるよう、トランザクションの前に確かめる。
	// 写した後に元が変えられても、写しは「変わる前の revision から写した」と記録されるため、ロックは要らない
	decks, err := c.copyPresetDecks(ctx, presetDeckIDs)
	if err != nil {
		return ChangedStarterDeckSet{}, err
	}
	changed, err := rdb.Transact(ctx, func(ctx context.Context) (ChangedStarterDeckSet, error) {
		id, publicID, state, err := c.load(ctx)
		if err != nil {
			return ChangedStarterDeckSet{}, err
		}
		if version(state) != revision {
			return ChangedStarterDeckSet{}, ErrRevisionConflict
		}
		events, err := starterdeckset.Decide(state, &starterdeckset.SetCommand{ID: id, PublicID: publicID, Decks: decks, SetBy: setBy}, c.now())
		if err != nil {
			return ChangedStarterDeckSet{}, err
		}
		if err := c.eventStore.Save(ctx, events); err != nil {
			return ChangedStarterDeckSet{}, err
		}
		for _, event := range events {
			if state, err = starterdeckset.Evolve(state, event); err != nil {
				return ChangedStarterDeckSet{}, err
			}
		}
		set := state.(starterdeckset.StarterDeckSet)
		if err := c.updateCardReferences(ctx, set.ID, set.Cards()); err != nil {
			return ChangedStarterDeckSet{}, err
		}
		return ChangedStarterDeckSet{Revision: set.Version}, nil
	})
	// 初めての設定がほかの人と重なった（一意な key を先に確保された）のも、未設定を見て操作したのに設定があった、という
	// 画面が古い状況なので、先に変えられたにする
	if errors.Is(err, starterdeckset.ErrAlreadyCreated) {
		return ChangedStarterDeckSet{}, ErrRevisionConflict
	}
	return changed, revisionConflict(err)
}

// 1 つしかない集約を、一意な key から引く。まだ無ければ識別子を発行して key を確保する（同じトランザクションで設定を保存するため、
// 設定の無い集約は残らない）
func (c *StarterDeckSetHandler) load(ctx context.Context) (starterdeckset.ID, uuid.UUID, starterdeckset.State, error) {
	id, found, err := c.find(ctx)
	if err != nil {
		return 0, uuid.Nil, nil, err
	}
	if !found {
		id, publicID, err := c.identifier.Issue(ctx)
		if err != nil {
			return 0, uuid.Nil, nil, err
		}
		if err := c.create(ctx, id); err != nil {
			return 0, uuid.Nil, nil, err
		}
		return id, publicID, starterdeckset.InitialState(), nil
	}
	state, err := c.eventStore.Load(ctx, id)
	if err != nil {
		return 0, uuid.Nil, nil, err
	}
	return id, uuid.Nil, state, nil
}

// 未設定は 0（画面は未設定を revision 0 で読む）
func version(state starterdeckset.State) int64 {
	if set, ok := state.(starterdeckset.StarterDeckSet); ok {
		return set.Version
	}
	return 0
}

// 誤りは最初の 1 つで止めず、まとめて input.Errors で返す。項目名は presetDeckIds.<番号> のように一覧の位置を含める
func (c *StarterDeckSetHandler) copyPresetDecks(ctx context.Context, presetDeckIDs []uuid.UUID) ([]starterdeckset.Deck, error) {
	if len(presetDeckIDs) != starterdeckset.RequiredDeckCount {
		return nil, input.Errors{&input.OutOfRange{Field: "presetDeckIds", Min: starterdeckset.RequiredDeckCount, Max: starterdeckset.RequiredDeckCount}}
	}
	invalid := input.Errors{}
	decks := make([]starterdeckset.Deck, 0, len(presetDeckIDs))
	seen := make(map[uuid.UUID]bool, len(presetDeckIDs))
	for i, publicID := range presetDeckIDs {
		field := fmt.Sprintf("presetDeckIds.%d", i)
		if seen[publicID] {
			invalid = append(invalid, &input.Duplicated{Field: field})
			continue
		}
		seen[publicID] = true
		deck, err := c.loadPresetDeck(ctx, publicID)
		if errors.Is(err, ErrPresetDeckNotFound) {
			invalid = append(invalid, &input.NotFound{Field: field})
			continue
		}
		if err != nil {
			return nil, err
		}
		copied, err := starterdeckset.NewDeck(deck)
		if errors.Is(err, starterdeckset.ErrDeckNotSelectable) {
			invalid = append(invalid, &input.TotalMismatch{Field: field, Required: presetdeck.MaxTotalCount})
			continue
		}
		if err != nil {
			return nil, err
		}
		decks = append(decks, copied)
	}
	if len(invalid) > 0 {
		return nil, invalid
	}
	return decks, nil
}

// 無い・削除したなら ErrPresetDeckNotFound
func (c *StarterDeckSetHandler) loadPresetDeck(ctx context.Context, publicID uuid.UUID) (presetdeck.PresetDeck, error) {
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
