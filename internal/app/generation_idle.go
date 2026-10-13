package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// Dedicated production servers may require an empty shared ComfyUI queue.
// This never interrupts or cancels work belonging to another Local Dream server.
func (s *Server) requireIdleComfy() error {
	if os.Getenv("LOCALDREAM_REQUIRE_IDLE_COMFY") != "1" {
		return nil
	}
	var queue struct {
		Running []json.RawMessage `json:"queue_running"`
		Pending []json.RawMessage `json:"queue_pending"`
	}
	if err := s.comfyJSON(http.MethodGet, "/queue", nil, &queue); err != nil {
		return fmt.Errorf("cannot verify shared ComfyUI queue: %w", err)
	}
	if len(queue.Running) != 0 || len(queue.Pending) != 0 {
		return fmt.Errorf("shared ComfyUI is busy (%d running, %d pending); no inference was submitted", len(queue.Running), len(queue.Pending))
	}
	return nil
}
