package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// MapCell aggregates a model's answers; an empty Machine or Category is a rollup over that axis.
// Rollups come from the database because medians of cells cannot be combined afterwards.
type MapCell struct {
	Machine      string   `json:"machine"`
	Provider     string   `json:"provider"`
	Model        string   `json:"model"`
	Category     string   `json:"category"`
	Runs         int      `json:"runs"`
	Errors       int      `json:"errors"`
	AvgScore     float64  `json:"avg_score"`
	P50TotalMs   *float64 `json:"p50_total_ms"`
	P50TokPerSec *float64 `json:"p50_tok_per_sec"`
}

// MapBenchmark summarizes a campaign for the map; Cells holds one {model, category, runs, avg_score} per pair.
type MapBenchmark struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
	DoneItems  int             `json:"done_items"`
	Machine    string          `json:"machine"`
	Models     []string        `json:"models"`
	Categories []string        `json:"categories"`
	Cells      json.RawMessage `json:"cells"`
}

// MapCells aggregates the most recent answer to each question per machine and model, so replaying
// a benchmark replaces its answers instead of counting them twice. Running campaigns wait for their end.
// shortcut: a tag served by two connections counts as one model, split on provider if that ever happens.
func (s *Store) MapCells(ctx context.Context) ([]MapCell, error) {
	rows, err := s.pool.Query(ctx, `WITH latest AS (
			SELECT DISTINCT ON (c.hardware->>'label', r.model, r.case_id, r.item_id)
				c.hardware->>'label' AS machine, r.provider_name, r.model, r.category, r.error, r.score, r.total_ms, r.tok_per_sec
			FROM runs r JOIN campaigns c ON c.id = r.campaign_id
			WHERE c.status <> $1
			ORDER BY c.hardware->>'label', r.model, r.case_id, r.item_id, c.created_at DESC, r.created_at DESC
		)
		SELECT COALESCE(machine, ''), min(provider_name), model, COALESCE(category, ''),
			count(*)::int,
			count(*) FILTER (WHERE error <> '')::int,
			avg(score),
			percentile_cont(0.5) WITHIN GROUP (ORDER BY total_ms),
			percentile_cont(0.5) WITHIN GROUP (ORDER BY tok_per_sec)
		FROM latest
		GROUP BY GROUPING SETS ((machine, model, category), (machine, model), (model, category), (model))
		ORDER BY 1, 3, 4`, StatusRunning)
	if err != nil {
		return nil, fmt.Errorf("compute map cells: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MapCell])
}

// MapBenchmarks lists campaigns newest first, with what each one played.
func (s *Store) MapBenchmarks(ctx context.Context) ([]MapBenchmark, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.id::text, c.name, c.status, c.created_at,
			(SELECT count(*)::int FROM runs r WHERE r.campaign_id = c.id),
			COALESCE(c.hardware->>'label', ''),
			ARRAY(SELECT DISTINCT r.model FROM runs r WHERE r.campaign_id = c.id ORDER BY 1),
			ARRAY(SELECT DISTINCT r.category FROM runs r WHERE r.campaign_id = c.id ORDER BY 1),
			COALESCE((SELECT jsonb_agg(jsonb_build_object('model', p.model, 'category', p.category, 'runs', p.runs, 'avg_score', p.avg_score))
				FROM (SELECT r.model, r.category, count(*) AS runs, avg(r.score) AS avg_score
					FROM runs r WHERE r.campaign_id = c.id GROUP BY r.model, r.category) p), '[]'::jsonb)
		FROM campaigns c ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list map benchmarks: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MapBenchmark])
}
