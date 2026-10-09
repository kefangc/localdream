package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) generationHandler(w http.ResponseWriter, r *http.Request) {
	pathID := strings.TrimPrefix(r.URL.Path, "/api/generations/")
	if strings.HasSuffix(pathID, "/video") {
		s.generationVideo(w, r, strings.TrimSuffix(pathID, "/video"))
		return
	}
	isCancel := r.Method == http.MethodPost && strings.HasSuffix(pathID, "/cancel")
	isArchive := r.Method == http.MethodPost && strings.HasSuffix(pathID, "/archive")
	isRestore := r.Method == http.MethodPost && strings.HasSuffix(pathID, "/restore")
	isMove := r.Method == http.MethodPost && strings.HasSuffix(pathID, "/move")
	isDelete := r.Method == http.MethodDelete
	id := strings.TrimSuffix(pathID, "/cancel")
	id = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(id, "/archive"), "/restore"), "/move")
	if id == "" {
		writeJSON(w, 404, nil)
		return
	}
	if r.Method == http.MethodGet {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, j := range s.jobs {
			if j.ID == id {
				writeJSON(w, 200, publicGeneration(j))
				return
			}
		}
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	if isCancel {
		g, status, err := s.cancelGeneration(id)
		if err != nil {
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]string{"status": g.Status})
		return
	}
	if isArchive || isRestore || isMove || isDelete {
		s.mu.Lock()
		defer s.mu.Unlock()
		idx := -1
		for i, g := range s.jobs {
			if g.ID == id {
				idx = i
				break
			}
		}
		if idx < 0 {
			writeJSON(w, 404, nil)
			return
		}
		g := s.jobs[idx]
		if isDelete {
			if activeGeneration(g) {
				writeJSON(w, 409, map[string]string{"error": "运行中的任务不能删除"})
				return
			}
			g.Deleted = true
		} else if isArchive {
			g.Archived = true
		} else if isRestore {
			g.Archived = false
		} else {
			var body struct {
				ProjectID string `json:"project_id"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				writeJSON(w, 400, nil)
				return
			}
			g.ProjectID = body.ProjectID
			if !s.validProjectLocked(g.ProjectID) {
				writeJSON(w, 400, map[string]string{"error": "项目不存在或已归档"})
				return
			}
		}
		g.UpdatedAt = time.Now()
		if err := s.saveRecord("generations", id, g); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		s.jobs[idx] = g
		writeJSON(w, 200, publicGeneration(g))
		return
	}
	writeJSON(w, 405, nil)
}
