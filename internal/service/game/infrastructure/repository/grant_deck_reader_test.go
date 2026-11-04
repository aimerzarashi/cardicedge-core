package repository_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 公開は studio の read model の行をそのまま写すため、書き手の型で JSON を作り、読み手が同じ形を読めることを確かめる
func insertGrantDeck(t *testing.T, ctx context.Context, id uuid.UUID, sortName string, status readmodel.GrantDeckStatus, name string, cards ...readmodel.DeckCard) {
	t.Helper()
	metadata, err := json.Marshal(readmodel.GrantDeckMetadata{ID: id, Name: name, Status: status, Cards: cards})
	require.NoError(t, err)
	row := models.GrantDeck{GrantDeckID: id.String(), SortName: sortName, Status: string(status), Metadata: metadata}
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)
	require.NoError(t, row.Insert(ctx, exec, boil.Infer()))
}

func TestGrantDeckReader_ListsOnlyPublishedOfGivenIDs(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	fireCard, fireRef := issueCardEntity(t, ctx)
	waterCard, waterRef := issueCardEntity(t, ctx)
	water, fire, draft, retired, other := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	insertGrantDeck(t, ctx, water, "b-water", readmodel.GrantDeckStatusPublished, "水", readmodel.DeckCard{CardID: waterCard, Count: 3}, readmodel.DeckCard{CardID: fireCard, Count: 1})
	insertGrantDeck(t, ctx, fire, "a-fire", readmodel.GrantDeckStatusPublished, "火", readmodel.DeckCard{CardID: fireCard, Count: 4})
	insertGrantDeck(t, ctx, draft, "c-draft", readmodel.GrantDeckStatusDraft, "準備中")
	insertGrantDeck(t, ctx, retired, "d-retired", readmodel.GrantDeckStatusRetired, "使用不可")
	insertGrantDeck(t, ctx, other, "e-other", readmodel.GrantDeckStatusPublished, "指さない")

	got, err := newGrantDeckReader(t, ctx).ListPublishedByIDs(ctx, []uuid.UUID{water, fire, draft, retired, uuid.New()})

	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]master.GrantDeck{
		fire:  {Name: "火", Cards: []master.DeckCard{{Card: fireRef, Count: 4}}},
		water: {Name: "水", Cards: []master.DeckCard{{Card: waterRef, Count: 3}, {Card: fireRef, Count: 1}}},
	}, got)
}

func TestGrantDeckReader_ReturnsEmptyWhenNoneOfGivenIDsIsPublished(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	draft := uuid.New()
	insertGrantDeck(t, ctx, draft, "a-draft", readmodel.GrantDeckStatusDraft, "準備中")

	got, err := newGrantDeckReader(t, ctx).ListPublishedByIDs(ctx, []uuid.UUID{draft})

	require.NoError(t, err)
	require.Empty(t, got)
}

// 写していないカード（game の domain_entities に行が無い）を入れた配布デッキは、黙って捨てず失敗にする（publish-master を流せば直る）
func TestGrantDeckReader_FailsForCardThatWasNotPublishedToGame(t *testing.T) {
	db, _ := rdbtest.NewDatabase(t)
	ctx := rdb.WithDB(t.Context(), db)
	fire := uuid.New()
	insertGrantDeck(t, ctx, fire, "a-fire", readmodel.GrantDeckStatusPublished, "火", readmodel.DeckCard{CardID: uuid.New(), Count: 4})

	_, err := newGrantDeckReader(t, ctx).ListPublishedByIDs(ctx, []uuid.UUID{fire})

	require.ErrorIs(t, err, domain.ErrEntityNotFound)
}

func newGrantDeckReader(t *testing.T, ctx context.Context) *repository.GrantDeckReader {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return repository.NewGrantDeckReader(identifier.NewEntityRefResolver(registry))
}

// 写したカードは、publish-master が game の domain_entities に行を作る（公開 ID は studio と同じ）。ここでは Creature の識別子の発行で用意する
func issueCardEntity(t *testing.T, ctx context.Context) (uuid.UUID, card.Ref) {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, publicID, err := identifier.New(registry, creature.NewID).Issue(ctx)
	require.NoError(t, err)
	return publicID, cardtest.Ref(t, id.EntityType(), id.Int64())
}
