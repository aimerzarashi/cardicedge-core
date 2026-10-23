package domaindef

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// Sync は、一覧の種別のうち DB に無いものを足し、足した後の DB の行が一覧とちょうど一致するかを確かめる。食い違えば
// （同じ id で名前が違う・同じ名前で id が違う・一覧に無い行がある）エラーを返す。行は消さない（過去のイベントが参照しているため）。
// アプリケーションの起動時に、合わせるトランザクションの中で呼ぶ（core#33）
func Sync(ctx context.Context, catalog Catalog) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	// 一意制約にかかる行（同じ名前で別の id）も足さずに通し、後の比べ合わせで食い違いとして返す
	// （ON CONFLICT の対象を書かないと、主キー・名前のどちらの重なりでも足さない）
	for _, entityType := range catalog.EntityTypes {
		if _, err := exec.ExecContext(ctx, `INSERT INTO domain_entity_types (id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			entityType.ID, entityType.Name); err != nil {
			return fmt.Errorf("failed to add domain_entity_types: %w", err)
		}
		for _, event := range entityType.Events {
			if _, err := exec.ExecContext(ctx, `INSERT INTO domain_event_types (id, entity_type_id, name) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
				event.ID, entityType.ID, event.Name); err != nil {
				return fmt.Errorf("failed to add domain_event_types: %w", err)
			}
		}
	}
	for _, keyType := range catalog.UniqueKeyTypes {
		if _, err := exec.ExecContext(ctx, `INSERT INTO domain_unique_key_types (id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			keyType.ID, keyType.Name); err != nil {
			return fmt.Errorf("failed to add domain_unique_key_types: %w", err)
		}
	}
	for _, referenceType := range catalog.ReferenceTypes {
		if _, err := exec.ExecContext(ctx, `INSERT INTO domain_reference_types (id, name) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			referenceType.ID, referenceType.Name); err != nil {
			return fmt.Errorf("failed to add domain_reference_types: %w", err)
		}
	}
	return verifyCatalog(ctx, exec, catalog)
}

// 行を「id: 名前」の文字列にして、両方向に比べる（どの列が食い違っても、行ごと出す）
func verifyCatalog(ctx context.Context, exec rdb.Executor, catalog Catalog) error {
	var entityTypes, eventTypes, uniqueKeyTypes, referenceTypes []string
	for _, entityType := range catalog.EntityTypes {
		entityTypes = append(entityTypes, fmt.Sprintf("%d: %s", entityType.ID, entityType.Name))
		for _, event := range entityType.Events {
			eventTypes = append(eventTypes, fmt.Sprintf("%d: %d.%s", event.ID, entityType.ID, event.Name))
		}
	}
	for _, keyType := range catalog.UniqueKeyTypes {
		uniqueKeyTypes = append(uniqueKeyTypes, fmt.Sprintf("%d: %s", keyType.ID, keyType.Name))
	}
	for _, referenceType := range catalog.ReferenceTypes {
		referenceTypes = append(referenceTypes, fmt.Sprintf("%d: %s", referenceType.ID, referenceType.Name))
	}
	tables := []struct {
		name     string
		query    string
		declared []string
	}{
		{name: "domain_entity_types", query: `SELECT id || ': ' || name FROM domain_entity_types`, declared: entityTypes},
		{name: "domain_event_types", query: `SELECT id || ': ' || entity_type_id || '.' || name FROM domain_event_types`, declared: eventTypes},
		{name: "domain_unique_key_types", query: `SELECT id || ': ' || name FROM domain_unique_key_types`, declared: uniqueKeyTypes},
		{name: "domain_reference_types", query: `SELECT id || ': ' || name FROM domain_reference_types`, declared: referenceTypes},
	}
	var errs []error
	for _, table := range tables {
		inDB, err := queryStrings(ctx, exec, table.query)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", table.name, err)
		}
		for _, row := range sortedDiff(table.declared, inDB) {
			errs = append(errs, fmt.Errorf("catalog has a row not in %s: %s", table.name, row))
		}
		for _, row := range sortedDiff(inDB, table.declared) {
			errs = append(errs, fmt.Errorf("%s has a row not in catalog: %s", table.name, row))
		}
	}
	return errors.Join(errs...)
}

func queryStrings(ctx context.Context, exec rdb.Executor, query string) ([]string, error) {
	rows, err := exec.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// a にあって b に無いものを、並べて返す（誤りの並びを毎回同じにするため）
func sortedDiff(a, b []string) []string {
	var diff []string
	for _, value := range a {
		if !slices.Contains(b, value) {
			diff = append(diff, value)
		}
	}
	slices.Sort(diff)
	return diff
}
