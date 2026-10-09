package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Server) assetsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.RLock()
		defer s.mu.RUnlock()
		writeJSON(w, 200, s.assets)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<20)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, head, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()
	id, err := uniqueID("asset")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	ext := strings.ToLower(filepath.Ext(head.Filename))
	kind := assetKind(head.Filename)
	if kind == "" {
		writeJSON(w, 400, map[string]string{"error": "不支持的素材格式"})
		return
	}
	name := id + ext
	dstPath := filepath.Join(s.root, "data", "assets", name)
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	size, err := io.Copy(dst, file)
	if syncErr := dst.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := dst.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(dstPath)
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	a := Asset{ID: id, ProjectID: strings.TrimSpace(r.FormValue("project_id")), Filename: name, Name: head.Filename, Type: kind, URL: "/media/" + name, Size: size, CreatedAt: time.Now()}
	s.mu.Lock()
	if !s.validProjectLocked(a.ProjectID) {
		s.mu.Unlock()
		_ = os.Remove(dstPath)
		writeJSON(w, 400, map[string]string{"error": "项目不存在或已归档"})
		return
	}
	if err := s.saveRecord("assets", id, a); err != nil {
		s.mu.Unlock()
		_ = os.Remove(dstPath)
		writeJSON(w, 500, map[string]string{"error": "素材保存失败: " + err.Error()})
		return
	}
	s.assets = append([]Asset{a}, s.assets...)
	s.mu.Unlock()
	writeJSON(w, 201, a)
}

func (s *Server) deleteAssetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && !(r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/project")) {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/assets/")
	if strings.HasSuffix(id, "/project") && r.Method == http.MethodPost {
		id = strings.TrimSuffix(id, "/project")
		var body struct {
			ProjectID string `json:"project_id"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			writeJSON(w, 400, nil)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, a := range s.assets {
			if a.ID == id {
				a.ProjectID = strings.TrimSpace(body.ProjectID)
				if !s.validProjectLocked(a.ProjectID) {
					writeJSON(w, 400, map[string]string{"error": "项目不存在或已归档"})
					return
				}
				if err := s.saveRecord("assets", id, a); err != nil {
					writeJSON(w, 500, map[string]string{"error": err.Error()})
					return
				}
				s.assets[i] = a
				writeJSON(w, 200, a)
				return
			}
		}
		writeJSON(w, 404, nil)
		return
	}
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "asset id is required"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, asset := range s.assets {
		if asset.ID != id {
			continue
		}
		// Remove from the library, but preserve the file and historical snapshots.
		// This also avoids a deletion racing an already queued generation.
		if s.db == nil {
			writeJSON(w, 500, map[string]string{"error": "database unavailable"})
			return
		}
		if _, err := s.db.Exec("DELETE FROM assets WHERE id=?", id); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		s.assets = append(s.assets[:i], s.assets[i+1:]...)
		writeJSON(w, http.StatusNoContent, nil)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "asset not found"})
}
