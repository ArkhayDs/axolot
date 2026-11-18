package api

import (
	"net/http"
	"slices"

	"github.com/arkhayds/axolot/internal/analytics"
	"github.com/arkhayds/axolot/internal/runner"
	"github.com/arkhayds/axolot/internal/store"
)

// connectionOf reads the connection type a view keeps, empty for all; an unknown one is refused with a 400.
func connectionOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	connection := r.URL.Query().Get("connection")
	if !slices.Contains([]string{"", store.ConnectionLocal, store.ConnectionAPI, store.ConnectionSubscription}, connection) {
		writeError(w, http.StatusBadRequest, "Type de connexion inconnu")
		return "", false
	}
	return connection, true
}

// getMap returns what the landing map draws and ranks, of one connection type when asked.
func (s *Server) getMap(w http.ResponseWriter, r *http.Request) {
	connection, ok := connectionOf(w, r)
	if !ok {
		return
	}
	hashes := s.currentHashes()
	cells, err := s.cfg.Store.MapCells(r.Context(), hashes, connection)
	if err != nil {
		internalError(w, r, err)
		return
	}
	items, err := s.cfg.Store.MapItems(r.Context(), hashes, connection)
	if err != nil {
		internalError(w, r, err)
		return
	}
	benchmarks, err := s.cfg.Store.MapBenchmarks(r.Context(), connection)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cells": cells, "items": items, "benchmarks": benchmarks, "current_machine": s.cfg.Hardware.Label})
}

// baseView is the cumulated database in the shape of a campaign's results, plus every model the picking offers.
type baseView struct {
	Stats    []store.MapCell     `json:"stats"`
	Verdicts []analytics.Verdict `json:"verdicts"`
	Answers  []store.Answer      `json:"answers"`
	Models   []string            `json:"models"`
}

// getBase returns the cumulated database, all machines together, of one connection type and some models when asked.
func (s *Server) getBase(w http.ResponseWriter, r *http.Request) {
	connection, ok := connectionOf(w, r)
	if !ok {
		return
	}
	hashes := s.currentHashes()
	cells, err := s.cfg.Store.MapCells(r.Context(), hashes, connection)
	if err != nil {
		internalError(w, r, err)
		return
	}
	answers, err := s.cfg.Store.MapAnswers(r.Context(), hashes, connection)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, baseOf(cells, answers, r.URL.Query()["model"]))
}

// baseOf keeps the rollups over machines of the picked models, all when none is. Filtering after the query holds
// because every figure is per model; the verdicts are drawn again among the models kept.
func baseOf(cells []store.MapCell, answers []store.Answer, picked []string) baseView {
	kept := func(model string) bool { return len(picked) == 0 || slices.Contains(picked, model) }
	view := baseView{Stats: []store.MapCell{}, Models: []string{}}
	var stats []analytics.ModelStats
	for _, cell := range cells {
		if cell.Machine != "" {
			continue
		}
		if cell.Category == "" {
			view.Models = append(view.Models, cell.Model)
		}
		if kept(cell.Model) {
			view.Stats = append(view.Stats, cell)
			stats = append(stats, cell.ModelStats)
		}
	}
	view.Verdicts = analytics.Verdicts(stats)
	view.Answers = slices.DeleteFunc(answers, func(answer store.Answer) bool { return !kept(answer.Label) })
	return view
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
			label := store.ModelLabel(target.Model, target.ConnectionType, target.ProviderName)
			for _, item := range plan.Case.Items {
				if done[key{label, "", plan.Case.ID, item.ID}] {
					continue
				}
				estimate.Answers++
				row, ok := known[key{label, plan.Case.Category, plan.Case.ID, item.ID}]
				if !ok {
					row, ok = known[key{label, plan.Case.Category, "", ""}]
					estimate.Partial = true
				}
				if !ok {
					estimate.Known = false
					continue
				}
				estimate.LowMs += row.AvgMs
				estimate.HighMs += row.P90Ms
				if !started[label] && !loaded[label] {
					loaded[label] = true
					if load := known[key{label, "", "", ""}].LoadMs; load != nil {
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
		Targets []struct {
			ProviderID int64  `json:"provider_id"`
			Model      string `json:"model"`
		} `json:"targets"`
		Cases []string `json:"cases"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	byID, err := s.providersByID(r.Context())
	if err != nil {
		internalError(w, r, err)
		return
	}
	var targets []runner.Target
	for _, t := range body.Targets {
		if p, ok := byID[t.ProviderID]; ok {
			targets = append(targets, targetOf(p, t.Model))
		}
	}
	var plans []runner.Plan
	for _, id := range body.Cases {
		if c, ok := s.findCase(id); ok {
			plans = append(plans, runner.Plan{Case: c, Targets: targets})
		}
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
