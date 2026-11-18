package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// npc_mindsets を読む（一覧と詳細）。並び順は索引（sort_name, npc_mindset_id）と同じにし、続きは行の比べ方で取る。
// sqlboiler を使わない理由は PresetDeckReader.List と同じ
type NPCMindsetReader struct{}

func NewNPCMindsetReader() *NPCMindsetReader {
	return &NPCMindsetReader{}
}

func (r *NPCMindsetReader) List(ctx context.Context, condition query.ListNPCMindsetsCondition) ([]query.NPCMindsetListRow, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var where []string
	var args []any
	arg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if condition.After != nil {
		where = append(where, fmt.Sprintf("(sort_name, npc_mindset_id) > (%s, %s)", arg(condition.After.SortName), arg(condition.After.NPCMindsetID)))
	}
	if len(condition.Statuses) > 0 {
		statuses := make([]string, len(condition.Statuses))
		for i, status := range condition.Statuses {
			statuses[i] = string(status)
		}
		where = append(where, fmt.Sprintf("status = ANY(%s)", arg(pq.Array(statuses))))
	}
	if condition.NameContains != "" {
		where = append(where, fmt.Sprintf(`sort_name LIKE '%%' || %s || '%%' ESCAPE '\'`, arg(likeEscaper.Replace(condition.NameContains))))
	}
	sql := "SELECT npc_mindset_id, sort_name, metadata FROM npc_mindsets"
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY sort_name, npc_mindset_id LIMIT " + arg(condition.Limit)

	rows, err := exec.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []query.NPCMindsetListRow
	for rows.Next() {
		var (
			id       uuid.UUID
			sortName string
			raw      []byte
		)
		if err := rows.Scan(&id, &sortName, &raw); err != nil {
			return nil, err
		}
		npcMindset, err := toNPCMindset(id, raw)
		if err != nil {
			return nil, err
		}
		cursor := query.NPCMindsetListCursor{SortName: sortName, NPCMindsetID: id}
		result = append(result, query.NPCMindsetListRow{NPCMindset: npcMindset, Cursor: cursor})
	}
	return result, rows.Err()
}

func (r *NPCMindsetReader) Get(ctx context.Context, id uuid.UUID) (query.NPCMindset, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.NPCMindset{}, err
	}
	row, err := models.FindNPCMindset(ctx, exec, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return query.NPCMindset{}, query.ErrNPCMindsetNotFound
	}
	if err != nil {
		return query.NPCMindset{}, err
	}
	return toNPCMindset(id, row.Metadata)
}

func toNPCMindset(id uuid.UUID, raw []byte) (query.NPCMindset, error) {
	var metadata readmodel.NPCMindsetMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return query.NPCMindset{}, fmt.Errorf("npc_mindsets.metadata of %s: %w", id, err)
	}
	// 並びは種類・基準の定義の順（map の順は不定なので固定する）。metadata に無い種類・基準は 0 にせず誤りにする
	actionWeights := make([]query.NPCMindsetActionWeight, 0, len(matchaction.Actions()))
	for _, action := range matchaction.Actions() {
		weight, ok := metadata.ActionWeights[action]
		if !ok {
			return query.NPCMindset{}, fmt.Errorf("npc_mindsets.metadata of %s: no weight of action %q", id, action)
		}
		actionWeights = append(actionWeights, query.NPCMindsetActionWeight{Action: string(action), Weight: weight})
	}
	criterionWeights := make([]query.NPCMindsetCriterionWeight, 0, len(npcmindset.Criteria()))
	for _, criterion := range npcmindset.Criteria() {
		weight, ok := metadata.CriterionWeights[criterion]
		if !ok {
			return query.NPCMindset{}, fmt.Errorf("npc_mindsets.metadata of %s: no weight of criterion %q", id, criterion)
		}
		criterionWeights = append(criterionWeights, query.NPCMindsetCriterionWeight{Criterion: string(criterion), Weight: weight})
	}
	return query.NPCMindset{
		ID: metadata.ID, Name: metadata.Name, Status: metadata.Status,
		Revision: metadata.Revision, RegisteredAt: metadata.RegisteredAt, RegisteredBy: metadata.RegisteredBy,
		UpdatedAt: metadata.UpdatedAt, UpdatedBy: metadata.UpdatedBy,
		ActionWeights: actionWeights, CriterionWeights: criterionWeights,
	}, nil
}
