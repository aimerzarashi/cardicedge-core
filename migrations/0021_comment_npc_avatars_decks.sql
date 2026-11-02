-- 使う NPC のデッキを持つようにした（core#87）ため、metadata の中身の説明を直す（適用済みの 0019 は書き換えない）
COMMENT ON COLUMN npc_avatars.metadata IS '表示に使う値（イベントの値。名前・状態・使う NPC のデッキの公開 ID の一覧・更新の番号（集約の版）・登録と更新の日時と、操作したオペレーターの公開 ID）';
