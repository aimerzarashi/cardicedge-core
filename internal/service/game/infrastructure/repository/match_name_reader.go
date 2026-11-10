package repository

import (
	"context"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 対戦に出す名前（アバター・NPC のアバター・手札のカード）を、read model の avatars・npc_avatars・cards から 1 回ずつ読む。
// 対戦は read model を持たず集約の状態から返す（query.MatchHandler）ので、公開 ID で持つ名前だけをここで引く
type MatchNameReader struct{}

func NewMatchNameReader() *MatchNameReader {
	return &MatchNameReader{}
}

func (r *MatchNameReader) Load(ctx context.Context, avatarIDs, npcAvatarIDs, cardIDs []uuid.UUID) (query.MatchNames, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.MatchNames{}, err
	}
	var names query.MatchNames
	if names.Avatars, err = namesOf(avatarIDs, func(args []any) ([]rowName, error) {
		rows, err := models.Avatars(qm.WhereIn(models.AvatarColumns.AvatarID+" IN ?", args...)).All(ctx, exec)
		return mapRows(rows, err, func(row *models.Avatar) rowName { return rowName{row.AvatarID, row.Metadata} })
	}); err != nil {
		return query.MatchNames{}, err
	}
	if names.NPCAvatars, err = namesOf(npcAvatarIDs, func(args []any) ([]rowName, error) {
		rows, err := models.NPCAvatars(qm.WhereIn(models.NPCAvatarColumns.NPCAvatarID+" IN ?", args...)).All(ctx, exec)
		return mapRows(rows, err, func(row *models.NPCAvatar) rowName { return rowName{row.NPCAvatarID, row.Metadata} })
	}); err != nil {
		return query.MatchNames{}, err
	}
	cards, err := loadCards(ctx, exec, cardIDs)
	if err != nil {
		return query.MatchNames{}, err
	}
	names.Cards = make(map[uuid.UUID]query.MatchCard, len(cards))
	for id, row := range cards {
		names.Cards[id] = query.MatchCard{ID: id, Name: row.card.Name, Type: row.card.Type, Cost: row.card.Cost, Rarity: row.card.Rarity, Creature: row.card.Creature, Item: row.card.Item, Spell: row.card.Spell}
	}
	return names, nil
}
