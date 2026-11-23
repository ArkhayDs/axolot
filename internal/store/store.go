// Package store is the PostgreSQL access layer: settings, connections, selections and RAG chunks.
// Campaign results live in results.go.
package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/arkhayds/axolot/internal/rag"
)

//go:embed migrations/*.sql
var migrations embed.FS

const settingsKey = "app"

// migrationLock serializes concurrent boots; any stable bigint works.
const migrationLock = 7240117

// Store wraps the connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// Settings is everything tunable from the UI.
type Settings struct {
	CorpusDir       string `json:"corpus_dir"`
	ChunkSize       int    `json:"chunk_size"`
	ChunkOverlap    int    `json:"chunk_overlap"`
	TopK            int    `json:"top_k"`
	EmbedModel      string `json:"embed_model"`
	EmbedProviderID int64  `json:"embed_provider_id"`
	ReservedGB      int    `json:"reserved_gb"`
}

// DefaultSettings is used until the user saves their own.
func DefaultSettings() Settings {
	return Settings{CorpusDir: "demo", ChunkSize: 800, ChunkOverlap: 120, TopK: 5, EmbedModel: "bge-m3", ReservedGB: 12}
}

// Provider is a saved model connection.
type Provider struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	BaseURL   string `json:"base_url"`
	HasAPIKey bool   `json:"has_api_key"`
	APIKeyEnc []byte `json:"-"`
}

// Selection is a model chosen for a case.
type Selection struct {
	CaseID     string `json:"case_id"`
	ProviderID int64  `json:"provider_id"`
	Model      string `json:"model"`
}

// CorpusIndex describes one indexed corpus.
type CorpusIndex struct {
	Corpus     string `json:"corpus"`
	EmbedModel string `json:"embed_model"`
	Chunks     int    `json:"chunks"`
	Files      int    `json:"files"`
}

// Open connects and applies pending migrations.
func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("migrate database: %w", err)
	}
	return s, nil
}

// Close releases the pool.
func (s *Store) Close() {
	s.pool.Close()
}

// migrate applies each unseen file in name order, inside one transaction per file.
func (s *Store) migrate(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLock); err != nil {
		return err
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrationLock)

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY)`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrations, "migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		var done bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE name = $1)`, entry.Name()).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		sql, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sql)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, entry.Name())
			return err
		})
		if err != nil {
			return fmt.Errorf("%s: %w", entry.Name(), err)
		}
	}
	return nil
}

// Settings returns saved settings, or the defaults.
func (s *Store) Settings(ctx context.Context) (Settings, error) {
	settings := DefaultSettings()
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, settingsKey).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return settings, fmt.Errorf("read settings: %w", err)
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return settings, fmt.Errorf("decode settings: %w", err)
	}
	return settings, nil
}

// SaveSettings replaces the settings.
func (s *Store) SaveSettings(ctx context.Context, settings Settings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, settingsKey, raw)
	if err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}

// Providers lists connections, oldest first.
func (s *Store) Providers(ctx context.Context) ([]Provider, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, kind, base_url, api_key_enc FROM providers ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Provider, error) {
		var p Provider
		err := row.Scan(&p.ID, &p.Name, &p.Kind, &p.BaseURL, &p.APIKeyEnc)
		p.HasAPIKey = len(p.APIKeyEnc) > 0
		return p, err
	})
}

// ErrNameTaken reports a name already used by another row of the same table.
var ErrNameTaken = errors.New("name already taken")

// nameTaken tells whether an insert failed on a unique constraint (only `name` is unique where it is used)
func nameTaken(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// CreateProvider saves a connection.
// @param apiKeyEnc - already encrypted key, nil when none
// @returns ErrNameTaken when the name exists
func (s *Store) CreateProvider(ctx context.Context, name, kind, baseURL string, apiKeyEnc []byte) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO providers (name, kind, base_url, api_key_enc)
		VALUES ($1, $2, $3, $4) RETURNING id`, name, kind, baseURL, apiKeyEnc).Scan(&id)
	if nameTaken(err) {
		return 0, ErrNameTaken
	}
	if err != nil {
		return 0, fmt.Errorf("create provider: %w", err)
	}
	return id, nil
}

// DeleteProvider removes a connection and its selections; past results keep the provider name.
func (s *Store) DeleteProvider(ctx context.Context, id int64) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM providers WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete provider: %w", err)
	}
	return nil
}

// Selections returns every chosen model.
func (s *Store) Selections(ctx context.Context) ([]Selection, error) {
	rows, err := s.pool.Query(ctx, `SELECT case_id, provider_id, model FROM case_models ORDER BY case_id, provider_id, model`)
	if err != nil {
		return nil, fmt.Errorf("list selections: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Selection])
}

// SetCaseModels replaces the models chosen for a case.
func (s *Store) SetCaseModels(ctx context.Context, caseID string, selections []Selection) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM case_models WHERE case_id = $1`, caseID); err != nil {
			return err
		}
		for _, sel := range selections {
			if _, err := tx.Exec(ctx, `INSERT INTO case_models (case_id, provider_id, model) VALUES ($1, $2, $3)`, caseID, sel.ProviderID, sel.Model); err != nil {
				return err
			}
		}
		return nil
	})
}

// ReplaceChunks atomically swaps a corpus index for one embedding model.
func (s *Store) ReplaceChunks(ctx context.Context, corpus, embedModel string, chunks []rag.Chunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("got %d vectors for %d chunks", len(vectors), len(chunks))
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM chunks WHERE corpus = $1 AND embed_model = $2`, corpus, embedModel); err != nil {
			return err
		}
		for i, chunk := range chunks {
			_, err := tx.Exec(ctx, `INSERT INTO chunks (corpus, embed_model, file, position, start_offset, content, embedding)
				VALUES ($1, $2, $3, $4, $5, $6, $7::vector)`,
				corpus, embedModel, chunk.File, chunk.Position, chunk.Start, chunk.Content, vectorLiteral(vectors[i]))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("replace chunks: %w", err)
	}
	return nil
}

// SearchChunks runs an exact cosine search: on a small corpus an index would add nothing.
func (s *Store) SearchChunks(ctx context.Context, corpus, embedModel string, vector []float32, limit int) ([]rag.Hit, error) {
	rows, err := s.pool.Query(ctx, `SELECT file, position, start_offset, content, 1 - (embedding <=> $3::vector)
		FROM chunks WHERE corpus = $1 AND embed_model = $2
		ORDER BY embedding <=> $3::vector LIMIT $4`, corpus, embedModel, vectorLiteral(vector), limit)
	if err != nil {
		return nil, fmt.Errorf("search chunks: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (rag.Hit, error) {
		var hit rag.Hit
		err := row.Scan(&hit.File, &hit.Position, &hit.Start, &hit.Content, &hit.Similarity)
		return hit, err
	})
}

// CorpusIndexes lists what has been indexed.
func (s *Store) CorpusIndexes(ctx context.Context) ([]CorpusIndex, error) {
	rows, err := s.pool.Query(ctx, `SELECT corpus, embed_model, count(*)::int, count(DISTINCT file)::int
		FROM chunks GROUP BY corpus, embed_model ORDER BY corpus, embed_model`)
	if err != nil {
		return nil, fmt.Errorf("list corpus indexes: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[CorpusIndex])
}

// vectorLiteral formats a vector the way pgvector parses text input, avoiding a dedicated dependency.
func vectorLiteral(vector []float32) string {
	parts := make([]string, len(vector))
	for i, v := range vector {
		parts[i] = strconv.FormatFloat(float64(v), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}
