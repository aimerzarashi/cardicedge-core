INSERT INTO domain_entity_types (id, name) VALUES (4000, 'spell');
INSERT INTO domain_event_types (id, entity_type_id, name) VALUES
  (4000, 4000, 'RegisteredEvent');

-- カードの一覧（0010）は Spell も購読する
INSERT INTO event_subscriptions (subscriber_id, entity_type_id) VALUES
  (1000, 4000);

-- 購読する種別を足すときの手順（internal/eventdriven/README.md）どおり、すでにある Spell の分のやることの行も入れる。
-- 登録の経路は同じリリースで入るが、手順をそろえておけば、足し方の判断を毎回しなくて済む
INSERT INTO event_subscriber_pending (subscriber_id, entity_type_id, entity_id, version)
SELECT 1000, entity_type_id, entity_id, max(aggregate_version)
FROM domain_events
WHERE entity_type_id = 4000
GROUP BY entity_type_id, entity_id;
