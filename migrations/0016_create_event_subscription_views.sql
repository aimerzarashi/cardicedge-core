-- 誰がどの集約の種別を購読しているか（event_subscriptions）と、やることの表（event_subscriber_pending）を、
-- 名前付きで見る確認用の VIEW（0007・0011 と同じ）。アプリからは使わない

CREATE VIEW event_subscription_view AS
SELECT
    sub.kind AS subscriber_kind,
    sub.name AS subscriber_name,
    ent.name AS entity_type
FROM event_subscriptions s
JOIN event_subscribers sub ON sub.id = s.subscriber_id
JOIN domain_entity_types ent ON ent.id = s.entity_type_id
ORDER BY sub.id, ent.id;

COMMENT ON VIEW event_subscription_view IS 'event_subscriptions を購読者名・entity_type 名付きで参照する確認用の VIEW。アプリからは使わない';

-- 処理中かは、その単位のアドバイザリーロックを持つ接続があるかで見る（core#19。ロックは接続に付き、表には残らない）。
-- ロックの番号の作り方（購読者の id を上位、集約の内部 ID を下位 48 ビット）は internal/eventdriven/lock.go の unitLockID と同じにする。
-- pg_locks は 64 ビットの番号を上位（classid）・下位（objid）の 32 ビットに分け、objsubid を 1 にして見せる
CREATE VIEW event_subscriber_pending_view AS
SELECT
    sub.kind AS subscriber_kind,
    sub.name AS subscriber_name,
    ent.name AS entity_type,
    p.entity_id,
    p.version,
    l.pid AS processing_pid,
    a.client_addr AS processing_client_addr,
    a.application_name AS processing_application
FROM event_subscriber_pending p
JOIN event_subscribers sub ON sub.id = p.subscriber_id
JOIN domain_entity_types ent ON ent.id = p.entity_type_id
LEFT JOIN pg_locks l
    ON l.locktype = 'advisory'
    AND l.granted
    AND l.objsubid = 1
    AND l.database = (SELECT oid FROM pg_database WHERE datname = current_database())
    AND l.classid = (((p.subscriber_id::bigint << 48) | p.entity_id) >> 32)::oid
    AND l.objid = (((p.subscriber_id::bigint << 48) | p.entity_id) & 4294967295)::oid
LEFT JOIN pg_stat_activity a ON a.pid = l.pid
ORDER BY sub.id, ent.id, p.entity_id;

COMMENT ON VIEW event_subscriber_pending_view IS 'event_subscriber_pending を購読者名・entity_type 名付きで参照し、処理中（単位のアドバイザリーロックを持つ接続がある）ならその接続の pid・接続元のアドレス（どの台か）・application_name を出す確認用の VIEW。アプリからは使わない';
