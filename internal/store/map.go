package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/arkhayds/axolot/internal/analytics"
)

// MapCell aggregates a model's cumulated answers; an empty Machine or Category is a rollup over that axis.
// Rollups come from the database because medians of cells cannot be combined afterwards.
type MapCell struct {
	Machine string `json:"machine"`
	analytics.ModelStats
	Questions  int      `json:"questions"`
	P95TotalMs *float64 `json:"p95_total_ms"`
}

// MapBenchmark summarizes a campaign for the map; Cells holds one {model, category, measured, avg_score} per pair.
type MapBenchmark struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
	FinishedAt *time.Time      `json:"finished_at"`
	DoneItems  int             `json:"done_items"`
	Machine    string          `json:"machine"`
	Models     []string        `json:"models"`
	Categories []string        `json:"categories"`
	Cells      json.RawMessage `json:"cells"`
}

// Answer is the part of a run the cumulated time distribution draws.
type Answer struct {
	ID       string `json:"id"`
	ItemID   string `json:"item_id"`
	Model    string `json:"model"`
	Category string `json:"category"`
	TotalMs  int64  `json:"total_ms"`
	Error    string `json:"error"`
}

// MapItem cumulates a model's answers to one question; an empty Machine is the rollup over machines.
type MapItem struct {
	Machine    string   `json:"machine"`
	Model      string   `json:"model"`
	CaseID     string   `json:"case_id"`
	ItemID     string   `json:"item_id"`
	Runs       int      `json:"runs"`
	AvgScore   *float64 `json:"avg_score"`
	P50TotalMs *float64 `json:"p50_total_ms"`
	Errors     int      `json:"errors"`
}

// cumulatedAnswers selects what the cumulated figures count: finished answers to the current version of each
// question. Scores average per (machine, model, question) first, so a question repeated ten times weighs as much
// as one played once; the weight does that in a single pass while times stay medians of the answers. An unmeasured
// answer (error) gets no weight and no time, a quick failure passing for a fast answer; a group without any has neither.
const cumulatedAnswers = `WITH answers AS (
		SELECT c.hardware->>'label' AS machine, r.*,
			CASE WHEN r.error = '' THEN 1.0::float8 / count(*) FILTER (WHERE r.error = '')
				OVER (PARTITION BY c.hardware->>'label', r.model, r.case_id, r.item_id) END AS weight
		FROM runs r JOIN campaigns c ON c.id = r.campaign_id
		WHERE c.status <> $1 AND r.item_hash = ANY($2)
	)`

