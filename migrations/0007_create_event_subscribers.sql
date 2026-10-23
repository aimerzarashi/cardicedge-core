-- イベント駆動（internal/eventdriven）の土台。domain_events に入ったイベントを通知し、購読者（Projection・Policy）が処理した位置と、
-- まだ作り直していない集約（やることの表）を持つ

-- 知らせの中身: entity_type_id:event_type_id:entity_id:aggregate_version:event_id（参照の api と同じ）。
-- 振り分けに使う値を先に並べ、受け取った側が DB を引かずに、どの集約のどの版かを知れるようにする。
-- 通知は commit のときに届くため、rollback したイベントは通知されない。イベントストアは版の衝突を ON CONFLICT DO NOTHING で
-- 受けるため、衝突して行が入らなかったときはトリガーも動かない。チャンネルはデータベースの単位なので、core_studio・core_game を
-- 別の DB にしていれば、互いの知らせは混ざらない（core#33）
CREATE FUNCTION notify_domain_event() RETURNS TRIGGER AS $$
BEGIN
  PERFORM pg_notify(
    'domain_events_channel',
    NEW.entity_type_id || ':' || NEW.event_type_id || ':' || NEW.entity_id || ':' || NEW.aggregate_version || ':' || NEW.id
  );
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION notify_domain_event() IS 'domain_events への INSERT を domain_events_channel に通知する（中身: entity_type_id:event_type_id:entity_id:aggregate_version:event_id）';

CREATE TRIGGER domain_events_notify
  AFTER INSERT ON domain_events
  FOR EACH ROW
  EXECUTE FUNCTION notify_domain_event();

-- 購読者・購読の行はマイグレーションでは入れず、起動時に、そのサービスの組み立て（RegisterProjections 等）とコードの一覧
-- （internal/wiring）から入れる（core#33）。id は自動で振らない（種別と同じ理由）
CREATE TABLE event_subscribers (
  id SMALLINT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('projection', 'policy')),
  name TEXT NOT NULL,
  UNIQUE (kind, name)
);

COMMENT ON TABLE event_subscribers IS 'イベントの購読者（Projection・Policy）の名前と id の対応。行は起動時にコードの一覧から入れる';
COMMENT ON COLUMN event_subscribers.kind IS '購読者の種類（projection・policy）';
COMMENT ON COLUMN event_subscribers.name IS 'コードが宣言する購読者の名前';

-- entity_id に domain_entities への外部キーを張らないのは、処理の位置は実行の状態であり、集約を消しても
-- 購読者が読み切る前に CASCADE で消えないようにするため（domain_events と同じ）
CREATE TABLE event_subscriber_checkpoints (
  subscriber_id SMALLINT NOT NULL REFERENCES event_subscribers(id),
  entity_type_id SMALLINT NOT NULL REFERENCES domain_entity_types(id),
  entity_id BIGINT NOT NULL,
  last_version BIGINT NOT NULL CHECK (last_version >= 1),
  updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
  PRIMARY KEY (subscriber_id, entity_type_id, entity_id)
);

COMMENT ON TABLE event_subscriber_checkpoints IS '購読者 × 集約ごとの、処理済みの集約の版。行は 1 件以上処理したときだけある';
COMMENT ON COLUMN event_subscriber_checkpoints.entity_id IS '集約の domain_entities.id（外部キーは張らない）';
COMMENT ON COLUMN event_subscriber_checkpoints.last_version IS '処理済みの集約の版（1 以上）';
COMMENT ON COLUMN event_subscriber_checkpoints.updated_at IS '最後に処理した時刻（書き込む SQL が入れる。DB のトリガーは使わない）';

CREATE VIEW event_subscriber_checkpoint_view AS
SELECT
    sub.kind AS subscriber_kind,
    sub.name AS subscriber_name,
    ent.name AS entity_type,
    cp.entity_id,
    cp.last_version,
    cp.updated_at
