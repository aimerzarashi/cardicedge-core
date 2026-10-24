// 開発で触る見本のマスタ（カード・プリセットデッキ・スターターデッキセット）を、studio のコマンドで登録する。
// 初期オペレーター（STUDIO_INITIAL_OPERATOR_EMAIL）として呼ぶ。開発の道具で、本番のコード（cmd/app・internal）からは import しない（core#37）
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

const projectionTimeout = 30 * time.Second

func main() {
	env := flag.String("env", "STUDIO_DB_DSN", "つなぎ先を持つ環境変数の名前")
	flag.Parse()
	if err := run(context.Background(), *env); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, env string) error {
	m, err := loadMaster(sampleMasterJSON)
	if err != nil {
		return fmt.Errorf("sample master: %w", err)
	}
	dsn := os.Getenv(env)
	if dsn == "" {
		return fmt.Errorf("-env の環境変数のつなぎ先が要る")
	}
	email := os.Getenv("STUDIO_INITIAL_OPERATOR_EMAIL")
	if email == "" {
		return fmt.Errorf("STUDIO_INITIAL_OPERATOR_EMAIL が要る")
	}
	db, err := rdb.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx = rdb.WithDB(ctx, db)
	registry, err := domaindef.Load(ctx)
	if err != nil {
		return err
	}
	address, err := operator.NewEmail(email)
	if err != nil {
		return err
	}
	operatorID, found, err := constraint.FindOperatorByEmail(ctx, address)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("オペレーター %s が登録されていない（core を一度起動して初期オペレーターを登録する）", email)
	}
	ctx = authentication.WithOperator(ctx, operatorID)
	return newSeeder(registry).seed(ctx, m)
}

// studio.New は部品を外に出さないため、tests/testhelper と同じく、ここで組み立てる
type seeder struct {
	creatures       *command.CreatureHandler
	items           *command.ItemHandler
	spells          *command.SpellHandler
	presetDecks     *command.PresetDeckHandler
	starterDeckSets *command.StarterDeckSetHandler
	cards           *query.CardHandler
	presetDeckList  *query.PresetDeckHandler
	starterDeckSet  *query.StarterDeckSetHandler
}

func newSeeder(registry *domaindef.Registry) *seeder {
	now := func() time.Time {
		return time.Now().UTC().Truncate(time.Microsecond)
	}
	entityRefs := identifier.NewEntityRefResolver(registry)
	cardReader := repository.NewCardReader()
	presetDeckReader := repository.NewPresetDeckReader()
	presetDeckStore := eventstore.New(registry, presetdeck.NewID, presetdeck.NewEventType, presetdeck.InitialState, presetdeck.Evolve)
	presetDeckIdentifier := identifier.New(registry, presetdeck.NewID)
	return &seeder{
		creatures: command.NewCreatureHandler(
			eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve),
			identifier.New(registry, creature.NewID),
			constraint.UpdateCreatureName,
			now,
		),
		items: command.NewItemHandler(
			eventstore.New(registry, item.NewID, item.NewEventType, item.InitialState, item.Evolve),
			identifier.New(registry, item.NewID),
			constraint.UpdateItemName,
			now,
		),
		spells: command.NewSpellHandler(
			eventstore.New(registry, spell.NewID, spell.NewEventType, spell.InitialState, spell.Evolve),
			identifier.New(registry, spell.NewID),
			constraint.UpdateSpellName,
			now,
		),
		presetDecks: command.NewPresetDeckHandler(
			presetDeckStore,
			presetDeckIdentifier,
			entityRefs,
			constraint.UpdatePresetDeckName,
			constraint.UpdatePresetDeckCardReferences,
			now,
		),
		starterDeckSets: command.NewStarterDeckSetHandler(
			eventstore.New(registry, starterdeckset.NewID, starterdeckset.NewEventType, starterdeckset.InitialState, starterdeckset.Evolve),
			identifier.New(registry, starterdeckset.NewID),
			constraint.FindStarterDeckSet,
			constraint.CreateStarterDeckSet,
			constraint.UpdateStarterDeckSetCardReferences,
			presetDeckStore,
			presetDeckIdentifier,
			now,
		),
		cards:          query.NewCardHandler(cardReader.List, cardReader.Get),
		presetDeckList: query.NewPresetDeckHandler(presetDeckReader.List, presetDeckReader.Get),
		starterDeckSet: query.NewStarterDeckSetHandler(repository.NewStarterDeckSetReader().Get, presetDeckReader.Revisions),
	}
}

