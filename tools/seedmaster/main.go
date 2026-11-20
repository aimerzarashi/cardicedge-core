// 開発で触る見本のマスタ（カード・プリセットデッキ・配布デッキ）を、studio のコマンドで登録する（カードと配布デッキは公開中にする）。
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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
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
	gameRules      *command.GameRulesHandler
	studioRules    *command.StudioRulesHandler
	creatures      *command.CreatureHandler
	items          *command.ItemHandler
	spells         *command.SpellHandler
	cardCommands   *command.CardHandler
	presetDecks    *command.PresetDeckHandler
	grantDecks     *command.GrantDeckHandler
	boards         *command.BoardHandler
	npcAvatars     *command.NPCAvatarHandler
	npcDecks       *command.NPCDeckHandler
	npcMindsets    *command.NPCMindsetHandler
	starterSets    *command.StarterSetHandler
	cards          *query.CardHandler
	presetDeckList *query.PresetDeckHandler
	grantDeckList  *query.GrantDeckHandler
	boardList      *query.BoardHandler
	npcAvatarList  *query.NPCAvatarHandler
	npcDeckList    *query.NPCDeckHandler
	npcMindsetList *query.NPCMindsetHandler
	starterSetList *query.StarterSetHandler
}

func newSeeder(registry *domaindef.Registry) *seeder {
	now := func() time.Time {
		return time.Now().UTC().Truncate(time.Microsecond)
	}
	entityRefs := identifier.NewEntityRefResolver(registry)
	cardReader := repository.NewCardReader()
	presetDeckReader := repository.NewPresetDeckReader()
	grantDeckReader := repository.NewGrantDeckReader()
	boardReader := repository.NewBoardReader()
	npcAvatarReader := repository.NewNPCAvatarReader()
	npcDeckReader := repository.NewNPCDeckReader()
	npcMindsetReader := repository.NewNPCMindsetReader()
	starterSetReader := repository.NewStarterSetReader()
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
	studioRules := command.NewStudioRulesHandler(
		eventstore.New(registry, studiorules.NewID, studiorules.NewEventType, studiorules.InitialState, studiorules.Evolve),
		identifier.New(registry, studiorules.NewID),
		constraint.FindStudioRules,
		constraint.CreateStudioRules,
		now,
	)
	return &seeder{
		gameRules:    gameRules,
		studioRules:  studioRules,
		creatures:    creatures,
		items:        items,
		spells:       spells,
		cardCommands: command.NewCardHandler(entityRefs, creatures, items, spells),
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
		grantDecks: command.NewGrantDeckHandler(
			eventstore.New(registry, grantdeck.NewID, grantdeck.NewEventType, grantdeck.InitialState, grantdeck.Evolve),
			identifier.New(registry, grantdeck.NewID),
			entityRefs,
			constraint.UpdateGrantDeckName,
			constraint.UpdateGrantDeckCardReferences,
			command.NewCardStatusReader(creatureStore, itemStore, spellStore).Statuses,
			presetDeckStore,
			presetDeckIdentifier,
			gameRules.Get,
			now,
		),
		boards: command.NewBoardHandler(
			eventstore.New(registry, board.NewID, board.NewEventType, board.InitialState, board.Evolve),
			identifier.New(registry, board.NewID),
			constraint.UpdateBoardName,
			studioRules.Get,
			now,
		),
		starterSets: command.NewStarterSetHandler(
			eventstore.New(registry, starterset.NewID, starterset.NewEventType, starterset.InitialState, starterset.Evolve),
			identifier.New(registry, starterset.NewID),
			entityRefs,
			constraint.UpdateStarterSetName,
			constraint.UpdateStarterSetGrantDeckReferences,
			constraint.UpdateStarterSetBoardReferences,
			constraint.UpdateStarterSetNPCAvatarReferences,
			command.NewGrantDeckStatusReader(eventstore.New(registry, grantdeck.NewID, grantdeck.NewEventType, grantdeck.InitialState, grantdeck.Evolve)).Statuses,
			command.NewBoardStatusReader(eventstore.New(registry, board.NewID, board.NewEventType, board.InitialState, board.Evolve)).Statuses,
			command.NewNPCAvatarStatusReader(eventstore.New(registry, npcavatar.NewID, npcavatar.NewEventType, npcavatar.InitialState, npcavatar.Evolve)).Statuses,
			constraint.FindGameRules,
			constraint.ReserveStarterSetQuota,
			constraint.ReleaseStarterSetQuota,
			now,
		),
		npcAvatars: command.NewNPCAvatarHandler(
			eventstore.New(registry, npcavatar.NewID, npcavatar.NewEventType, npcavatar.InitialState, npcavatar.Evolve),
			identifier.New(registry, npcavatar.NewID),
			entityRefs,
			constraint.UpdateNPCAvatarName,
			constraint.UpdateNPCAvatarDeckReferences,
			command.NewNPCDeckStatusReader(eventstore.New(registry, npcdeck.NewID, npcdeck.NewEventType, npcdeck.InitialState, npcdeck.Evolve)).Statuses,
			constraint.UpdateNPCAvatarMindsetReferences,
			command.NewNPCMindsetStatusReader(eventstore.New(registry, npcmindset.NewID, npcmindset.NewEventType, npcmindset.InitialState, npcmindset.Evolve)).Statuses,
			now,
		),
		npcMindsets: command.NewNPCMindsetHandler(
			eventstore.New(registry, npcmindset.NewID, npcmindset.NewEventType, npcmindset.InitialState, npcmindset.Evolve),
			identifier.New(registry, npcmindset.NewID),
			constraint.UpdateNPCMindsetName,
			now,
		),
		npcDecks: command.NewNPCDeckHandler(
			eventstore.New(registry, npcdeck.NewID, npcdeck.NewEventType, npcdeck.InitialState, npcdeck.Evolve),
			identifier.New(registry, npcdeck.NewID),
			entityRefs,
			constraint.UpdateNPCDeckName,
			constraint.UpdateNPCDeckCardReferences,
			command.NewCardStatusReader(creatureStore, itemStore, spellStore).Statuses,
			presetDeckStore,
			presetDeckIdentifier,
			gameRules.Get,
			now,
		),
		cards:          query.NewCardHandler(cardReader.List, cardReader.Get),
		presetDeckList: query.NewPresetDeckHandler(presetDeckReader.List, presetDeckReader.Get),
		grantDeckList:  query.NewGrantDeckHandler(grantDeckReader.List, grantDeckReader.Get),
		boardList:      query.NewBoardHandler(boardReader.List, boardReader.Get),
		npcAvatarList:  query.NewNPCAvatarHandler(npcAvatarReader.List, npcAvatarReader.Get),
		npcDeckList:    query.NewNPCDeckHandler(npcDeckReader.List, npcDeckReader.Get),
		npcMindsetList: query.NewNPCMindsetHandler(npcMindsetReader.List, npcMindsetReader.Get),
		starterSetList: query.NewStarterSetHandler(starterSetReader.List, starterSetReader.Get),
	}
}

