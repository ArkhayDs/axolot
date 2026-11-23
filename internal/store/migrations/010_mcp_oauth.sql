-- OAuth credentials of an MCP server signed in from the browser: registered client and tokens, sealed as one JSON value.
-- Rewritten on each refresh, since some servers hand out a new refresh token every time.
ALTER TABLE mcp_servers ADD COLUMN oauth_enc bytea;
