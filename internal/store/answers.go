package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CaseAnswer is the latest answer of a model to a question, with what the whole base knows about that pair.
type CaseAnswer struct {
	Run
	Answers  int     `json:"answers"`
	AvgScore float64 `json:"avg_score"`
}

// CaseAnswers returns one row per model and question of a case, all benchmarks together.
// @param hashes - current question versions: an edited question must not show answers to its old wording
func (s *Store) CaseAnswers(ctx context.Context, caseID string, hashes []string) ([]CaseAnswer, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (r.model, r.item_id) `+runColumns+`,
			count(*) OVER same_question, avg(r.score) OVER same_question
		FROM runs r JOIN campaigns c ON c.id = r.campaign_id
		WHERE r.case_id = $1 AND r.item_hash = ANY($2)
		WINDOW same_question AS (PARTITION BY r.model, r.item_id)
		ORDER BY r.model, r.item_id, c.created_at DESC`, caseID, hashes)
	if err != nil {
		return nil, fmt.Errorf("list case answers: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (CaseAnswer, error) {
		var a CaseAnswer
		var answers int64
		err := row.Scan(&a.ID, &a.CampaignID, &a.CaseID, &a.ItemID, &a.Category, &a.ProviderName, &a.Model, &a.Output, &a.Error,
			&a.TTFTMs, &a.TotalMs, &a.LoadMs, &a.PromptTokens, &a.OutputTokens, &a.TokPerSec, &a.Score, &a.Checks, &a.Retrieved,
			&a.RetrievalHit, &a.ItemHash, &answers, &a.AvgScore)
		a.Answers = int(answers)
		return a, err
	})
}
