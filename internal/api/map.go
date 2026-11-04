package api

import (
	"net/http"

	"github.com/arkhayds/axolot/internal/analytics"
	"github.com/arkhayds/axolot/internal/runner"
	"github.com/arkhayds/axolot/internal/store"
)

// getMap returns what the landing map draws and ranks.
func (s *Server) getMap(w http.ResponseWriter, r *http.Request) {
	cells, err := s.cfg.Store.MapCells(r.Context(), s.currentHashes())
	if err != nil {
		internalError(w, r, err)
		return
	}
	benchmarks, err := s.cfg.Store.MapBenchmarks(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cells": cells, "benchmarks": benchmarks, "current_machine": s.cfg.Hardware.Label})
}

// getBase returns the cumulated database in the shape of a campaign's results: all machines together.
func (s *Server) getBase(w http.ResponseWriter, r *http.Request) {
	hashes := s.currentHashes()
	cells, err := s.cfg.Store.MapCells(r.Context(), hashes)
	if err != nil {
		internalError(w, r, err)
		return
	}
	answers, err := s.cfg.Store.MapAnswers(r.Context(), hashes)
	if err != nil {
		internalError(w, r, err)
		return
	}
	rollups := make([]store.MapCell, 0, len(cells))
	stats := make([]analytics.ModelStats, 0, len(cells))
	for _, cell := range cells {
		if cell.Machine == "" {
			rollups = append(rollups, cell)
			stats = append(stats, cell.ModelStats)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"stats": rollups, "verdicts": analytics.Verdicts(stats), "answers": answers})
}

// currentHashes lists the versions of the questions as loaded, the only answers the cumulated figures count.
func (s *Server) currentHashes() []string {
	var hashes []string
	for _, c := range s.cfg.Cases {
		for _, item := range c.Items {
			hashes = append(hashes, item.Hash)
		}
	}
	return hashes
}

// Estimate is a duration range; Known is false as soon as one answer has nothing to be estimated from.
type Estimate struct {
	Answers int     `json:"answers"`
	LowMs   float64 `json:"low_ms"`
	HighMs  float64 `json:"high_ms"`
	Partial bool    `json:"partial"`
	Known   bool    `json:"known"`
}

// computeEstimate sums what is left to answer: the mean for the low bound, p90 for the high one, the model's
// mean on the same theme when a question was never played, and each load once for models yet to start.
func computeEstimate(rows []store.EstimateRow, plans []runner.Plan, played []store.PlayedItem) Estimate {
	type key struct{ model, category, caseID, itemID string }
	known := make(map[key]store.EstimateRow, len(rows))
	for _, row := range rows {
		known[key{row.Model, row.Category, row.CaseID, row.ItemID}] = row
	}
	done := make(map[key]bool, len(played))
	started := map[string]bool{}
	for _, item := range played {
		done[key{item.Model, "", item.CaseID, item.ItemID}] = true
		started[item.Model] = true
	}
	estimate := Estimate{Known: true}
	loaded := map[string]bool{}
	for _, plan := range plans {
		for _, target := range plan.Targets {
			for _, item := range plan.Case.Items {
				if done[key{target.Model, "", plan.Case.ID, item.ID}] {
					continue
				}
				estimate.Answers++
				row, ok := known[key{target.Model, plan.Case.Category, plan.Case.ID, item.ID}]
				if !ok {
					row, ok = known[key{target.Model, plan.Case.Category, "", ""}]
					estimate.Partial = true
				}
				if !ok {
					estimate.Known = false
					continue
				}
				estimate.LowMs += row.AvgMs
				estimate.HighMs += row.P90Ms
				if !started[target.Model] && !loaded[target.Model] {
					loaded[target.Model] = true
					if load := known[key{target.Model, "", "", ""}].LoadMs; load != nil {
						estimate.LowMs += *load
						estimate.HighMs += *load
					}
				}
			}
		}
	}
	return estimate
}

// postEstimate estimates a benchmark of the given models and scenarios on this machine.
func (s *Server) postEstimate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Models []string `json:"models"`
		Cases  []string `json:"cases"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	var plans []runner.Plan
	for _, id := range body.Cases {
		c, ok := s.findCase(id)
		if !ok {
			continue
		}
		plan := runner.Plan{Case: c}
		for _, model := range body.Models {
			plan.Targets = append(plan.Targets, runner.Target{Model: model})
		}
		plans = append(plans, plan)
	}
	rows, err := s.cfg.Store.EstimateRows(r.Context(), s.cfg.Hardware.Label, s.currentHashes())
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, computeEstimate(rows, plans, nil))
}

// getRemaining estimates what the running benchmark still has to answer.
func (s *Server) getRemaining(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	id, plans := s.active, s.activePlans
	s.mu.Unlock()
	if id == "" {
		writeJSON(w, http.StatusOK, Estimate{})
		return
	}
	rows, err := s.cfg.Store.EstimateRows(r.Context(), s.cfg.Hardware.Label, s.currentHashes())
	if err != nil {
		internalError(w, r, err)
		return
	}
	played, err := s.cfg.Store.PlayedItems(r.Context(), id)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, computeEstimate(rows, plans, played))
}
