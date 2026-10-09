package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) importWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, nil)
		return
	}
	var in struct {
		Workflow json.RawMessage `json:"workflow"`
		Name     string          `json:"name"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid workflow"})
		return
	}
	var doc struct {
		Nodes []struct {
			ID      int             `json:"id"`
			Type    string          `json:"type"`
			Title   string          `json:"title"`
			Widgets json.RawMessage `json:"widgets_values"`
		} `json:"nodes"`
	}
	workflowBytes := in.Workflow
	if len(workflowBytes) > 0 && workflowBytes[0] == '"' {
		var encoded string
		if json.Unmarshal(workflowBytes, &encoded) == nil {
			workflowBytes = []byte(encoded)
		}
	}
	if json.Unmarshal(workflowBytes, &doc) != nil {
		writeJSON(w, 400, map[string]string{"error": "workflow must be API JSON or exported JSON"})
		return
	}
	candidates := []map[string]any{}
	for _, n := range doc.Nodes {
		t := strings.ToLower(n.Type + " " + n.Title)
		role := ""
		if strings.Contains(t, "text") || strings.Contains(t, "prompt") || strings.Contains(t, "clip") {
			role = "prompt"
		}
		if strings.Contains(t, "loadimage") || strings.Contains(t, "image") {
			role = "image"
		}
		if strings.Contains(t, "video") {
			role = "video"
		}
		if strings.Contains(t, "audio") {
			role = "audio"
		}
		if strings.Contains(t, "duration") || strings.Contains(t, "length") {
			role = "duration"
		}
		if role != "" {
			candidates = append(candidates, map[string]any{"node_id": n.ID, "node_type": n.Type, "role": role, "confidence": 0.55})
		}
	}
	writeJSON(w, 200, map[string]any{"name": in.Name, "candidates": candidates, "warnings": []string{"自动推断结果需要人工确认后才能执行"}})
}
