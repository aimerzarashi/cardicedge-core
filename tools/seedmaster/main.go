// 開発で触る見本のマスタ（カード・プリセットデッキ・スターターデッキ）を、studio のコマンドで登録する（カードとスターターデッキは公開中にする）。
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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
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
	gameRules       *command.GameRulesHandler
	creatures       *command.CreatureHandler
	items           *command.ItemHandler
	spells          *command.SpellHandler
	cardStatuses    *command.CardHandler
	presetDecks     *command.PresetDeckHandler
	starterDecks    *command.StarterDeckHandler
	cards           *query.CardHandler
	presetDeckList  *query.PresetDeckHandler
	starterDeckList *query.StarterDeckHandler
}

func newSeeder(registry *domaindef.Registry) *seeder {
	now := func() time.Time {
		return time.Now().UTC().Truncate(time.Microsecond)
	}
	entityRefs := identifier.NewEntityRefResolver(registry)
	cardReader := repository.NewCardReader()
	presetDeckReader := repository.NewPresetDeckReader()
	starterDeckReader := repository.NewStarterDeckReader()
	creatureStore := eventstore.New(registry, creature.NewID, creature.NewEventType, creature.InitialState, creature.Evolve)
	itemStore := eventstore.New(registry, item.NewID, item.NewEventType, item.InitialState, item.Evolve)
	spellStore := eventstore.New(registry, spell.NewID, spell.NewEventType, spell.InitialState, spell.Evolve)
	creatures := command.NewCreatureHandler(creatureStore, identifier.New(registry, creature.NewID), constraint.UpdateCreatureName, now)
	items := command.NewItemHandler(itemStore, identifier.New(registry, item.NewID), constraint.UpdateItemName, now)
	spells := command.NewSpellHandler(spellStore, identifier.New(registry, spell.NewID), constraint.UpdateSpellName, now)
	presetDeckStore := eventstore.New(registry, presetdeck.NewID, presetdeck.NewEventType, presetdeck.InitialState, presetdeck.Evolve)
	presetDeckIdentifier := identifier.New(registry, presetdeck.NewID)
	gameRules := command.NewGameRulesHandler(
		eventstore.New(registry, gamerules.NewID, gamerules.NewEventType, gamerules.InitialState, gamerules.Evolve),
		identifier.New(registry, gamerules.NewID),
		constraint.FindGameRules,
		constraint.CreateGameRules,
		now,
	)
	return &seeder{
		gameRules:    gameRules,
		creatures:    creatures,
		items:        items,
		spells:       spells,
		cardStatuses: command.NewCardHandler(entityRefs, creatures, items, spells),
		presetDecks: command.NewPresetDeckHandler(
			presetDeckStore,
			presetDeckIdentifier,
			entityRefs,
			constraint.UpdatePresetDeckName,
			constraint.UpdatePresetDeckCardReferences,
			command.NewCardStatusReader(creatureStore, itemStore, spellStore).Statuses,
			gameRules.Get,
			now,
		),
		starterDecks: command.NewStarterDeckHandler(
			eventstore.New(registry, starterdeck.NewID, starterdeck.NewEventType, starterdeck.InitialState, starterdeck.Evolve),
			identifier.New(registry, starterdeck.NewID),
			entityRefs,
			constraint.UpdateStarterDeckName,
			constraint.UpdateStarterDeckCardReferences,
			command.NewCardStatusReader(creatureStore, itemStore, spellStore).Statuses,
			presetDeckStore,
			presetDeckIdentifier,
			constraint.FindGameRules,
			constraint.ReserveStarterDeckQuota,
			constraint.ReleaseStarterDeckQuota,
			gameRules.Get,
			now,
		),
		cards:           query.NewCardHandler(cardReader.List, cardReader.Get),
		presetDeckList:  query.NewPresetDeckHandler(presetDeckReader.List, presetDeckReader.Get),
		starterDeckList: query.NewStarterDeckHandler(starterDeckReader.List, starterDeckReader.Get, presetDeckReader.Revisions),
	}
}

