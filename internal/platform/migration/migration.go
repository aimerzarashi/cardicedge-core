// Package migration は DB マイグレーションを差分だけ適用する。
//
// 仕様:
//   - ファイル名は NNNN_name.sql。NNNN の昇順に適用する
//   - 適用済みのものは schema_migrations に1適用1行で記録する（適用にかかった時間 execution_ms も記録する）
//   - 1ファイルを1トランザクションで適用し、失敗したらそのファイルの変更と記録を戻す
//   - 同時実行は pg_advisory_lock で防ぐ
//   - 適用済みの最大より小さい未適用ファイル（番号の抜け）があれば、何も適用せずにエラーにする
//   - down（元に戻す）は無い。誤りは新しいマイグレーションで直す
package migration

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// lockKey は pg_advisory_lock のキー。同じ DB で migrate が同時に走らないようにする。
const lockKey int64 = 7_390_001

var fileNamePattern = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

// Migration は1つのマイグレーションファイル。
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// Applied は適用した結果。
type Applied struct {
	Version     int
	Name        string
	ExecutionMs int64
}

// Load は fsys 直下の NNNN_name.sql を読み込み、Version の昇順で返す。
// .sql 以外のファイルは無視する。名前の形式が違う .sql と、Version の重複はエラーにする。
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations: %w", err)
	}

	var migrations []Migration
	seen := map[int]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m := fileNamePattern.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("invalid migration file name %q: must be NNNN_name.sql (name: [a-z0-9_]+)", e.Name())
		}
		version, _ := strconv.Atoi(m[1])
		if prev, ok := seen[version]; ok {
			return nil, fmt.Errorf("duplicate migration version %04d: %q and %q", version, prev, e.Name())
		}
		seen[version] = e.Name()

		body, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read %q: %w", e.Name(), err)
		}
		migrations = append(migrations, Migration{Version: version, Name: m[2], SQL: string(body)})
	}

	slices.SortFunc(migrations, func(a, b Migration) int { return a.Version - b.Version })
	return migrations, nil
}

// Apply は migrations のうち未適用のものを Version の昇順に適用し、適用したものを返す。
func Apply(ctx context.Context, db *sql.DB, migrations []Migration) ([]Applied, error) {
	// advisory lock はセッション単位のため、ロックから適用まで同じ接続を使う
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return nil, fmt.Errorf("failed to acquire migration lock: %w", err)
	}
	defer conn.ExecContext(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, lockKey)

	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version      INTEGER PRIMARY KEY,
		name         TEXT NOT NULL,
		applied_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
		execution_ms INTEGER NOT NULL
	)`); err != nil {
		return nil, fmt.Errorf("failed to create schema_migrations: %w", err)
	}

	applied, maxApplied, err := appliedVersions(ctx, conn)
	if err != nil {
		return nil, err
	}

	var pending []Migration
	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}
		if m.Version < maxApplied {
			return nil, fmt.Errorf("migration %04d_%s is not applied but older than the latest applied version %04d: renumber it after %04d",
				m.Version, m.Name, maxApplied, maxApplied)
		}
		pending = append(pending, m)
	}

	var results []Applied
	for _, m := range pending {
		ms, err := applyOne(ctx, conn, m)
		if err != nil {
			return results, err
		}
		results = append(results, Applied{Version: m.Version, Name: m.Name, ExecutionMs: ms})
	}
	return results, nil
}

func appliedVersions(ctx context.Context, conn *sql.Conn) (map[int]bool, int, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read schema_migrations: %w", err)
	}
	defer rows.Close()

	applied := map[int]bool{}
	maxVersion := 0
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, 0, fmt.Errorf("failed to scan schema_migrations: %w", err)
		}
		applied[v] = true
		maxVersion = max(maxVersion, v)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to read schema_migrations: %w", err)
	}
	return applied, maxVersion, nil
}

// applyOne は1ファイルを1トランザクションで適用して記録し、適用にかかった時間（ミリ秒）を返す。
func applyOne(ctx context.Context, conn *sql.Conn, m Migration) (int64, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction for %04d_%s: %w", m.Version, m.Name, err)
	}
	defer tx.Rollback()

	start := time.Now()
	// 引数なしの Exec はファイル内の複数の文をまとめて実行できる（simple query protocol）
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return 0, fmt.Errorf("failed to apply %04d_%s: %w", m.Version, m.Name, err)
	}
	ms := time.Since(start).Milliseconds()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, execution_ms) VALUES ($1, $2, $3)`,
		m.Version, m.Name, ms); err != nil {
		return 0, fmt.Errorf("failed to record %04d_%s: %w", m.Version, m.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit %04d_%s: %w", m.Version, m.Name, err)
	}
	return ms, nil
}