func (s *seeder) seed(ctx context.Context, m master) error {
	if err := s.seedGameRules(ctx); err != nil {
		return err
	}
	if err := s.seedStudioRules(ctx); err != nil {
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
	if err := s.seedGrantDecks(ctx, m.GrantDecks, deckIDs); err != nil {
		return err
	}
	if err := s.seedBoards(ctx, m.Boards); err != nil {
		return err
	}
	// NPC のアバターは、公開中の NPC のデッキと思考パターンを割り当てて公開するため、NPC のデッキ・思考パターンより後に作る
	npcDeckIDs, err := s.seedNPCDecks(ctx, m.NPCDecks, deckIDs)
	if err != nil {
		return err
	}
	npcMindsetIDs, err := s.seedNPCMindsets(ctx, m.NPCMindsets)
	if err != nil {
		return err
	}
	if err := s.seedNPCAvatars(ctx, m.NPCAvatars, npcDeckIDs, npcMindsetIDs); err != nil {
		return err
	}
	// スターターセットは、公開中の配布デッキとボードを指定するため、どちらよりも後に作る
	return s.seedStarterSets(ctx, m.StarterSets)
}

// 登録してレイアウトと既定のルールを入れ、公開する。何度流しても同じ結果になるよう、同じ名前のボードがあれば登録しない。
// レイアウト・既定のルール・公開は、準備中のまま止まったものだけ続ける（公開中・使用不可のものは触らない。使用不可にしたものを、流し直しで公開に戻さない）。
// 続けて make publish-master を流せるよう、core の投影が公開中を書くまで待つ
func (s *seeder) seedBoards(ctx context.Context, boards []masterBoard) error {
	existing, err := s.listBoards(ctx)
	if err != nil {
		return err
	}
	for _, b := range boards {
		key := domain.NormalizeName(b.Name)
		current, ok := existing[key]
		if !ok {
			registered, err := s.boards.Register(ctx, b.Name)
			if errors.Is(err, command.ErrBoardNameAlreadyInUse) {
				// 登録済みだが read model にまだ無い（投影の遅れ）
				err = waitFor(ctx, "boards", func() (bool, error) {
					existing, err = s.listBoards(ctx)
					_, ok := existing[key]
					return ok, err
				})
				current = existing[key]
			} else if err == nil {
				current = query.Board{ID: registered.ID, Status: readmodel.BoardStatusDraft, Revision: registered.Revision}
				slog.InfoContext(ctx, "registered board", "name", b.Name)
			}
			if err != nil {
				return fmt.Errorf("register board %q: %w", b.Name, err)
			}
		}
		if current.Status != readmodel.BoardStatusDraft {
			continue
		}
		if err := s.publishBoard(ctx, b, current); err != nil {
			return fmt.Errorf("publish board %q: %w", b.Name, err)
		}
	}
	return waitFor(ctx, "boards", func() (bool, error) {
		existing, err := s.listBoards(ctx)
		if err != nil {
			return false, err
		}
		for _, b := range boards {
			// 無い（投影がまだ行を書いていない）ものも待つ。無いと状態は空の値で、準備中と比べるだけでは抜けてしまう
			published, ok := existing[domain.NormalizeName(b.Name)]
			if !ok || published.Status == readmodel.BoardStatusDraft {
				return false, nil
			}
		}
		return true, nil
	})
}

// レイアウトと既定のルールを入れてから公開する。入っていれば変えずに成功する（revision は進まない）ので、途中で止まった続きも
// 同じ手順でよい。read model は投影の遅れで古いことがあり、その revision で操作すると先に変えられたになるため、追いつくのを待って
// 読み直す（publishGrantDeck と同じ）
func (s *seeder) publishBoard(ctx context.Context, b masterBoard, current query.Board) error {
	key := domain.NormalizeName(b.Name)
	for {
		changed, err := s.boards.ChangeLayout(ctx, current.ID, current.Revision, b.layoutInput())
		if err == nil {
			changed, err = s.boards.ChangeDefaultRules(ctx, current.ID, changed.Revision, b.defaultRulesInput())
		}
		if err == nil {
			_, err = s.boards.Publish(ctx, current.ID, changed.Revision)
		}
		if !errors.Is(err, command.ErrRevisionConflict) {
			if err == nil {
				slog.InfoContext(ctx, "published board", "name", b.Name)
			}
			return err
		}
		stale := current.Revision
		if err := waitFor(ctx, "boards", func() (bool, error) {
			existing, err := s.listBoards(ctx)
			current = existing[key]
			return current.Revision > stale, err
		}); err != nil {
			return err
		}
	}
}

// 正規化した名前（重複の判定と同じ）からボードを引く表
func (s *seeder) listBoards(ctx context.Context) (map[string]query.Board, error) {
	boards := map[string]query.Board{}
	var after *string
	for {
		first := pagination.MaxFirst
		page, err := s.boardList.List(ctx, query.BoardFilter{}, &first, after)
		if err != nil {
			return nil, err
		}
		for _, edge := range page.Edges {
			boards[domain.NormalizeName(edge.Node.Name)] = edge.Node
		}
		if !page.PageInfo.HasNextPage {
			return boards, nil
		}
		after = page.PageInfo.EndCursor
	}
}

// 登録して配るデッキと解放するボードを指定し、公開する。何度流しても同じ結果になるよう、同じ名前のスターターセットがあれば登録しない。
// 指定と公開は、準備中のまま止まったものだけ続ける（公開中・使用不可のものは触らない。使用不可にしたものを、流し直しで公開に戻さない）。
// 指定は同じ集まりなら何も起きないため、準備中のものには毎回呼ぶ。公開中にできるのは同時に 1 つだけなので、ほかのスターターセットが
// 公開中だと公開で止まる（公開中のものを、見本のために黙って使用不可にしない）
func (s *seeder) seedStarterSets(ctx context.Context, sets []masterStarterSet) error {
	if len(sets) == 0 {
		return nil
	}
	existing, err := s.listStarterSets(ctx)
	if err != nil {
		return err
	}
	grantDecks, err := s.listGrantDecks(ctx)
	if err != nil {
		return err
	}
	boards, err := s.listBoards(ctx)
	if err != nil {
		return err
	}
	npcAvatars, err := s.listNPCAvatars(ctx)
	if err != nil {
		return err
	}
	for _, set := range sets {
		name := set.Name
		key := domain.NormalizeName(name)
		current, ok := existing[key]
		if !ok {
			registered, err := s.starterSets.Register(ctx, name)
			if errors.Is(err, command.ErrStarterSetNameAlreadyInUse) {
				// 登録済みだが read model にまだ無い（投影の遅れ）
				err = waitFor(ctx, "starter_sets", func() (bool, error) {
					existing, err = s.listStarterSets(ctx)
					_, ok := existing[key]
					return ok, err
				})
				current = existing[key]
			} else if err == nil {
				current = query.StarterSet{ID: registered.ID, Status: readmodel.StarterSetStatusDraft, Revision: registered.Revision}
				slog.InfoContext(ctx, "registered starter set", "name", name)
			}
			if err != nil {
				return fmt.Errorf("register starter set %q: %w", name, err)
			}
		}
		if current.Status != readmodel.StarterSetStatusDraft {
			continue
		}
		grantDeckIDs := make([]uuid.UUID, 0, len(set.GrantDecks))
		for _, grantDeck := range set.GrantDecks {
			grantDeckIDs = append(grantDeckIDs, grantDecks[domain.NormalizeName(grantDeck)].ID)
		}
		boardIDs := make([]uuid.UUID, 0, len(set.UnlockBoards))
		for _, b := range set.UnlockBoards {
			boardIDs = append(boardIDs, boards[domain.NormalizeName(b)].ID)
		}
		npcAvatarIDs := make([]uuid.UUID, 0, len(set.UnlockNPCAvatars))
		for _, a := range set.UnlockNPCAvatars {
			npcAvatarIDs = append(npcAvatarIDs, npcAvatars[domain.NormalizeName(a)].ID)
		}
		assigned, err := s.changeStarterSet(ctx, name, current, func(revision int64) (command.ChangedStarterSet, error) {
			changed, err := s.starterSets.SetGrantDecks(ctx, current.ID, revision, grantDeckIDs)
			if err == nil && changed.Revision != revision {
				slog.InfoContext(ctx, "assigned grant decks to starter set", "name", name, "grant_decks", set.GrantDecks)
			}
			return changed, err
		})
		if err != nil {
			return fmt.Errorf("assign grant decks to starter set %q: %w", name, err)
		}
		current.Revision = assigned.Revision
		assigned, err = s.changeStarterSet(ctx, name, current, func(revision int64) (command.ChangedStarterSet, error) {
			changed, err := s.starterSets.SetUnlockBoards(ctx, current.ID, revision, boardIDs)
			if err == nil && changed.Revision != revision {
				slog.InfoContext(ctx, "assigned unlock boards to starter set", "name", name, "unlock_boards", set.UnlockBoards)
			}
			return changed, err
		})
		if err != nil {
			return fmt.Errorf("assign unlock boards to starter set %q: %w", name, err)
		}
		current.Revision = assigned.Revision
		assigned, err = s.changeStarterSet(ctx, name, current, func(revision int64) (command.ChangedStarterSet, error) {
			changed, err := s.starterSets.SetUnlockNPCAvatars(ctx, current.ID, revision, npcAvatarIDs)
			if err == nil && changed.Revision != revision {
				slog.InfoContext(ctx, "assigned unlock npc avatars to starter set", "name", name, "unlock_npc_avatars", set.UnlockNPCAvatars)
			}
			return changed, err
		})
		if err != nil {
			return fmt.Errorf("assign unlock npc avatars to starter set %q: %w", name, err)
		}
		current.Revision = assigned.Revision
		if _, err := s.changeStarterSet(ctx, name, current, func(revision int64) (command.ChangedStarterSet, error) {
			changed, err := s.starterSets.Publish(ctx, current.ID, revision)
			if err == nil {
				slog.InfoContext(ctx, "published starter set", "name", name)
			}
			return changed, err
		}); err != nil {
			return fmt.Errorf("publish starter set %q: %w", name, err)
		}
	}
	return waitFor(ctx, "starter_sets", func() (bool, error) {
		existing, err := s.listStarterSets(ctx)
		if err != nil {
			return false, err
		}
		for _, set := range sets {
			// 無い（投影がまだ行を書いていない）ものも待つ。無いと状態は空の値で、準備中と比べるだけでは抜けてしまう
			published, ok := existing[domain.NormalizeName(set.Name)]
			if !ok || published.Status == readmodel.StarterSetStatusDraft {
				return false, nil
			}
		}
		return true, nil
	})
}

// read model は投影の遅れで古いことがあり、その revision で操作すると先に変えられたになるため、追いつくのを待って
// 読み直す（changeNPCAvatar と同じ）
func (s *seeder) changeStarterSet(ctx context.Context, name string, current query.StarterSet, change func(revision int64) (command.ChangedStarterSet, error)) (command.ChangedStarterSet, error) {
	key := domain.NormalizeName(name)
	for {
		changed, err := change(current.Revision)
		if !errors.Is(err, command.ErrRevisionConflict) {
			return changed, err
		}
		stale := current.Revision
		if err := waitFor(ctx, "starter_sets", func() (bool, error) {
			existing, err := s.listStarterSets(ctx)
			current = existing[key]
			return current.Revision > stale, err
		}); err != nil {
			return command.ChangedStarterSet{}, err
		}
	}
}

// 正規化した名前（重複の判定と同じ）からスターターセットを引く表
func (s *seeder) listStarterSets(ctx context.Context) (map[string]query.StarterSet, error) {
	starterSets := map[string]query.StarterSet{}
	var after *string
	for {
		first := pagination.MaxFirst
		page, err := s.starterSetList.List(ctx, query.StarterSetFilter{}, &first, after)
		if err != nil {
			return nil, err
		}
		for _, edge := range page.Edges {
			starterSets[domain.NormalizeName(edge.Node.Name)] = edge.Node
		}
		if !page.PageInfo.HasNextPage {
			return starterSets, nil
		}
		after = page.PageInfo.EndCursor
	}
}

// 登録して使うデッキを割り当て、公開する。何度流しても同じ結果になるよう、同じ名前の NPC のアバターがあれば登録しない。
// 割り当てと公開は、準備中のまま止まったものだけ続ける（公開中・使用不可のものは触らない。使用不可にしたものを、流し直しで公開に戻さない。
// すでにデッキや思考パターンを持たずに公開済みのものにも、後から割り当てない）。割り当ては同じ集まりなら何も起きないため、準備中のものには毎回呼ぶ
func (s *seeder) seedNPCAvatars(ctx context.Context, avatars []masterNPCAvatar, npcDeckIDs, npcMindsetIDs map[string]uuid.UUID) error {
	existing, err := s.listNPCAvatars(ctx)
	if err != nil {
		return err
	}
	for _, avatar := range avatars {
		name := avatar.Name
		key := domain.NormalizeName(name)
		current, ok := existing[key]
		if !ok {
			registered, err := s.npcAvatars.Register(ctx, name)
			if errors.Is(err, command.ErrNPCAvatarNameAlreadyInUse) {
				// 登録済みだが read model にまだ無い（投影の遅れ）
				err = waitFor(ctx, "npc_avatars", func() (bool, error) {
					existing, err = s.listNPCAvatars(ctx)
					_, ok := existing[key]
					return ok, err
				})
				current = existing[key]
			} else if err == nil {
				current = query.NPCAvatar{ID: registered.ID, Status: readmodel.NPCAvatarStatusDraft, Revision: registered.Revision}
				slog.InfoContext(ctx, "registered npc avatar", "name", name)
			}
			if err != nil {
				return fmt.Errorf("register npc avatar %q: %w", name, err)
			}
		}
		if current.Status != readmodel.NPCAvatarStatusDraft {
			continue
		}
		deckIDs := make([]uuid.UUID, 0, len(avatar.Decks))
		for _, deck := range avatar.Decks {
			deckIDs = append(deckIDs, npcDeckIDs[deck])
		}
		assigned, err := s.changeNPCAvatar(ctx, name, current, func(revision int64) (command.ChangedNPCAvatar, error) {
			changed, err := s.npcAvatars.SetDecks(ctx, current.ID, revision, deckIDs)
			if err == nil && changed.Revision != revision {
				slog.InfoContext(ctx, "assigned decks to npc avatar", "name", name, "decks", avatar.Decks)
			}
			return changed, err
		})
		if err != nil {
			return fmt.Errorf("assign decks to npc avatar %q: %w", name, err)
		}
		current.Revision = assigned.Revision
		mindsetIDs := make([]uuid.UUID, 0, len(avatar.Mindsets))
		for _, mindset := range avatar.Mindsets {
			mindsetIDs = append(mindsetIDs, npcMindsetIDs[mindset])
		}
		assigned, err = s.changeNPCAvatar(ctx, name, current, func(revision int64) (command.ChangedNPCAvatar, error) {
			changed, err := s.npcAvatars.SetMindsets(ctx, current.ID, revision, mindsetIDs)
			if err == nil && changed.Revision != revision {
				slog.InfoContext(ctx, "assigned mindsets to npc avatar", "name", name, "mindsets", avatar.Mindsets)
			}
			return changed, err
		})
		if err != nil {
			return fmt.Errorf("assign mindsets to npc avatar %q: %w", name, err)
		}
		current.Revision = assigned.Revision
		if _, err := s.changeNPCAvatar(ctx, name, current, func(revision int64) (command.ChangedNPCAvatar, error) {
			changed, err := s.npcAvatars.Publish(ctx, current.ID, revision)
			if err == nil {
				slog.InfoContext(ctx, "published npc avatar", "name", name)
			}
			return changed, err
		}); err != nil {
			return fmt.Errorf("publish npc avatar %q: %w", name, err)
		}
	}
	return waitFor(ctx, "npc_avatars", func() (bool, error) {
		existing, err := s.listNPCAvatars(ctx)
		if err != nil {
			return false, err
		}
		for _, avatar := range avatars {
			// 無い（投影がまだ行を書いていない）ものも待つ。無いと状態は空の値で、準備中と比べるだけでは抜けてしまう
			published, ok := existing[domain.NormalizeName(avatar.Name)]
			if !ok || published.Status == readmodel.NPCAvatarStatusDraft {
				return false, nil
			}
		}
		return true, nil
	})
}

// read model は投影の遅れで古いことがあり、その revision で操作すると先に変えられたになるため、追いつくのを待って
// 読み直す（publishBoard と同じ）
func (s *seeder) changeNPCAvatar(ctx context.Context, name string, current query.NPCAvatar, change func(revision int64) (command.ChangedNPCAvatar, error)) (command.ChangedNPCAvatar, error) {
	key := domain.NormalizeName(name)
	for {
		changed, err := change(current.Revision)
		if !errors.Is(err, command.ErrRevisionConflict) {
			return changed, err
		}
		stale := current.Revision
		if err := waitFor(ctx, "npc_avatars", func() (bool, error) {
			existing, err := s.listNPCAvatars(ctx)
			current = existing[key]
			return current.Revision > stale, err
		}); err != nil {
			return command.ChangedNPCAvatar{}, err
		}
	}
}

// 正規化した名前（重複の判定と同じ）から NPC のアバターを引く表
func (s *seeder) listNPCAvatars(ctx context.Context) (map[string]query.NPCAvatar, error) {
	npcAvatars := map[string]query.NPCAvatar{}
	var after *string
	for {
		first := pagination.MaxFirst
		page, err := s.npcAvatarList.List(ctx, query.NPCAvatarFilter{}, &first, after)
		if err != nil {
			return nil, err
		}
		for _, edge := range page.Edges {
			npcAvatars[domain.NormalizeName(edge.Node.Name)] = edge.Node
		}
		if !page.PageInfo.HasNextPage {
			return npcAvatars, nil
		}
		after = page.PageInfo.EndCursor
	}
}

// プリセットデッキから、元と同じ名前を指定して写して NPC のデッキを作り、公開する。何度流しても同じ結果になるよう、同じ名前の NPC のデッキが
// あれば作らない。公開は、準備中のまま止まったものだけ続ける（使用不可にしたものを、流し直しで公開に戻さない）。
// NPC のデッキは配布デッキと別の集まりなので、同じ名前でも重ならない
func (s *seeder) seedNPCDecks(ctx context.Context, names []string, presetDeckIDs map[string]uuid.UUID) (map[string]uuid.UUID, error) {
	existing, err := s.listNPCDecks(ctx)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		key := domain.NormalizeName(name)
		current, ok := existing[key]
		if !ok {
			copied, err := s.npcDecks.RegisterFromPresetDeck(ctx, presetDeckIDs[name], name)
			if errors.Is(err, command.ErrNPCDeckNameAlreadyInUse) {
				// 作成済みだが read model にまだ無い（投影の遅れ）
				err = waitFor(ctx, "npc_decks", func() (bool, error) {
					existing, err = s.listNPCDecks(ctx)
					_, ok := existing[key]
					return ok, err
				})
				current = existing[key]
			} else if err == nil {
				current = query.NPCDeck{ID: copied.ID, Status: readmodel.NPCDeckStatusDraft, Revision: copied.Revision}
				slog.InfoContext(ctx, "registered npc deck", "name", name)
			}
			if err != nil {
				return nil, fmt.Errorf("register npc deck %q: %w", name, err)
			}
		}
		if current.Status != readmodel.NPCDeckStatusDraft {
			continue
		}
		if err := s.publishNPCDeck(ctx, name, current); err != nil {
			return nil, fmt.Errorf("publish npc deck %q: %w", name, err)
		}
	}
	// 名前から公開 ID を引く表を返す。投影が公開中を書くまで待ってから作る（NPC のアバターの割り当ては、デッキの状態を集約から読む
	// ため待たなくてもよいが、続けて流す道具が公開中を読めるようにそろえる）
	ids := make(map[string]uuid.UUID, len(names))
	err = waitFor(ctx, "npc_decks", func() (bool, error) {
		existing, err := s.listNPCDecks(ctx)
		if err != nil {
			return false, err
		}
		for _, name := range names {
			// 無い（投影がまだ行を書いていない）ものも待つ。無いと状態は空の値で、準備中と比べるだけでは抜けてしまう
			npcDeck, ok := existing[domain.NormalizeName(name)]
			if !ok || npcDeck.Status == readmodel.NPCDeckStatusDraft {
				return false, nil
			}
			ids[name] = npcDeck.ID
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// read model は投影の遅れで古いことがある（作った直後・止まった直後）。その revision で公開すると先に変えられたになるため、
// 追いつくのを待って読み直す（publishCard と同じ）
func (s *seeder) publishNPCDeck(ctx context.Context, name string, current query.NPCDeck) error {
	key := domain.NormalizeName(name)
	for {
		_, err := s.npcDecks.Publish(ctx, current.ID, current.Revision)
		if !errors.Is(err, command.ErrRevisionConflict) {
			if err == nil {
				slog.InfoContext(ctx, "published npc deck", "name", name)
			}
			return err
		}
		stale := current.Revision
		if err := waitFor(ctx, "npc_decks", func() (bool, error) {
			existing, err := s.listNPCDecks(ctx)
			current = existing[key]
			return current.Revision > stale, err
		}); err != nil {
			return err
		}
	}
}

// 正規化した名前（重複の判定と同じ）から NPC のデッキを引く表
func (s *seeder) listNPCDecks(ctx context.Context) (map[string]query.NPCDeck, error) {
	npcDecks := map[string]query.NPCDeck{}
	var after *string
	for {
		first := pagination.MaxFirst
		page, err := s.npcDeckList.List(ctx, query.NPCDeckFilter{}, &first, after)
		if err != nil {
			return nil, err
		}
		for _, edge := range page.Edges {
			npcDecks[domain.NormalizeName(edge.Node.Name)] = edge.Node
		}
		if !page.PageInfo.HasNextPage {
			return npcDecks, nil
		}
		after = page.PageInfo.EndCursor
	}
}

// プリセットデッキから、元と同じ名前を指定して写して配布デッキを作り、公開する。何度流しても同じ結果になるよう、同じ名前の配布デッキが
// あれば作らない。公開は、準備中のまま止まったものだけ続ける（使用不可にしたものを、流し直しで公開に戻さない）。
// 続けて make publish-master を流せるよう、core の投影が公開中を書くまで待つ
func (s *seeder) seedGrantDecks(ctx context.Context, names []string, presetDeckIDs map[string]uuid.UUID) error {
	existing, err := s.listGrantDecks(ctx)
	if err != nil {
		return err
	}
	for _, name := range names {
		key := domain.NormalizeName(name)
		current, ok := existing[key]
		if !ok {
			copied, err := s.grantDecks.RegisterFromPresetDeck(ctx, presetDeckIDs[name], name)
			if errors.Is(err, command.ErrGrantDeckNameAlreadyInUse) {
				// 作成済みだが read model にまだ無い（投影の遅れ）
				err = waitFor(ctx, "grant_decks", func() (bool, error) {
					existing, err = s.listGrantDecks(ctx)
					_, ok := existing[key]
					return ok, err
				})
				current = existing[key]
			} else if err == nil {
				current = query.GrantDeck{ID: copied.ID, Status: readmodel.GrantDeckStatusDraft, Revision: copied.Revision}
				slog.InfoContext(ctx, "registered grant deck", "name", name)
			}
			if err != nil {
				return fmt.Errorf("register grant deck %q: %w", name, err)
			}
		}
		if current.Status != readmodel.GrantDeckStatusDraft {
			continue
		}
		if err := s.publishGrantDeck(ctx, name, current); err != nil {
			return fmt.Errorf("publish grant deck %q: %w", name, err)
		}
	}
	return waitFor(ctx, "grant_decks", func() (bool, error) {
		existing, err := s.listGrantDecks(ctx)
		if err != nil {
			return false, err
		}
		for _, name := range names {
			// 無い（投影がまだ行を書いていない）ものも待つ。無いと状態は空の値で、準備中と比べるだけでは抜けてしまう
			deck, ok := existing[domain.NormalizeName(name)]
			if !ok || deck.Status == readmodel.GrantDeckStatusDraft {
				return false, nil
			}
		}
		return true, nil
	})
}

// read model は投影の遅れで古いことがある（作った直後・止まった直後）。その revision で公開すると先に変えられたになるため、
// 追いつくのを待って読み直す（publishCard と同じ）
func (s *seeder) publishGrantDeck(ctx context.Context, name string, current query.GrantDeck) error {
	key := domain.NormalizeName(name)
	for {
		_, err := s.grantDecks.Publish(ctx, current.ID, current.Revision)
		if !errors.Is(err, command.ErrRevisionConflict) {
			if err == nil {
				slog.InfoContext(ctx, "published grant deck", "name", name)
			}
			return err
		}
		stale := current.Revision
		if err := waitFor(ctx, "grant_decks", func() (bool, error) {
			existing, err := s.listGrantDecks(ctx)
			current = existing[key]
			return current.Revision > stale, err
		}); err != nil {
			return err
		}
	}
}

// 正規化した名前（重複の判定と同じ）から配布デッキを引く表
func (s *seeder) listGrantDecks(ctx context.Context) (map[string]query.GrantDeck, error) {
	decks := map[string]query.GrantDeck{}
	var after *string
	for {
		first := pagination.MaxFirst
		page, err := s.grantDeckList.List(ctx, query.GrantDeckFilter{}, &first, after)
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
		if current, err = s.changeCardSpec(ctx, c, current); err != nil {
			return nil, fmt.Errorf("change card spec %q: %w", c.Name, err)
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
		_, err := s.cardCommands.Publish(ctx, current.ID, current.Revision)
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

// 準備中のカードに見本の性能を入れ、変えた後の revision を持たせて返す（同じ値なら変わらず、revision も進まない）。準備中でなければ
// 何もしない（公開した後は性能を変えられない）。read model は投影の遅れで古いことがあり（止まった直後に流し直すと、性能の変更・公開の
// イベントがまだ投影されていない）、その revision で変えると先に変えられたになるため、publishCard と同じく追いつくのを待って読み直す
func (s *seeder) changeCardSpec(ctx context.Context, c masterCard, current query.Card) (query.Card, error) {
	key := domain.NormalizeName(c.Name)
	for {
		if current.Status != readmodel.CardStatusDraft {
			return current, nil
		}
		changed, err := s.cardCommands.ChangeCost(ctx, current.ID, current.Revision, c.Cost)
		if err == nil {
			changed, err = s.cardCommands.ChangeRarity(ctx, changed.ID, changed.Revision, c.Rarity)
		}
		// 属性・攻撃力・体力・効果はクリーチャー（効果はアイテムも）（validate が、クリーチャーには必ず書いてあることを確かめている）
		if err == nil && c.Type == "creature" {
			changed, err = s.creatures.ChangeElements(ctx, changed.ID, changed.Revision, c.Elements)
		}
		if err == nil && c.Type == "creature" {
			changed, err = s.creatures.ChangeStats(ctx, changed.ID, changed.Revision, *c.Attack, *c.HitPoints)
		}
		if err == nil && c.Type == "creature" {
			changed, err = s.creatures.ChangeEffects(ctx, changed.ID, changed.Revision, masterEffects(c.Effects))
		}
		if err == nil && c.Type == "creature" {
			changed, err = s.creatures.ChangeItemRestrictions(ctx, changed.ID, changed.Revision, c.ItemRestrictions)
		}
		if err == nil && c.Type == "creature" {
			changed, err = s.creatures.ChangePlacementRestrictions(ctx, changed.ID, changed.Revision, c.PlacementRestrictions)
		}
		// 分類と効果はアイテムだけ（validate が、アイテムには必ず書いてあることを確かめている）。効果は分類で持てるものだけなので、分類を先に変える
		if err == nil && c.Type == "item" {
			changed, err = s.items.ChangeCategory(ctx, changed.ID, changed.Revision, c.Category)
		}
		if err == nil && c.Type == "item" {
			changed, err = s.items.ChangeEffects(ctx, changed.ID, changed.Revision, masterEffects(c.Effects))
		}
		// 効果はスペルだけ（書いていなければ効果なしのまま）
		if err == nil && c.Type == "spell" {
			changed, err = s.spells.ChangeEffect(ctx, changed.ID, changed.Revision, c.spellEffect())
		}
		if err == nil {
			current.Revision = changed.Revision
			return current, nil
		}
		if !errors.Is(err, command.ErrRevisionConflict) {
			return query.Card{}, err
		}
		stale := current.Revision
		if err := waitFor(ctx, "cards", func() (bool, error) {
			existing, err := s.listCards(ctx)
			current = existing[key]
			return current.Revision > stale, err
		}); err != nil {
			return query.Card{}, err
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

// ボードの登録がルールを読むため、core を起動する前に流しても動くよう、起動時と同じ関数で作る。
// game に写さない（publish-master の対象ではない）ので、read model は待たない
func (s *seeder) seedStudioRules(ctx context.Context) error {
	if _, err := s.studioRules.CreateInitial(ctx); err != nil {
		return fmt.Errorf("create studio rules: %w", err)
	}
	return nil
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

// 登録して採点基準ごとの重みを入れ、公開する。何度流しても同じ結果になるよう、同じ名前の思考パターンがあれば登録しない。
// 重みの変更と公開は、準備中のまま止まったものだけ続ける（公開中・使用不可のものは触らない。使用不可にしたものを、流し直しで公開に戻さない）。
// 行動の種類ごとの重みは、登録のときの値（すべて 1）のままにする。名前から公開 ID を引く表を返す
func (s *seeder) seedNPCMindsets(ctx context.Context, mindsets []masterNPCMindset) (map[string]uuid.UUID, error) {
	existing, err := s.listNPCMindsets(ctx)
	if err != nil {
		return nil, err
	}
	for _, mindset := range mindsets {
		name := mindset.Name
		key := domain.NormalizeName(name)
		current, ok := existing[key]
		if !ok {
			registered, err := s.npcMindsets.Register(ctx, name)
			if errors.Is(err, command.ErrNPCMindsetNameAlreadyInUse) {
				// 登録済みだが read model にまだ無い（投影の遅れ）
				err = waitFor(ctx, "npc_mindsets", func() (bool, error) {
					existing, err = s.listNPCMindsets(ctx)
					_, ok := existing[key]
					return ok, err
				})
				current = existing[key]
			} else if err == nil {
				current = query.NPCMindset{ID: registered.ID, Status: readmodel.NPCMindsetStatusDraft, Revision: registered.Revision}
				slog.InfoContext(ctx, "registered npc mindset", "name", name)
			}
			if err != nil {
				return nil, fmt.Errorf("register npc mindset %q: %w", name, err)
			}
		}
		if current.Status != readmodel.NPCMindsetStatusDraft {
			continue
		}
		weights := make([]command.CriterionWeightInput, 0, len(npcmindset.Criteria()))
		for _, criterion := range npcmindset.Criteria() {
			weights = append(weights, command.CriterionWeightInput{Criterion: string(criterion), Weight: mindset.CriterionWeights[string(criterion)]})
		}
		changed, err := s.changeNPCMindset(ctx, name, current, func(revision int64) (command.ChangedNPCMindset, error) {
			changed, err := s.npcMindsets.SetCriterionWeights(ctx, current.ID, revision, weights)
			if err == nil && changed.Revision != revision {
				slog.InfoContext(ctx, "set criterion weights of npc mindset", "name", name, "criterion_weights", mindset.CriterionWeights)
			}
			return changed, err
		})
		if err != nil {
			return nil, fmt.Errorf("set criterion weights of npc mindset %q: %w", name, err)
		}
		current.Revision = changed.Revision
		if _, err := s.changeNPCMindset(ctx, name, current, func(revision int64) (command.ChangedNPCMindset, error) {
			changed, err := s.npcMindsets.Publish(ctx, current.ID, revision)
			if err == nil {
				slog.InfoContext(ctx, "published npc mindset", "name", name)
			}
			return changed, err
		}); err != nil {
			return nil, fmt.Errorf("publish npc mindset %q: %w", name, err)
		}
	}
	ids := make(map[string]uuid.UUID, len(mindsets))
	err = waitFor(ctx, "npc_mindsets", func() (bool, error) {
		existing, err := s.listNPCMindsets(ctx)
		if err != nil {
			return false, err
		}
		for _, mindset := range mindsets {
			// 無い（投影がまだ行を書いていない）ものも待つ。無いと状態は空の値で、準備中と比べるだけでは抜けてしまう
			published, ok := existing[domain.NormalizeName(mindset.Name)]
			if !ok || published.Status == readmodel.NPCMindsetStatusDraft {
				return false, nil
			}
			ids[mindset.Name] = published.ID
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// read model は投影の遅れで古いことがあり、その revision で操作すると先に変えられたになるため、追いつくのを待って読み直す（changeNPCAvatar と同じ）
func (s *seeder) changeNPCMindset(ctx context.Context, name string, current query.NPCMindset, change func(revision int64) (command.ChangedNPCMindset, error)) (command.ChangedNPCMindset, error) {
	key := domain.NormalizeName(name)
	for {
		changed, err := change(current.Revision)
		if !errors.Is(err, command.ErrRevisionConflict) {
			return changed, err
		}
		stale := current.Revision
		if err := waitFor(ctx, "npc_mindsets", func() (bool, error) {
			existing, err := s.listNPCMindsets(ctx)
			current = existing[key]
			return current.Revision > stale, err
		}); err != nil {
			return command.ChangedNPCMindset{}, err
		}
	}
}

// 正規化した名前（重複の判定と同じ）から NPC の思考パターンを引く表
func (s *seeder) listNPCMindsets(ctx context.Context) (map[string]query.NPCMindset, error) {
	npcMindsets := map[string]query.NPCMindset{}
	var after *string
	for {
		first := pagination.MaxFirst
		page, err := s.npcMindsetList.List(ctx, query.NPCMindsetFilter{}, &first, after)
		if err != nil {
			return nil, err
		}
		for _, edge := range page.Edges {
			npcMindsets[domain.NormalizeName(edge.Node.Name)] = edge.Node
		}
		if !page.PageInfo.HasNextPage {
			return npcMindsets, nil
		}
		after = page.PageInfo.EndCursor
	}
}