FROM event_subscriber_checkpoints cp
JOIN event_subscribers sub ON sub.id = cp.subscriber_id
JOIN domain_entity_types ent ON ent.id = cp.entity_type_id
ORDER BY sub.id, ent.id, cp.entity_id;

COMMENT ON VIEW event_subscriber_checkpoint_view IS 'event_subscriber_checkpoints を購読者名・entity_type 名付きで参照する確認用の VIEW（参照の api の 0032 と同じ）。アプリからは使わない';

-- 購読者がまだ作り直していない集約（やることの表）を、イベントと同じトランザクションでトリガーが記録する。
-- Poller はこの表だけを読む。参照の api の Poller は間隔ごとに domain_events をすべてチェックポイントと突き合わせ、
-- 遅れが無くてもイベントの総数に比例して重くなるため採らない（イベントは機能とともに増え続ける）

-- トリガーは Go のコードを見られないため、誰がどの集約の種別を購読するかを DB にも持つ。行は起動時に、コードの宣言
-- （ProjectionSpec.Subscriptions）に合わせて足し・消す（core#33）。前は「デプロイの途中で新旧の core が同時に動くと書き換え合う」ため
-- 起動時に書き込まずマイグレーションで入れていたが、2 つの DB に同じマイグレーションを流すことにしたため改めた。
-- 新旧が同時に動くデプロイをするときに考え直す（core#19）
CREATE TABLE event_subscriptions (
  subscriber_id SMALLINT NOT NULL REFERENCES event_subscribers(id),
  entity_type_id SMALLINT NOT NULL REFERENCES domain_entity_types(id),
  PRIMARY KEY (subscriber_id, entity_type_id)
);

-- トリガーが種別から購読者を引く
CREATE INDEX idx_event_subscriptions_entity_type_id ON event_subscriptions (entity_type_id);

COMMENT ON TABLE event_subscriptions IS '購読者が購読する集約の種別（その種別のイベントをすべて受け取る）';

-- entity_id に外部キーを張らないのは、event_subscriber_checkpoints と同じ理由（実行の状態であり、集約を消しても CASCADE で消さない）
CREATE TABLE event_subscriber_pending (
  subscriber_id SMALLINT NOT NULL REFERENCES event_subscribers(id),
  entity_type_id SMALLINT NOT NULL REFERENCES domain_entity_types(id),
  entity_id BIGINT NOT NULL,
  version BIGINT NOT NULL,
  PRIMARY KEY (subscriber_id, entity_type_id, entity_id)
);

COMMENT ON TABLE event_subscriber_pending IS '購読者がまだ作り直していない集約。イベントの INSERT でトリガーが入れ、購読者が処理した版以下なら同じトランザクションで消す';
COMMENT ON COLUMN event_subscriber_pending.version IS '入ったイベントの最新の版';

-- 処理中に新しいイベントが入ると版が上がるため、購読者が消す条件（処理した版以下）に当たらず残る
CREATE FUNCTION enqueue_event_subscriber_pending() RETURNS TRIGGER AS $$
BEGIN
  INSERT INTO event_subscriber_pending (subscriber_id, entity_type_id, entity_id, version)
  SELECT s.subscriber_id, NEW.entity_type_id, NEW.entity_id, NEW.aggregate_version
  FROM event_subscriptions s
  WHERE s.entity_type_id = NEW.entity_type_id
  ON CONFLICT (subscriber_id, entity_type_id, entity_id) DO UPDATE
  SET version = GREATEST(event_subscriber_pending.version, EXCLUDED.version);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION enqueue_event_subscriber_pending() IS 'domain_events への INSERT で、その種別を購読する購読者ごとに event_subscriber_pending に行を入れる';

CREATE TRIGGER domain_events_enqueue_pending
  AFTER INSERT ON domain_events
  FOR EACH ROW
  EXECUTE FUNCTION enqueue_event_subscriber_pending();
