DELETE FROM panda_deliveries
WHERE state IN ('completed', 'stopped')
  AND NOT EXISTS (
      SELECT 1 FROM panda_delivery_items
      WHERE delivery_id = panda_deliveries.id
        AND state NOT IN ('completed', 'skipped')
  );
