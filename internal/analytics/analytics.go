// Package analytics turns campaign statistics into per-task verdicts readable by a non-technical audience.
package analytics

// compromiseMargin is how many score points below the best a model may stay and still count as equivalent.
const compromiseMargin = 10.0

// ModelStats aggregates one model's runs; an empty Category means all categories.
type ModelStats struct {
	Provider       string   `json:"provider"`
	Model          string   `json:"model"`
	Category       string   `json:"category"`
	Runs           int      `json:"runs"`
	Errors         int      `json:"errors"`
	AvgScore       float64  `json:"avg_score"`
	P50TTFTMs      *float64 `json:"p50_ttft_ms"`
	P50TotalMs     *float64 `json:"p50_total_ms"`
	P50TokPerSec   *float64 `json:"p50_tok_per_sec"`
	MaxLoadMs      *int64   `json:"max_load_ms"`
	RetrievalHits  int      `json:"retrieval_hits"`
	RetrievalTotal int      `json:"retrieval_total"`
}

// Pick names a winning model with its figures.
type Pick struct {
	Model      string   `json:"model"`
	Provider   string   `json:"provider"`
	Score      float64  `json:"score"`
	P50TotalMs *float64 `json:"p50_total_ms"`
}

// Verdict gives the winners of one category (empty = overall).
type Verdict struct {
	Category   string `json:"category"`
	Quality    *Pick  `json:"quality"`
	Speed      *Pick  `json:"speed"`
	Compromise *Pick  `json:"compromise"`
}

// Verdicts picks winners per category, in input order.
// Models failing every run are excluded: their zero speed must not win anything.
func Verdicts(stats []ModelStats) []Verdict {
	byCategory := map[string][]ModelStats{}
	var order []string
	for _, s := range stats {
		if s.Errors == s.Runs {
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
		if s.AvgScore >= quality.AvgScore-compromiseMargin && isFaster(s, compromise) {
			compromise = s
		}
	}
	verdict.Quality, verdict.Speed, verdict.Compromise = toPick(quality), toPick(speed), toPick(compromise)
	return verdict
}

// isBetterQuality prefers the higher score, then the faster model on a tie.
func isBetterQuality(a, b *ModelStats) bool {
	if a.AvgScore != b.AvgScore {
		return a.AvgScore > b.AvgScore
	}
	return isFaster(a, b)
}

// isFaster compares median total time; a model without timing never wins.
func isFaster(a, b *ModelStats) bool {
	if a.P50TotalMs == nil {
		return false
	}
	return b == nil || b.P50TotalMs == nil || *a.P50TotalMs < *b.P50TotalMs
}

// toPick converts a winner, nil staying nil.
func toPick(s *ModelStats) *Pick {
	if s == nil {
		return nil
	}
	return &Pick{Model: s.Model, Provider: s.Provider, Score: s.AvgScore, P50TotalMs: s.P50TotalMs}
}
