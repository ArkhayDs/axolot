-- Older Claude answers whose connection was deleted stayed of unknown type: until connection types, Claude was only
-- reached through the subscription bridge, so they are subscription answers.
UPDATE runs SET connection_type = 'subscription' WHERE connection_type IS NULL AND model LIKE 'claude-%';
