-- The Claude bridge gets its own connection kind, so classifying its answers no longer compares URLs with the environment.
-- Its row is found by the name the server always gives it.
ALTER TABLE providers DROP CONSTRAINT providers_kind_check;
ALTER TABLE providers ADD CONSTRAINT providers_kind_check CHECK (kind IN ('ollama', 'openai', 'bridge'));
UPDATE providers SET kind = 'bridge' WHERE name = 'Claude (abonnement)';
-- Older answers of a still existing connection take its type. Claude answers whose connection was deleted came through
-- the bridge, the only way to reach Claude until connection types.
UPDATE runs r SET connection_type = CASE p.kind WHEN 'ollama' THEN 'local' WHEN 'bridge' THEN 'subscription' ELSE 'api' END
FROM providers p WHERE p.name = r.provider_name AND r.connection_type IS NULL;
UPDATE runs SET connection_type = 'subscription' WHERE connection_type IS NULL AND model LIKE 'claude-%';
