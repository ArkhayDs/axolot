-- Per-tool permission of an MCP server, keyed by the tool name the server gives: allow, ask or block.
-- A tool missing from the map gets its default from the server's read-only hint when listed.
ALTER TABLE mcp_servers ADD COLUMN tool_modes jsonb NOT NULL DEFAULT '{}';
