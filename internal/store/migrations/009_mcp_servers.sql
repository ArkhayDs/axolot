-- MCP servers whose tools the Chat hands to models. The token is sealed like API keys, NULL when the server needs none.
-- enabled is kept across sessions, unlike the Chat model picks.
CREATE TABLE mcp_servers (
    id bigserial PRIMARY KEY,
    name text NOT NULL UNIQUE,
    url text NOT NULL,
    token_enc bytea,
    enabled boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);
