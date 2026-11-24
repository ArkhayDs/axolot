package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// McpServer is a saved MCP server.
type McpServer struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Enabled  bool   `json:"enabled"`
	TokenEnc []byte `json:"-"`
	OAuthEnc []byte `json:"-"`
	// ToolModes maps a tool name, as the server gives it, to allow, ask or block; unset tools are absent.
	ToolModes map[string]string `json:"-"`
}

// McpServers lists MCP servers, oldest first.
func (s *Store) McpServers(ctx context.Context) ([]McpServer, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, url, enabled, token_enc, oauth_enc, tool_modes FROM mcp_servers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (McpServer, error) {
		var m McpServer
		err := row.Scan(&m.ID, &m.Name, &m.URL, &m.Enabled, &m.TokenEnc, &m.OAuthEnc, &m.ToolModes)
		return m, err
	})
}

// McpServer loads one MCP server.
// @returns ErrNotFound when no server has this id
func (s *Store) McpServer(ctx context.Context, id int64) (McpServer, error) {
	var m McpServer
	err := s.pool.QueryRow(ctx, `SELECT id, name, url, enabled, token_enc, oauth_enc, tool_modes FROM mcp_servers WHERE id = $1`, id).
		Scan(&m.ID, &m.Name, &m.URL, &m.Enabled, &m.TokenEnc, &m.OAuthEnc, &m.ToolModes)
	if errors.Is(err, pgx.ErrNoRows) {
		return McpServer{}, ErrNotFound
	}
	if err != nil {
		return McpServer{}, fmt.Errorf("load mcp server: %w", err)
	}
	return m, nil
}

// SetMcpServerOAuth saves the OAuth credentials of a server, replacing previous ones.
// @param oauthEnc - already encrypted credentials
// @returns ErrNotFound when no server has this id
func (s *Store) SetMcpServerOAuth(ctx context.Context, id int64, oauthEnc []byte) error {
	tag, err := s.pool.Exec(ctx, `UPDATE mcp_servers SET oauth_enc = $2 WHERE id = $1`, id, oauthEnc)
	if err != nil {
		return fmt.Errorf("set mcp server oauth: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetMcpToolMode saves the permission of one tool, keeping the other tools' ones.
// @param mode - allow, ask or block, checked by the caller
// @returns ErrNotFound when no server has this id
func (s *Store) SetMcpToolMode(ctx context.Context, id int64, tool, mode string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE mcp_servers SET tool_modes = tool_modes || jsonb_build_object($2::text, $3::text) WHERE id = $1`,
		id, tool, mode)
	if err != nil {
		return fmt.Errorf("set mcp tool mode: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateMcpServer saves an enabled MCP server.
// @param tokenEnc - already encrypted token, nil when none
// @returns ErrNameTaken when the name exists
func (s *Store) CreateMcpServer(ctx context.Context, name, url string, tokenEnc []byte) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO mcp_servers (name, url, token_enc) VALUES ($1, $2, $3) RETURNING id`,
		name, url, tokenEnc).Scan(&id)
	if nameTaken(err, "mcp_servers") {
		return 0, ErrNameTaken
	}
	if err != nil {
		return 0, fmt.Errorf("create mcp server: %w", err)
	}
	return id, nil
}

// SetMcpServerEnabled turns a server's tools on or off for the Chat.
// @returns ErrNotFound when no server has this id
func (s *Store) SetMcpServerEnabled(ctx context.Context, id int64, enabled bool) error {
	tag, err := s.pool.Exec(ctx, `UPDATE mcp_servers SET enabled = $2 WHERE id = $1`, id, enabled)
	if err != nil {
		return fmt.Errorf("set mcp server enabled: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMcpServer removes an MCP server.
// @returns ErrNotFound when no server has this id
func (s *Store) DeleteMcpServer(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete mcp server: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