// MapCells cumulates answers per machine, model and theme, with their rollups.
// shortcut: a tag served by two connections counts as one model, split on provider if that ever happens.
func (s *Store) MapCells(ctx context.Context, hashes []string) ([]MapCell, error) {
	rows, err := s.pool.Query(ctx, cumulatedAnswers+`
		SELECT COALESCE(machine, ''), min(provider_name), model, COALESCE(category, ''),
			count(*)::int,
			count(*) FILTER (WHERE error <> '')::int,
			sum(score * weight) / sum(weight),
			`+statsFigures+`,
			count(DISTINCT case_id || '/' || item_id)::int,
			percentile_cont(0.95) WITHIN GROUP (ORDER BY total_ms) FILTER (WHERE error = '')
		FROM answers
		GROUP BY GROUPING SETS ((machine, model, category), (machine, model), (model, category), (model))
		ORDER BY 1, 3, 4`, StatusRunning, hashes)
	if err != nil {
		return nil, fmt.Errorf("compute map cells: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MapCell])
}

// MapItems cumulates answers per machine, model and question, with the rollup over machines.
func (s *Store) MapItems(ctx context.Context, hashes []string) ([]MapItem, error) {
	rows, err := s.pool.Query(ctx, cumulatedAnswers+`
		SELECT COALESCE(machine, ''), model, case_id, item_id,
			count(*)::int,
			sum(score * weight) / sum(weight),
			percentile_cont(0.5) WITHIN GROUP (ORDER BY total_ms) FILTER (WHERE error = ''),
			count(*) FILTER (WHERE error <> '')::int
		FROM answers
		GROUP BY GROUPING SETS ((machine, model, case_id, item_id), (model, case_id, item_id))`, StatusRunning, hashes)
	if err != nil {
		return nil, fmt.Errorf("compute map items: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MapItem])
}

// MapAnswers lists what MapCells counted, without the heavy columns.
func (s *Store) MapAnswers(ctx context.Context, hashes []string) ([]Answer, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id::text, r.item_id, r.model, r.category, r.total_ms, r.error
		FROM runs r JOIN campaigns c ON c.id = r.campaign_id
		WHERE c.status <> $1 AND r.item_hash = ANY($2)`, StatusRunning, hashes)
	if err != nil {
		return nil, fmt.Errorf("list map answers: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Answer])
}

// EstimateRow holds a model's durations on one machine: per question when CaseID is set,
// per theme when only Category is, and overall with its load time when neither is.
type EstimateRow struct {
	Model    string   `json:"model"`
	Category string   `json:"category"`
	CaseID   string   `json:"case_id"`
	ItemID   string   `json:"item_id"`
	AvgMs    float64  `json:"avg_ms"`
	P90Ms    float64  `json:"p90_ms"`
	LoadMs   *float64 `json:"load_ms"`
}

// EstimateRows reads what duration estimates are built from, measured answers only. Means rather than medians,
// because a sum of medians underestimates a total that has long answers.
func (s *Store) EstimateRows(ctx context.Context, machine string, hashes []string) ([]EstimateRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.model, COALESCE(r.category, ''), COALESCE(r.case_id, ''), COALESCE(r.item_id, ''),
			avg(r.total_ms)::float8,
			percentile_cont(0.9) WITHIN GROUP (ORDER BY r.total_ms),
			percentile_cont(0.5) WITHIN GROUP (ORDER BY r.load_ms) FILTER (WHERE r.load_ms > 0)
		FROM runs r JOIN campaigns c ON c.id = r.campaign_id
		WHERE c.status <> $1 AND c.hardware->>'label' = $2 AND r.item_hash = ANY($3) AND r.error = ''
		GROUP BY GROUPING SETS ((r.model, r.category, r.case_id, r.item_id), (r.model, r.category), (r.model))`, StatusRunning, machine, hashes)
	if err != nil {
		return nil, fmt.Errorf("read estimate rows: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[EstimateRow])
}

// PlayedItem identifies one answer already given.
type PlayedItem struct {
	Model  string
	CaseID string
	ItemID string
}

// PlayedItems lists what a campaign has answered so far.
func (s *Store) PlayedItems(ctx context.Context, campaignID string) ([]PlayedItem, error) {
	rows, err := s.pool.Query(ctx, `SELECT model, case_id, item_id FROM runs WHERE campaign_id = $1::uuid`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list played items: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[PlayedItem])
}

// MapBenchmarks lists campaigns newest first, with what each one played.
func (s *Store) MapBenchmarks(ctx context.Context) ([]MapBenchmark, error) {
	rows, err := s.pool.Query(ctx, `SELECT c.id::text, c.name, c.status, c.created_at, c.finished_at,
			(SELECT count(*)::int FROM runs r WHERE r.campaign_id = c.id),
			COALESCE(c.hardware->>'label', ''),
			ARRAY(SELECT DISTINCT r.model FROM runs r WHERE r.campaign_id = c.id ORDER BY 1),
			ARRAY(SELECT DISTINCT r.category FROM runs r WHERE r.campaign_id = c.id ORDER BY 1),
			COALESCE((SELECT jsonb_agg(jsonb_build_object('model', p.model, 'category', p.category, 'measured', p.measured, 'avg_score', p.avg_score))
				FROM (SELECT r.model, r.category, count(*) FILTER (WHERE r.error = '') AS measured,
						avg(r.score) FILTER (WHERE r.error = '') AS avg_score
					FROM runs r WHERE r.campaign_id = c.id GROUP BY r.model, r.category) p), '[]'::jsonb)
		FROM campaigns c ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list map benchmarks: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[MapBenchmark])
}
