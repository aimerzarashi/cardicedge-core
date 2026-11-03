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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
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
	cardStatuses   *command.CardHandler
	presetDecks    *command.PresetDeckHandler
	grantDecks     *command.GrantDeckHandler
	boards         *command.BoardHandler
	npcAvatars     *command.NPCAvatarHandler
	npcDecks       *command.NPCDeckHandler
	cards          *query.CardHandler
	presetDeckList *query.PresetDeckHandler
	grantDeckList  *query.GrantDeckHandler
	boardList      *query.BoardHandler
	npcAvatarList  *query.NPCAvatarHandler
	npcDeckList    *query.NPCDeckHandler
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
		grantDecks: command.NewGrantDeckHandler(
			eventstore.New(registry, grantdeck.NewID, grantdeck.NewEventType, grantdeck.InitialState, grantdeck.Evolve),
			identifier.New(registry, grantdeck.NewID),
			entityRefs,
			constraint.UpdateGrantDeckName,
			constraint.UpdateGrantDeckCardReferences,
			command.NewCardStatusReader(creatureStore, itemStore, spellStore).Statuses,
			presetDeckStore,
			presetDeckIdentifier,
			constraint.FindGameRules,
			constraint.ReserveGrantDeckQuota,
			constraint.ReleaseGrantDeckQuota,
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
		npcAvatars: command.NewNPCAvatarHandler(
			eventstore.New(registry, npcavatar.NewID, npcavatar.NewEventType, npcavatar.InitialState, npcavatar.Evolve),
			identifier.New(registry, npcavatar.NewID),
			entityRefs,
			constraint.UpdateNPCAvatarName,
			constraint.UpdateNPCAvatarDeckReferences,
			command.NewNPCDeckStatusReader(eventstore.New(registry, npcdeck.NewID, npcdeck.NewEventType, npcdeck.InitialState, npcdeck.Evolve)).Statuses,
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
		grantDeckList:  query.NewGrantDeckHandler(grantDeckReader.List, grantDeckReader.Get, presetDeckReader.Revisions),
		boardList:      query.NewBoardHandler(boardReader.List, boardReader.Get),
		npcAvatarList:  query.NewNPCAvatarHandler(npcAvatarReader.List, npcAvatarReader.Get),
		npcDeckList:    query.NewNPCDeckHandler(npcDeckReader.List, npcDeckReader.Get, presetDeckReader.Revisions),
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
	// NPC のアバターは、公開中の NPC のデッキを割り当てて公開するため、NPC のデッキより後に作る
	npcDeckIDs, err := s.seedNPCDecks(ctx, m.NPCDecks, deckIDs)
	if err != nil {
		return err
	}
	return s.seedNPCAvatars(ctx, m.NPCAvatars, npcDeckIDs)
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

// 登録して使うデッキを割り当て、公開する。何度流しても同じ結果になるよう、同じ名前の NPC のアバターがあれば登録しない。
// 割り当てと公開は、準備中のまま止まったものだけ続ける（公開中・使用不可のものは触らない。使用不可にしたものを、流し直しで公開に戻さない。
// すでにデッキを持たずに公開済みのものにも、後から割り当てない）。割り当ては同じデッキの集まりなら何も起きないため、準備中のものには毎回呼ぶ
func (s *seeder) seedNPCAvatars(ctx context.Context, avatars []masterNPCAvatar, npcDeckIDs map[string]uuid.UUID) error {
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
