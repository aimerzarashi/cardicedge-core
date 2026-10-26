-- ポリシー（イベントを受けてコマンドを動かす購読者）の失敗の記録。回数が上限に達した行はデッドレターとして、
-- やり直さずに残す。行を消す（外す）と、チェックポイントの続きから進む。投影は失敗の記録を持たない
-- （何度作り直しても害が無く、やり直しで困っていないため）

-- entity_id に外部キーを張らないのは、event_subscriber_checkpoints と同じ理由（実行の状態であり、集約を消しても CASCADE で消さない）
CREATE TABLE event_subscriber_failures (
  subscriber_id SMALLINT NOT NULL REFERENCES event_subscribers(id),
  entity_type_id SMALLINT NOT NULL REFERENCES domain_entity_types(id),
  entity_id BIGINT NOT NULL,
  failure_count INTEGER NOT NULL CHECK (failure_count >= 1),
  last_failed_at TIMESTAMP WITH TIME ZONE NOT NULL,
  PRIMARY KEY (subscriber_id, entity_type_id, entity_id)
);

COMMENT ON TABLE event_subscriber_failures IS '購読者 × 集約ごとの、続けて失敗した回数と最後に失敗した時刻。行は失敗しているあいだだけある（全部成功したら消す）。回数が上限に達した行はデッドレター';
COMMENT ON COLUMN event_subscriber_failures.entity_id IS '集約の domain_entities.id（外部キーは張らない）';
COMMENT ON COLUMN event_subscriber_failures.failure_count IS '続けて失敗した回数（1 以上）';
COMMENT ON COLUMN event_subscriber_failures.last_failed_at IS '最後に失敗した時刻（書き込む SQL が入れる。DB のトリガーは使わない）';

CREATE VIEW event_subscriber_failure_view AS
SELECT
    sub.kind AS subscriber_kind,
    sub.name AS subscriber_name,
    ent.name AS entity_type,
    f.entity_id,
    f.failure_count,
    f.last_failed_at
FROM event_subscriber_failures f
JOIN event_subscribers sub ON sub.id = f.subscriber_id
JOIN domain_entity_types ent ON ent.id = f.entity_type_id
ORDER BY sub.id, ent.id, f.entity_id;

COMMENT ON VIEW event_subscriber_failure_view IS 'event_subscriber_failures を購読者名・entity_type 名付きで参照する確認用の VIEW。アプリからは使わない';
