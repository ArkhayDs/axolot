package api

import (
	"slices"
	"testing"

	"github.com/arkhayds/axolot/cases"
	"github.com/arkhayds/axolot/internal/analytics"
	"github.com/arkhayds/axolot/internal/runner"
	"github.com/arkhayds/axolot/internal/store"
)

func TestComputeEstimate(t *testing.T) {
	load := 1000.0
	rows := []store.EstimateRow{
		{Model: "m", Category: "code", CaseID: "c", ItemID: "known", AvgMs: 100, P90Ms: 200},
		{Model: "m", Category: "code", AvgMs: 300, P90Ms: 500},
		{Model: "m", LoadMs: &load},
		{Model: "m (API · OpenAI)", Category: "code", AvgMs: 300, P90Ms: 500},
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
	viaAPI := func(connection string) []runner.Plan {
		p := plan("m", "known")
		p[0].Targets[0].ConnectionType, p[0].Targets[0].ProviderName = store.ConnectionAPI, connection
		return p
	}
	if got := computeEstimate(rows, viaAPI("OpenAI"), nil); got != (Estimate{Answers: 1, LowMs: 300, HighMs: 500, Partial: true, Known: true}) {
		t.Fatalf("an API model is estimated from its own connection, not the local one of the same name: %+v", got)
	}
	if got := computeEstimate(rows, viaAPI("OpenRouter"), nil); got.Known {
		t.Fatalf("an API model must not be estimated from another connection: %+v", got)
	}
	played := []store.PlayedItem{{Model: "m", CaseID: "c", ItemID: "known"}}
	if got := computeEstimate(rows, plan("m", "known", "new"), played); got != (Estimate{Answers: 1, LowMs: 300, HighMs: 500, Partial: true, Known: true}) {
		t.Fatalf("remaining skips played answers and the load already done: %+v", got)
	}
}

func TestBaseOfKeepsThePickedModels(t *testing.T) {
	quick, slow := 100.0, 900.0
	cell := func(machine, model, category string, ms *float64) store.MapCell {
		score := 90.0
		return store.MapCell{Machine: machine, ModelStats: analytics.ModelStats{Model: model, Category: category, AvgScore: &score, P50TotalMs: ms}}
	}
	cells := []store.MapCell{
		cell("", "a", "", &slow), cell("", "a", "code", &slow), cell("", "b", "", &quick), cell("", "b", "code", &quick),
		cell("mac", "a", "", &slow),
	}
	answers := []store.Answer{{Label: "a"}, {Label: "b"}}

	all := baseOf(cells, answers, nil)
	if len(all.Stats) != 4 || len(all.Answers) != 2 || !slices.Equal(all.Models, []string{"a", "b"}) {
		t.Fatalf("no pick keeps every model over machines: %+v", all)
	}
	picked := baseOf(cells, answers, []string{"a"})
	if len(picked.Stats) != 2 || len(picked.Answers) != 1 || !slices.Equal(picked.Models, []string{"a", "b"}) {
		t.Fatalf("a pick keeps its figures, the menu keeps every model: %+v", picked)
	}
	if speed := picked.Verdicts[0].Speed; speed == nil || speed.Model != "a" {
		t.Fatalf("verdicts must be drawn among the picked models only: %+v", picked.Verdicts)
	}
}
