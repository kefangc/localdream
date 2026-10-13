package app

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) projectsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.RLock()
		defer s.mu.RUnlock()
		projects := []Project{}
		for _, p := range s.projects {
			if !p.Deleted {
				projects = append(projects, p)
			}
		}
		writeJSON(w, 200, projects)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, nil)
		return
	}
	var in Project
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" {
		writeJSON(w, 400, map[string]string{"error": "项目名称不能为空"})
		return
	}
	id, err := uniqueID("project")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	now := time.Now()
	p := Project{ID: id, Name: strings.TrimSpace(in.Name), Description: strings.TrimSpace(in.Description), CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err = s.saveRecord("projects", id, p); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.projects = append([]Project{p}, s.projects...)
	writeJSON(w, 201, p)
}

func (s *Server) projectHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/projects/"), "/"), "/")
	id := parts[0]
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, p := range s.projects {
		if p.ID == id && !p.Deleted {
			idx = i
			break
		}
	}
	if idx < 0 {
		writeJSON(w, 404, map[string]string{"error": "项目不存在"})
		return
	}
	if r.Method == http.MethodDelete && len(parts) == 1 || r.Method == http.MethodPost && len(parts) == 2 && (parts[1] == "archive" || parts[1] == "restore") {
		p := s.projects[idx]
		if r.Method == http.MethodDelete {
			p.Deleted = true
		} else {
			p.Archived = parts[1] == "archive"
		}
		p.UpdatedAt = time.Now()
		if err := s.saveRecord("projects", id, p); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		s.projects[idx] = p
		writeJSON(w, 200, p)
		return
	}
	if r.Method == http.MethodGet {
		assets := []Asset{}
		for _, a := range s.assets {
			if a.ProjectID == id {
				assets = append(assets, a)
			}
		}
		writeJSON(w, 200, map[string]any{"project": s.projects[idx], "assets": assets})
		return
	}
	if len(parts) == 3 && parts[1] == "assets" && r.Method == http.MethodPost {
		if s.projects[idx].Archived {
			writeJSON(w, 400, map[string]string{"error": "请先恢复已归档项目"})
			return
		}
		aid := parts[2]
		found := false
		for i := range s.assets {
			if s.assets[i].ID == aid {
				a := s.assets[i]
				a.ProjectID = id
				found = true
				if err := s.saveRecord("assets", aid, a); err != nil {
					writeJSON(w, 500, map[string]string{"error": err.Error()})
					return
				}
				s.assets[i] = a
				break
			}
		}
		if !found {
			writeJSON(w, 404, map[string]string{"error": "素材不存在"})
			return
		}
		writeJSON(w, 200, s.assets)
		return
	}
	if r.Method == http.MethodPut {
		var in Project
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			writeJSON(w, 400, nil)
			return
		}
		p := s.projects[idx]
		if strings.TrimSpace(in.Name) != "" {
			p.Name = strings.TrimSpace(in.Name)
		}
		p.Description = in.Description
		p.UpdatedAt = time.Now()
		if err := s.saveRecord("projects", id, p); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		s.projects[idx] = p
		writeJSON(w, 200, p)
		return
	}
	writeJSON(w, 405, nil)
}

// An empty id explicitly means a personal, unassigned creation.
func (s *Server) validProjectLocked(id string) bool {
	if id == "" {
		return true
	}
	for _, p := range s.projects {
		if p.ID == id && !p.Deleted && !p.Archived {
			return true
		}
	}
	return false
}
