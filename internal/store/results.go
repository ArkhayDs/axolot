package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/arkhayds/axolot/internal/analytics"
)

// Campaign statuses.
const (
	StatusRunning     = "running"
	StatusDone        = "done"
	StatusCancelled   = "cancelled"
	StatusInterrupted = "interrupted"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("not found")

// Campaign is one benchmark session, with the hardware it ran on.
type Campaign struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Status     string          `json:"status"`
	Hardware   json.RawMessage `json:"hardware"`
	RAGConfig  json.RawMessage `json:"rag_config"`
	TotalItems int             `json:"total_items"`
	DoneItems  int             `json:"done_items"`
	CreatedAt  time.Time       `json:"created_at"`
	FinishedAt *time.Time      `json:"finished_at"`
}

// Run is one model answering one item.
type Run struct {
	ID           string          `json:"id"`
	CampaignID   string          `json:"campaign_id"`
	CaseID       string          `json:"case_id"`
	ItemID       string          `json:"item_id"`
	Category     string          `json:"category"`
	ProviderName string          `json:"provider_name"`
	Model        string          `json:"model"`
	Output       string          `json:"output"`
	Error        string          `json:"error"`
	TTFTMs       *int64          `json:"ttft_ms"`
	TotalMs      int64           `json:"total_ms"`
	LoadMs       *int64          `json:"load_ms"`
	CLIStartupMs *int64          `json:"cli_startup_ms"`
	CLIExitMs    *int64          `json:"cli_exit_ms"`
	PromptTokens *int            `json:"prompt_tokens"`
	OutputTokens *int            `json:"output_tokens"`
	TokPerSec    *float64        `json:"tok_per_sec"`
	Score        float64         `json:"score"`
	Checks       json.RawMessage `json:"checks"`
	Retrieved    json.RawMessage `json:"retrieved"`
	RetrievalHit *bool           `json:"retrieval_hit"`
	ItemHash     string          `json:"item_hash"`
}

// CreateCampaign opens a campaign and returns its id.
func (s *Store) CreateCampaign(ctx context.Context, name string, hardware, ragConfig json.RawMessage, totalItems int) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `INSERT INTO campaigns (name, status, hardware, rag_config, total_items)
		VALUES ($1, $2, $3, $4, $5) RETURNING id::text`, name, StatusRunning, hardware, ragConfig, totalItems).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create campaign: %w", err)
	}
	return id, nil
}

// FinishCampaign records the final status.
func (s *Store) FinishCampaign(ctx context.Context, id, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE campaigns SET status = $2, finished_at = now() WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("finish campaign: %w", err)
	}
	return nil
}

// MarkInterrupted closes campaigns left running by a previous process.
func (s *Store) MarkInterrupted(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE campaigns SET status = $1, finished_at = now() WHERE status = $2`, StatusInterrupted, StatusRunning)
	if err != nil {
		return fmt.Errorf("mark interrupted campaigns: %w", err)
	}
	return nil
}

// runColumns must stay in the order of the Run fields.
const runColumns = `r.id::text, r.campaign_id::text, r.case_id, r.item_id, r.category, r.provider_name, r.model, r.output, r.error,
	r.ttft_ms, r.total_ms, r.load_ms, r.cli_startup_ms, r.cli_exit_ms, r.prompt_tokens, r.output_tokens, r.tok_per_sec, r.score, r.checks, r.retrieved,
	r.retrieval_hit, COALESCE(r.item_hash, '')`

const campaignColumns = `c.id::text, c.name, c.status, c.hardware, c.rag_config, c.total_items,
	(SELECT count(*)::int FROM runs r WHERE r.campaign_id = c.id), c.created_at, c.finished_at`

// Campaigns lists campaigns, newest first.
func (s *Store) Campaigns(ctx context.Context) ([]Campaign, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+campaignColumns+` FROM campaigns c ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Campaign])
}

// CampaignByID returns one campaign or ErrNotFound.
func (s *Store) CampaignByID(ctx context.Context, id string) (Campaign, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+campaignColumns+` FROM campaigns c WHERE c.id = $1::uuid`, id)
	if err != nil {
		return Campaign{}, fmt.Errorf("read campaign: %w", err)
	}
	campaign, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[Campaign])
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, ErrNotFound
	}
	return campaign, err
}

// CreateRun stores one result.
func (s *Store) CreateRun(ctx context.Context, run Run) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO runs (campaign_id, case_id, item_id, category, provider_name, model, output, error,
		ttft_ms, total_ms, load_ms, cli_startup_ms, cli_exit_ms, prompt_tokens, output_tokens, tok_per_sec, score, checks, retrieved, retrieval_hit,
		item_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)`,
		run.CampaignID, run.CaseID, run.ItemID, run.Category, run.ProviderName, run.Model, run.Output, run.Error,
		run.TTFTMs, run.TotalMs, run.LoadMs, run.CLIStartupMs, run.CLIExitMs, run.PromptTokens, run.OutputTokens, run.TokPerSec, run.Score,
		run.Checks, run.Retrieved, run.RetrievalHit, run.ItemHash)
	if err != nil {
		return fmt.Errorf("create run: %w", err)
	}
	return nil
}

// Runs returns the results of a campaign, grouped by case then item.
func (s *Store) Runs(ctx context.Context, campaignID string) ([]Run, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+runColumns+`
		FROM runs r WHERE r.campaign_id = $1::uuid ORDER BY r.case_id, r.item_id, r.provider_name, r.model`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Run])
}

// statsFigures must stay in the order of the analytics.ModelStats fields from P50TTFTMs.
// Net times count an answer without CLI measure as 0, and stay NULL when no answer of the group has one.
const statsFigures = `percentile_cont(0.5) WITHIN GROUP (ORDER BY ttft_ms) FILTER (WHERE error = ''),
	percentile_cont(0.5) WITHIN GROUP (ORDER BY total_ms) FILTER (WHERE error = ''),
	CASE WHEN count(cli_startup_ms) FILTER (WHERE error = '') > 0 THEN percentile_cont(0.5)
		WITHIN GROUP (ORDER BY ttft_ms - COALESCE(cli_startup_ms, 0)) FILTER (WHERE error = '') END,
	CASE WHEN count(cli_exit_ms) FILTER (WHERE error = '') > 0 THEN percentile_cont(0.5)
		WITHIN GROUP (ORDER BY total_ms - COALESCE(cli_startup_ms, 0) - COALESCE(cli_exit_ms, 0)) FILTER (WHERE error = '') END,
	percentile_cont(0.5) WITHIN GROUP (ORDER BY tok_per_sec) FILTER (WHERE error = ''),
	max(load_ms),
	count(*) FILTER (WHERE retrieval_hit)::int,
	count(retrieval_hit)::int`

// Stats aggregates a campaign per model, overall (empty category) and per category.
// Medians, because a model's first run carries its load time and would skew an average.
// Scores and times count measured answers only: an error (bridge, quota, connection) says nothing about the model,
// and a quick failure would pass for a fast answer.
func (s *Store) Stats(ctx context.Context, campaignID string) ([]analytics.ModelStats, error) {
	rows, err := s.pool.Query(ctx, `SELECT provider_name, model, COALESCE(category, ''),
			count(*)::int,
			count(*) FILTER (WHERE error <> '')::int,
			avg(score) FILTER (WHERE error = ''),
			`+statsFigures+`
		FROM runs WHERE campaign_id = $1::uuid
		GROUP BY GROUPING SETS ((provider_name, model), (provider_name, model, category))
		ORDER BY 1, 2, 3`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("compute stats: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[analytics.ModelStats])
}
