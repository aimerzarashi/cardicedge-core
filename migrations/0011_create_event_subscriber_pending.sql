-- 購読者がまだ作り直していない集約（やることの表）を、イベントと同じトランザクションでトリガーが記録する。
-- Poller はこの表だけを読む。参照の api の Poller は間隔ごとに domain_events をすべてチェックポイントと突き合わせ、
-- 遅れが無くてもイベントの総数に比例して重くなるため採らない（イベントは機能とともに増え続ける）

-- トリガーは Go のコードを見られないため、誰がどの集約の種別を購読するかを DB にも持つ。
-- 購読者の行（event_subscribers）と同じマイグレーションで足し、起動時にコードの宣言（ProjectionSpec.Subscriptions）と照合する。
-- 起動時にコードから書き込まないのは、デプロイの途中で新旧の core が同時に動くと書き換え合うため
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

-- カードの一覧（0010）は Creature を購読する
INSERT INTO event_subscriptions (subscriber_id, entity_type_id) VALUES
  (1000, 2000);

-- 投影を足す前に登録された集約も作り直すため、やることの行を入れておく（投影を足すときの手順）
INSERT INTO event_subscriber_pending (subscriber_id, entity_type_id, entity_id, version)
SELECT 1000, entity_type_id, entity_id, max(aggregate_version)
FROM domain_events
WHERE entity_type_id = 2000
GROUP BY entity_type_id, entity_id;
