package api

import "net/http"

// getMap returns what the landing map draws and ranks.
func (s *Server) getMap(w http.ResponseWriter, r *http.Request) {
	cells, err := s.cfg.Store.MapCells(r.Context())
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
