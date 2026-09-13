CREATE TABLE panda_delivery_items (
    delivery_id INTEGER NOT NULL REFERENCES panda_deliveries(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    gallery_id INTEGER NOT NULL,
    state TEXT NOT NULL,
    cleanup_attempts INTEGER NOT NULL,
    next_cleanup INTEGER NOT NULL,
    data TEXT NOT NULL,
    PRIMARY KEY (delivery_id, position),
    UNIQUE (delivery_id, gallery_id)
);

CREATE INDEX panda_delivery_items_unfinished ON panda_delivery_items (delivery_id, position)
    WHERE state IN ('queued', 'transferring', 'transferred', 'saved', 'importing');
CREATE INDEX panda_delivery_items_state ON panda_delivery_items (delivery_id, state);
CREATE INDEX panda_delivery_items_cleanup ON panda_delivery_items (state, cleanup_attempts, next_cleanup);

INSERT INTO panda_delivery_items (delivery_id, position, gallery_id, state, cleanup_attempts, next_cleanup, data)
SELECT d.id, item.key, json_extract(item.value, '$.gallery_id'),
       json_extract(item.value, '$.state'), json_extract(item.value, '$.cleanup_attempts'),
       CAST((julianday(json_extract(d.data, '$.checkpoints[' || item.key || '].next_cleanup')) - 2440587.5) * 86400000 AS INTEGER),
       json_object('item', json(item.value), 'checkpoint', json_extract(d.data, '$.checkpoints[' || item.key || ']'))
FROM panda_deliveries d, json_each(d.data, '$.batch.items') item;

UPDATE panda_deliveries SET data = json_remove(data, '$.batch.items', '$.checkpoints');
