package constraint

import (
	"context"

	"github.com/lib/pq"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 参照の種別は名前から引く（種別が無いと何も足せないが、起動時に domaindef で照合している）
const deleteReferencesQuery = `
	DELETE FROM domain_references r
	USING domain_reference_types t
	WHERE t.id = r.reference_type_id AND t.name = $1 AND r.referencing_id = $2 AND NOT (r.referenced_id = ANY($3))
`

// 参照される側の識別子が無ければ、外部キーの違反（23503）のエラーになる（参照する前に、コマンドが存在を確かめるため、コードの誤りのときだけ）
const insertReferencesQuery = `
	INSERT INTO domain_references (reference_type_id, referencing_id, referenced_id)
	SELECT t.id, $2, referenced.id
	FROM domain_reference_types t, unnest($3::bigint[]) AS referenced(id)
	WHERE t.name = $1
	ON CONFLICT DO NOTHING
`

// 参照する側（referencingID）の、その種別の参照を referencedIDs にそろえる（無いものは足し、外れたものは消す）
func updateReferences(ctx context.Context, referenceType domain.ReferenceType, referencingID int64, referencedIDs []int64) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	ids := pq.Array(referencedIDs)
	if _, err := exec.ExecContext(ctx, deleteReferencesQuery, referenceType.String(), referencingID, ids); err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, insertReferencesQuery, referenceType.String(), referencingID, ids)
	return err
}