func (s *seeder) seed(ctx context.Context, m master) error {
	if err := s.seedGameRules(ctx); err != nil {
		return err
	}
	cardIDs, err := s.seedCards(ctx, m.Cards)
	if err != nil {
		return err
	}
	deckIDs, err := s.seedPresetDecks(ctx, m.PresetDecks, cardIDs)
	if err != nil {
		return err
	}
	return s.seedStarterDecks(ctx, m.StarterDecks, deckIDs)
}

// プリセットデッキから写してスターターデッキを作り、公開する。何度流しても同じ結果になるよう、同じ名前のスターターデッキが
// あれば作らない。公開は、準備中のまま止まったものだけ続ける（使用不可にしたものを、流し直しで公開に戻さない）。
// 続けて make publish-master を流せるよう、core の投影が公開中を書くまで待つ
func (s *seeder) seedStarterDecks(ctx context.Context, names []string, presetDeckIDs map[string]uuid.UUID) error {
	existing, err := s.listStarterDecks(ctx)
	if err != nil {
		return err
	}
	for _, name := range names {
		key := domain.NormalizeName(name)
		current, ok := existing[key]
		if !ok {
			copied, err := s.starterDecks.RegisterFromPresetDeck(ctx, presetDeckIDs[name])
			if errors.Is(err, command.ErrStarterDeckNameAlreadyInUse) {
				// 作成済みだが read model にまだ無い（投影の遅れ）
				err = waitFor(ctx, "starter_decks", func() (bool, error) {
					existing, err = s.listStarterDecks(ctx)
					_, ok := existing[key]
					return ok, err
				})
				current = existing[key]
			} else if err == nil {
				current = query.StarterDeck{ID: copied.ID, Status: readmodel.StarterDeckStatusDraft, Revision: copied.Revision}
				slog.InfoContext(ctx, "registered starter deck", "name", name)
			}
			if err != nil {
				return fmt.Errorf("register starter deck %q: %w", name, err)
			}
		}
		if current.Status != readmodel.StarterDeckStatusDraft {
			continue
		}
		if err := s.publishStarterDeck(ctx, name, current); err != nil {
			return fmt.Errorf("publish starter deck %q: %w", name, err)
		}
	}
	return waitFor(ctx, "starter_decks", func() (bool, error) {
		existing, err := s.listStarterDecks(ctx)
		if err != nil {
			return false, err
		}
		for _, name := range names {
			if existing[domain.NormalizeName(name)].Status == readmodel.StarterDeckStatusDraft {
				return false, nil
			}
		}
		return true, nil
	})
}

// read model は投影の遅れで古いことがある（作った直後・止まった直後）。その revision で公開すると先に変えられたになるため、
// 追いつくのを待って読み直す（publishCard と同じ）
func (s *seeder) publishStarterDeck(ctx context.Context, name string, current query.StarterDeck) error {
	key := domain.NormalizeName(name)
	for {
		_, err := s.starterDecks.Publish(ctx, current.ID, current.Revision)
		if !errors.Is(err, command.ErrRevisionConflict) {
			if err == nil {
				slog.InfoContext(ctx, "published starter deck", "name", name)
			}
			return err
		}
		stale := current.Revision
		if err := waitFor(ctx, "starter_decks", func() (bool, error) {
			existing, err := s.listStarterDecks(ctx)
			current = existing[key]
			return current.Revision > stale, err
		}); err != nil {
			return err
		}
	}
}

// 正規化した名前（重複の判定と同じ）からスターターデッキを引く表
func (s *seeder) listStarterDecks(ctx context.Context) (map[string]query.StarterDeck, error) {
	decks := map[string]query.StarterDeck{}
	var after *string
	for {
		first := pagination.MaxFirst
		page, err := s.starterDeckList.List(ctx, query.StarterDeckFilter{}, &first, after)
		if err != nil {
			return nil, err
		}
		for _, edge := range page.Edges {
			decks[domain.NormalizeName(edge.Node.Name)] = edge.Node
		}
		if !page.PageInfo.HasNextPage {
			return decks, nil
		}
		after = page.PageInfo.EndCursor
	}
}

