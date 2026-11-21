-- A favorite is new to the score page when the collector first observed it
-- after the active model was calculated. Existing observations get the time
-- of this migration; every model is calculated later, because 0012 deleted
-- the earlier ones.
ALTER TABLE favorite_observations ADD COLUMN observed_at TIMESTAMPTZ NOT NULL DEFAULT now();
