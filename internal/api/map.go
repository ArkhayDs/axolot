package api

import (
	"net/http"

	"github.com/arkhayds/axolot/internal/analytics"
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
