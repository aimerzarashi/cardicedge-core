package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
)

// core_game の starter_sets（make publish-master が studio の read model から全行を状態つきで写した表）のうち、公開中のものを読む。
// 準備中・使用不可も写してあるのは、配布デッキと同じく、後で配り済みのセットを表示したくなっても表を作り直さずに済むようにするため
type StarterSetReader struct {
	grantDecks *GrantDeckReader
	// 解放するボードの公開 ID を、game の内部 ID（make publish-master が domain_entities に作る行）に引き直す
	boards domain.Identifier[board.ID]
}

func NewStarterSetReader(grantDecks *GrantDeckReader, boards domain.Identifier[board.ID]) *StarterSetReader {
	return &StarterSetReader{grantDecks: grantDecks, boards: boards}
}

// 写した表の metadata のうち、配布に要る項目。studio の read model の型を import せず、この JSON の形だけを契約にする
// （GrantDeckReader と同じ）。どちらも公開 ID の配列（割り当てた順）。配るデッキは写した grant_decks から引き、
// 解放するボードは game の内部 ID に引き直して返す（game の集約は game の内部 ID で指すため）
type starterSetMetadata struct {
	GrantDecks   []uuid.UUID `json:"grant_decks"`
	UnlockBoards []uuid.UUID `json:"unlock_boards"`
}

// 公開中のセットが無ければ found が false。公開中は同時に 1 つだけの決まりだが、写した表では守れないため（写す側の間違いでも
// 配布が止まらないよう）、2 つ以上あれば名前の正規化 → 公開 ID の順で先頭を使う。
// 配るデッキは、セットが持つ順に並べ、公開中でないもの（使用不可にした後・写していないもの）は飛ばす
func (r *StarterSetReader) GetPublished(ctx context.Context) (master.StarterSet, bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return master.StarterSet{}, false, err
	}
	row, err := models.StarterSets(
		models.StarterSetWhere.Status.EQ("published"),
		qm.OrderBy(models.StarterSetColumns.SortName+", "+models.StarterSetColumns.StarterSetID),
		qm.Limit(1),
	).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return master.StarterSet{}, false, nil
	}
	if err != nil {
		return master.StarterSet{}, false, err
	}
	var metadata starterSetMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return master.StarterSet{}, false, fmt.Errorf("starter_sets.metadata of %s: %w", row.StarterSetID, err)
	}
	published, err := r.grantDecks.ListPublishedByIDs(ctx, metadata.GrantDecks)
	if err != nil {
		return master.StarterSet{}, false, err
	}
	grantDecks := make([]master.GrantDeck, 0, len(metadata.GrantDecks))
	for _, id := range metadata.GrantDecks {
		if grantDeck, ok := published[id]; ok {
			grantDecks = append(grantDecks, grantDeck)
		}
	}
	unlockBoards := make([]board.ID, 0, len(metadata.UnlockBoards))
	for _, publicID := range metadata.UnlockBoards {
		// 写していないボード（publish-master が domain_entities に行を作っていない）は ErrEntityNotFound。待てば直る失敗として返す
		boardID, err := r.boards.Resolve(ctx, publicID)
		if err != nil {
			return master.StarterSet{}, false, fmt.Errorf("starter_sets.metadata of %s: unlock board %s: %w", row.StarterSetID, publicID, err)
		}
		unlockBoards = append(unlockBoards, boardID)
	}
	return master.StarterSet{GrantDecks: grantDecks, UnlockBoards: unlockBoards}, true, nil
}
