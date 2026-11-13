-- The model sampled at its own default temperature instead of the requested one: a reasoning model retried without it,
-- or the Claude CLI, which has no temperature option. Older answers through an API key cannot be told apart and stay unmarked.
ALTER TABLE runs ADD COLUMN default_temperature boolean NOT NULL DEFAULT false;
UPDATE runs SET default_temperature = true WHERE connection_type = 'subscription';
