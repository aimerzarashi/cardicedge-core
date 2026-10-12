-- 集約を足すたびに、その集約の種別とイベント種別を新しいマイグレーションで足す
INSERT INTO domain_entity_types (id, name) VALUES (1000, 'operator');
INSERT INTO domain_event_types (id, entity_type_id, name) VALUES
  (1000, 1000, 'RegisteredEvent'),
  (1001, 1000, 'LinkedEvent');
