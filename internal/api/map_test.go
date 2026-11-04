package api

import (
	"testing"

	"github.com/arkhayds/axolot/cases"
	"github.com/arkhayds/axolot/internal/runner"
	"github.com/arkhayds/axolot/internal/store"
)

func TestComputeEstimate(t *testing.T) {
	load := 1000.0
	rows := []store.EstimateRow{
		{Model: "m", Category: "code", CaseID: "c", ItemID: "known", AvgMs: 100, P90Ms: 200},
		{Model: "m", Category: "code", AvgMs: 300, P90Ms: 500},
		{Model: "m", LoadMs: &load},
	}
	plan := func(model string, items ...string) []runner.Plan {
		c := cases.Case{ID: "c", Category: "code"}
		for _, id := range items {
			c.Items = append(c.Items, cases.Item{ID: id})
		}
		return []runner.Plan{{Case: c, Targets: []runner.Target{{Model: model}}}}
	}

	if got := computeEstimate(rows, plan("m", "known"), nil); got != (Estimate{Answers: 1, LowMs: 1100, HighMs: 1200, Known: true}) {
		t.Fatalf("known question: %+v", got)
	}
	if got := computeEstimate(rows, plan("m", "known", "new"), nil); got != (Estimate{Answers: 2, LowMs: 1400, HighMs: 1700, Partial: true, Known: true}) {
		t.Fatalf("theme fallback, load once: %+v", got)
	}
	if got := computeEstimate(rows, plan("other", "known"), nil); got.Known || got.Answers != 1 {
		t.Fatalf("unknown model must not get a figure: %+v", got)
	}
	played := []store.PlayedItem{{Model: "m", CaseID: "c", ItemID: "known"}}
	if got := computeEstimate(rows, plan("m", "known", "new"), played); got != (Estimate{Answers: 1, LowMs: 300, HighMs: 500, Partial: true, Known: true}) {
		t.Fatalf("remaining skips played answers and the load already done: %+v", got)
	}
}
