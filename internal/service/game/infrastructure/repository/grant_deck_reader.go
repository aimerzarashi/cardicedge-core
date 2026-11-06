package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
)

// core_game の grant_decks（make publish-master が studio の read model から全行を状態つきで写した表）のうち、公開中だけを読む。
// どれを配るかはスターターセット（StarterSetReader）が決める。
// 準備中・使用不可も写してあるのは、配り済みのデッキの元を後で表示したくなったとき、表を作り直さずに済むようにするため
type GrantDeckReader struct {
	// 写したカードの公開 ID を、game の内部 ID（make publish-master が domain_entities に作る行）に引き直す
	entityRefs domain.EntityRefResolver
}

func NewGrantDeckReader(entityRefs domain.EntityRefResolver) *GrantDeckReader {
	return &GrantDeckReader{entityRefs: entityRefs}
}

// 写した表の metadata のうち、配布に要る項目。studio の read model の型を import せず、この JSON の形だけを契約にする
// （公開した版と game の版が一致しなくても、読む項目が変わらない限り読める）
type grantDeckMetadata struct {
	Name  string `json:"name"`
	Cards []struct {
		CardID uuid.UUID `json:"card_id"`
		Count  int       `json:"count"`
	} `json:"cards"`
}

// 渡した公開 ID のうち公開中のものを、公開 ID から引く表で返す。公開中でないもの（準備中・使用不可・写していない）は含めない。
// 並べるのは呼ぶ側（スターターセットが持つ順）なので、ここでは並べない
func (r *GrantDeckReader) ListPublishedByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]master.GrantDeck, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	idStrings := make([]string, len(ids))
	for i, id := range ids {
		idStrings[i] = id.String()
	}
	rows, err := models.GrantDecks(
		models.GrantDeckWhere.Status.EQ("published"),
		models.GrantDeckWhere.GrantDeckID.IN(idStrings),
	).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	metadatas := make(map[uuid.UUID]grantDeckMetadata, len(rows))
	var cardIDs []uuid.UUID
	for _, row := range rows {
		id, err := uuid.Parse(row.GrantDeckID)
		if err != nil {
			return nil, fmt.Errorf("grant_decks.grant_deck_id %q: %w", row.GrantDeckID, err)
		}
		var metadata grantDeckMetadata
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("grant_decks.metadata of %s: %w", row.GrantDeckID, err)
		}
		metadatas[id] = metadata
		for _, c := range metadata.Cards {
			cardIDs = append(cardIDs, c.CardID)
		}
	}
	// 写していないカード（publish-master が domain_entities に行を作っていない）は、待てば直る失敗として返す
	entityRefs, err := r.entityRefs.ResolveAll(ctx, cardIDs)
	if err != nil {
		return nil, err
	}
	decks := make(map[uuid.UUID]master.GrantDeck, len(rows))
	for id, metadata := range metadatas {
		cards := make([]master.DeckCard, 0, len(metadata.Cards))
		for _, c := range metadata.Cards {
			entityRef, ok := entityRefs[c.CardID]
			if !ok {
				return nil, fmt.Errorf("grant_decks.metadata of %s: card %s: %w", id, c.CardID, domain.ErrEntityNotFound)
			}
			ref, err := card.NewRef(entityRef.EntityType(), entityRef.ID())
			if err != nil {
				return nil, fmt.Errorf("grant_decks.metadata of %s: card %s: %w", id, c.CardID, err)
			}
			cards = append(cards, master.DeckCard{Card: ref, Count: c.Count})
		}
		decks[id] = master.GrantDeck{Name: metadata.Name, Cards: cards}
	}
	return decks, nil
}