func (s *seeder) seed(ctx context.Context, m master) error {
	set, err := s.starterDeckSet.Get(ctx)
	if err != nil {
		return err
	}
	// 何度流しても壊れないよう、設定済みなら何もしない（途中で止まったときは、使われている名前のものを引いて続ける）
	if set.Revision > 0 {
		slog.InfoContext(ctx, "starter deck set is already set")
		return nil
	}
	cardIDs, err := s.seedCards(ctx, m.Cards)
	if err != nil {
		return err
	}
	deckIDs, err := s.seedPresetDecks(ctx, m.PresetDecks, cardIDs)
	if err != nil {
		return err
	}
	ids := make([]uuid.UUID, 0, len(m.StarterDeckSet))
	for _, name := range m.StarterDeckSet {
		ids = append(ids, deckIDs[name])
	}
	// 先に変えられたなら、止まる前に設定し終えていて、read model がまだ追いついていないだけ（下で追いつくのを待つ）
	_, err = s.starterDeckSets.Set(ctx, 0, ids)
	if err != nil && !errors.Is(err, command.ErrRevisionConflict) {
		return fmt.Errorf("set starter deck set: %w", err)
	}
	if err == nil {
		slog.InfoContext(ctx, "set starter deck set")
	}
	// 続けて make publish-master を流せるよう、core の投影が read model を書くまで待つ
	return waitFor(ctx, "starter_deck_sets", func() (bool, error) {
		set, err := s.starterDeckSet.Get(ctx)
		return set.Revision > 0, err
	})
}

