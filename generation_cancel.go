package main

import "net/http"

// cancelGeneration is shared by HTTP and MCP. Serializing with submission
// prevents a locally cancelled task from being submitted after cancellation.
func (s *Server) cancelGeneration(id string) (Generation, int, error) {
	s.submitMu.Lock()
	defer s.submitMu.Unlock()
	g, ok := s.job(id)
	if !ok || g.Deleted {
		return Generation{}, http.StatusNotFound, toolProblem("generation_not_found", "任务不存在", false)
	}
	if g.Status == "cancelled" {
		return g, http.StatusOK, nil
	}
	if !activeGeneration(g) {
		return Generation{}, http.StatusConflict, toolProblem("generation_not_active", "只能取消排队中或生成中的任务", false)
	}
	// A created task has never been submitted. Do not require a reachable ComfyUI.
	if g.Phase != "created" {
		if err := s.cancelComfyJob(g); err != nil {
			return Generation{}, http.StatusBadGateway, err
		}
	}
	if err := s.updateJob(id, "cancelled", 0, ""); err != nil {
		return Generation{}, http.StatusInternalServerError, err
	}
	g, _ = s.job(id)
	return g, http.StatusOK, nil
}