// カードの名前（表示する値）から公開 ID を引く表を返す。登録したカードは公開中にする（準備中のカードはデッキに入れられないため）
func (s *seeder) seedCards(ctx context.Context, cards []masterCard) (map[string]uuid.UUID, error) {
	ids := make(map[string]uuid.UUID, len(cards))
	existing, err := s.listCards(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range cards {
		key := domain.NormalizeName(c.Name)
		current, ok := existing[key]
		if !ok {
			registered, err := s.registerCard(ctx, c)
			if errors.Is(err, command.ErrCardNameAlreadyInUse) {
				// 登録済みだが read model にまだ無い（投影の遅れ）
				err = waitFor(ctx, "cards", func() (bool, error) {
					existing, err = s.listCards(ctx)
					_, ok := existing[key]
					return ok, err
				})
				current = existing[key]
			} else if err == nil {
				current = query.Card{ID: registered.ID, Status: readmodel.CardStatusDraft, Revision: registered.Revision}
				slog.InfoContext(ctx, "registered card", "type", c.Type, "name", c.Name)
			}
			if err != nil {
				return nil, fmt.Errorf("register card %q: %w", c.Name, err)
			}
		}
		if err := s.publishCard(ctx, c.Name, current); err != nil {
			return nil, fmt.Errorf("publish card %q: %w", c.Name, err)
		}
		ids[c.Name] = current.ID
	}
	return ids, nil
}

// 公開中のカードは何もしない（途中で止まる前に公開し終えている）。read model は投影の遅れで古いことがあり（止まった直後に
// 流し直すと、公開のイベントがまだ投影されていない）、その revision で公開すると先に変えられたになるため、追いつくのを待って読み直す
func (s *seeder) publishCard(ctx context.Context, name string, current query.Card) error {
	key := domain.NormalizeName(name)
	for {
		if current.Status == readmodel.CardStatusPublished {
			return nil
		}
		_, err := s.cardStatuses.Publish(ctx, current.ID, current.Revision)
		if !errors.Is(err, command.ErrRevisionConflict) {
			if err == nil {
				slog.InfoContext(ctx, "published card", "name", name)
			}
			return err
		}
		stale := current.Revision
		if err := waitFor(ctx, "cards", func() (bool, error) {
			existing, err := s.listCards(ctx)
			current = existing[key]
			return current.Revision > stale, err
		}); err != nil {
			return err
		}
	}
}

func (s *seeder) registerCard(ctx context.Context, c masterCard) (command.ChangedCard, error) {
	switch c.Type {
	case "creature":
		registered, err := s.creatures.Register(ctx, c.Name)
		return command.ChangedCard{ID: registered.ID, Revision: registered.Revision}, err
	case "item":
		registered, err := s.items.Register(ctx, c.Name)
		return command.ChangedCard{ID: registered.ID, Revision: registered.Revision}, err
	default:
		registered, err := s.spells.Register(ctx, c.Name)
		return command.ChangedCard{ID: registered.ID, Revision: registered.Revision}, err
	}
}

// 正規化した名前（重複の判定と同じ）からカードを引く表
func (s *seeder) listCards(ctx context.Context) (map[string]query.Card, error) {
	cards := map[string]query.Card{}
	var after *string
	for {
		first := pagination.MaxFirst
		page, err := s.cards.List(ctx, query.CardFilter{}, &first, after)
		if err != nil {
			return nil, err
		}
		for _, edge := range page.Edges {
			cards[domain.NormalizeName(edge.Node.Name)] = edge.Node
		}
		if !page.PageInfo.HasNextPage {
			return cards, nil
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
		key := domain.NormalizeName(d.Name)
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
	key := domain.NormalizeName(name)
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
			decks[domain.NormalizeName(edge.Node.Name)] = edge.Node
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

// core は起動時にゲーム全体のルールを作るので、ふだんは何もしない。core を起動する前に流しても、プリセットデッキの確かめ
// （ルールを読む）が動くよう、起動時と同じ関数で作る。続けて make publish-master を流せるよう、read model を待つ
func (s *seeder) seedGameRules(ctx context.Context) error {
	if _, err := s.gameRules.CreateInitial(ctx); err != nil {
		return fmt.Errorf("create game rules: %w", err)
	}
	return waitFor(ctx, "game_rules", func() (bool, error) {
		exec, err := rdb.GetExecutor(ctx)
		if err != nil {
			return false, err
		}
		return models.GameRules().Exists(ctx, exec)
	})
}