// カードの名前（表示する値）から公開 ID を引く表を返す
func (s *seeder) seedCards(ctx context.Context, cards []masterCard) (map[string]uuid.UUID, error) {
	ids := make(map[string]uuid.UUID, len(cards))
	existing, err := s.listCards(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range cards {
		if id, ok := existing[card.NormalizeName(c.Name)]; ok {
			ids[c.Name] = id
			continue
		}
		id, err := s.registerCard(ctx, c)
		if errors.Is(err, command.ErrCardNameAlreadyInUse) {
			// 登録済みだが read model にまだ無い（投影の遅れ）
			err = waitFor(ctx, "cards", func() (bool, error) {
				existing, err = s.listCards(ctx)
				_, ok := existing[card.NormalizeName(c.Name)]
				return ok, err
			})
			id = existing[card.NormalizeName(c.Name)]
		}
		if err != nil {
			return nil, fmt.Errorf("register card %q: %w", c.Name, err)
		}
		ids[c.Name] = id
		slog.InfoContext(ctx, "registered card", "type", c.Type, "name", c.Name)
	}
	return ids, nil
}

func (s *seeder) registerCard(ctx context.Context, c masterCard) (uuid.UUID, error) {
	switch c.Type {
	case "creature":
		registered, err := s.creatures.Register(ctx, c.Name)
		return registered.ID, err
	case "item":
		registered, err := s.items.Register(ctx, c.Name)
		return registered.ID, err
	default:
		registered, err := s.spells.Register(ctx, c.Name)
		return registered.ID, err
	}
}

// 正規化した名前（重複の判定と同じ）から公開 ID を引く表
func (s *seeder) listCards(ctx context.Context) (map[string]uuid.UUID, error) {
	ids := map[string]uuid.UUID{}
	var after *string
	for {
		first := pagination.MaxFirst
		page, err := s.cards.List(ctx, query.CardFilter{}, &first, after)
		if err != nil {
			return nil, err
		}
		for _, edge := range page.Edges {
			ids[card.NormalizeName(edge.Node.Name)] = edge.Node.ID
		}
		if !page.PageInfo.HasNextPage {
			return ids, nil
		}
		after = page.PageInfo.EndCursor
	}
}

func (s *seeder) seedPresetDecks(ctx context.Context, decks []masterPresetDeck, cardIDs map[string]uuid.UUID) (map[string]uuid.UUID, error) {
	ids := make(map[string]uuid.UUID, len(decks))
	existing, err := s.listPresetDecks(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range decks {
		counts := make([]command.CardCountInput, 0, len(d.Cards))
		for _, e := range d.Cards {
			counts = append(counts, command.CardCountInput{CardID: cardIDs[e.Card], Count: e.Count})
		}
		key := card.NormalizeName(d.Name)
		current, ok := existing[key]
		if !ok {
			registered, err := s.presetDecks.Register(ctx, d.Name)
			if errors.Is(err, command.ErrPresetDeckNameAlreadyInUse) {
				err = waitFor(ctx, "preset_decks", func() (bool, error) {
					existing, err = s.listPresetDecks(ctx)
					_, ok := existing[key]
					return ok, err
				})
				current = existing[key]
			} else if err == nil {
				current = query.PresetDeck{ID: registered.ID, Revision: registered.Revision}
			}
			if err != nil {
				return nil, fmt.Errorf("register preset deck %q: %w", d.Name, err)
			}
			slog.InfoContext(ctx, "registered preset deck", "name", d.Name)
		}
		ids[d.Name] = current.ID
		if err := s.setCardCounts(ctx, d.Name, current, counts); err != nil {
			return nil, fmt.Errorf("set card counts of preset deck %q: %w", d.Name, err)
		}
	}
	return ids, nil
}

// 枚数まで入っているデッキは、途中で止まる前に済んでいる。read model は投影の遅れで古いことがあり（止まった直後に流し直すと、
// 枚数を入れたイベントがまだ投影されていない）、その revision で入れると先に変えられたになるため、追いつくのを待って読み直す
func (s *seeder) setCardCounts(ctx context.Context, name string, current query.PresetDeck, counts []command.CardCountInput) error {
	key := card.NormalizeName(name)
	for {
		if current.TotalCount == deckTotalCount {
			return nil
		}
		_, err := s.presetDecks.SetCardCounts(ctx, current.ID, current.Revision, counts)
		if !errors.Is(err, command.ErrRevisionConflict) {
			if err == nil {
				slog.InfoContext(ctx, "set card counts", "name", name)
			}
			return err
		}
		stale := current.Revision
		if err := waitFor(ctx, "preset_decks", func() (bool, error) {
			existing, err := s.listPresetDecks(ctx)
			current = existing[key]
			return current.Revision > stale, err
		}); err != nil {
			return err
		}
	}
}

func (s *seeder) listPresetDecks(ctx context.Context) (map[string]query.PresetDeck, error) {
	decks := map[string]query.PresetDeck{}
	var after *string
	for {
		first := pagination.MaxFirst
		page, err := s.presetDeckList.List(ctx, query.PresetDeckFilter{}, &first, after)
		if err != nil {
			return nil, err
		}
		for _, edge := range page.Edges {
			decks[card.NormalizeName(edge.Node.Name)] = edge.Node
		}
		if !page.PageInfo.HasNextPage {
			return decks, nil
		}
		after = page.PageInfo.EndCursor
	}
}

func waitFor(ctx context.Context, what string, done func() (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, projectionTimeout)
	defer cancel()
	for {
		ok, err := done()
		if err != nil || ok {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s の read model が %s 以内にできなかった（core が動いているか確かめる）", what, projectionTimeout)
		case <-time.After(200 * time.Millisecond):
		}
	}
}
