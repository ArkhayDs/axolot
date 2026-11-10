// Package analytics turns campaign statistics into per-task verdicts readable by a non-technical audience.
package analytics

// compromiseMargin is how many score points below the best a model may stay and still count as equivalent.
const compromiseMargin = 10.0

// ModelStats aggregates one model's runs; an empty Category means all categories.
// Net times leave out the Claude CLI's own time, nil when no answer went through the bridge.
type ModelStats struct {
	Provider       string   `json:"provider"`
	Model          string   `json:"model"`
	Category       string   `json:"category"`
	Runs           int      `json:"runs"`
	Errors         int      `json:"errors"`
	AvgScore       *float64 `json:"avg_score"`
	P50TTFTMs      *float64 `json:"p50_ttft_ms"`
	P50TotalMs     *float64 `json:"p50_total_ms"`
	P50NetTTFTMs   *float64 `json:"p50_net_ttft_ms"`
	P50NetTotalMs  *float64 `json:"p50_net_total_ms"`
	P50TokPerSec   *float64 `json:"p50_tok_per_sec"`
	MaxLoadMs      *int64   `json:"max_load_ms"`
	RetrievalHits  int      `json:"retrieval_hits"`
	RetrievalTotal int      `json:"retrieval_total"`
}

// Pick names a winning model with its score.
type Pick struct {
	Model    string  `json:"model"`
	Provider string  `json:"provider"`
	Score    float64 `json:"score"`
}

// Verdict gives the winners of one category (empty = overall).
type Verdict struct {
	Category   string `json:"category"`
	Quality    *Pick  `json:"quality"`
	Speed      *Pick  `json:"speed"`
	Compromise *Pick  `json:"compromise"`
}

// Verdicts picks winners per category, in input order.
// Models without any measured answer are excluded: their zero speed must not win anything.
func Verdicts(stats []ModelStats) []Verdict {
	byCategory := map[string][]ModelStats{}
	var order []string
	for _, s := range stats {
		if s.AvgScore == nil {
			continue
		}
		if _, seen := byCategory[s.Category]; !seen {
			order = append(order, s.Category)
		}
		byCategory[s.Category] = append(byCategory[s.Category], s)
	}
	verdicts := make([]Verdict, 0, len(order))
	for _, category := range order {
		verdicts = append(verdicts, verdictFor(category, byCategory[category]))
	}
	return verdicts
}

// verdictFor computes the three picks of one category.
// Compromise: fastest model whose score stays within compromiseMargin of the best.
func verdictFor(category string, stats []ModelStats) Verdict {
	verdict := Verdict{Category: category}
	var quality, speed, compromise *ModelStats
	for i := range stats {
		s := &stats[i]
		if quality == nil || isBetterQuality(s, quality) {
			quality = s
		}
		if isFaster(s, speed) {
			speed = s
		}
	}
	for i := range stats {
		s := &stats[i]
		if *s.AvgScore >= *quality.AvgScore-compromiseMargin && isFaster(s, compromise) {
			compromise = s
		}
	}
	verdict.Quality, verdict.Speed, verdict.Compromise = toPick(quality), toPick(speed), toPick(compromise)
	return verdict
}

// isBetterQuality prefers the higher score, then the faster model on a tie.
func isBetterQuality(a, b *ModelStats) bool {
	if *a.AvgScore != *b.AvgScore {
		return *a.AvgScore > *b.AvgScore
	}
	return isFaster(a, b)
}

// isFaster compares median total time, net of the CLI when measured; a model without timing never wins.
func isFaster(a, b *ModelStats) bool {
	if modelMs(a) == nil {
		return false
	}
	return b == nil || modelMs(b) == nil || *modelMs(a) < *modelMs(b)
}

// modelMs is the model's own median time: the CLI startup comes from the tooling, not the model.
func modelMs(s *ModelStats) *float64 {
	if s.P50NetTotalMs != nil {
		return s.P50NetTotalMs
	}
	return s.P50TotalMs
}

// toPick converts a winner, nil staying nil.
func toPick(s *ModelStats) *Pick {
	if s == nil {
		return nil
	}
	return &Pick{Model: s.Model, Provider: s.Provider, Score: *s.AvgScore}
}
