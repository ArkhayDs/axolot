package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNameTaken reports a name already used by another row of the same table.
var ErrNameTaken = errors.New("name already taken")

// McpServer is a saved MCP server.
type McpServer struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Enabled  bool   `json:"enabled"`
	HasToken bool   `json:"has_token"`
	TokenEnc []byte `json:"-"`
}

// McpServers lists MCP servers, oldest first.
func (s *Store) McpServers(ctx context.Context) ([]McpServer, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, url, enabled, token_enc FROM mcp_servers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (McpServer, error) {
		var m McpServer
		err := row.Scan(&m.ID, &m.Name, &m.URL, &m.Enabled, &m.TokenEnc)
		m.HasToken = len(m.TokenEnc) > 0
		return m, err
	})
}

// CreateMcpServer saves an enabled MCP server.
// @param tokenEnc - already encrypted token, nil when none
// @returns ErrNameTaken when the name exists
func (s *Store) CreateMcpServer(ctx context.Context, name, url string, tokenEnc []byte) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO mcp_servers (name, url, token_enc) VALUES ($1, $2, $3) RETURNING id`,
		name, url, tokenEnc).Scan(&id)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return 0, ErrNameTaken
	}
	if err != nil {
		return 0, fmt.Errorf("create mcp server: %w", err)
	}
	return id, nil
}

// SetMcpServerEnabled turns a server's tools on or off for the Chat.
func (s *Store) SetMcpServerEnabled(ctx context.Context, id int64, enabled bool) error {
	if _, err := s.pool.Exec(ctx, `UPDATE mcp_servers SET enabled = $2 WHERE id = $1`, id, enabled); err != nil {
		return fmt.Errorf("set mcp server enabled: %w", err)
	}
	return nil
}

// DeleteMcpServer removes an MCP server.
func (s *Store) DeleteMcpServer(ctx context.Context, id int64) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete mcp server: %w", err)
	}
	return nil
}
