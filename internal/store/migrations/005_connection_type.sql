-- How the answer was served: local model, cloud API, or cloud subscription through the Claude bridge.
-- Copied on the run, like provider_name, so a renamed or deleted connection keeps its history.
-- Older answers: those of a still existing Ollama connection become local, the others stay NULL (unknown).
ALTER TABLE runs ADD COLUMN connection_type text CHECK (connection_type IN ('local', 'api', 'subscription'));
UPDATE runs r SET connection_type = 'local' FROM providers p WHERE p.name = r.provider_name AND p.kind = 'ollama';
