package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
)

func (s *Server) loadLongVideos() error {
	rows, err := s.db.Query("SELECT payload FROM long_videos ORDER BY json_extract(payload,'$.created_at') DESC")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var v LongVideo
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return err
		}
		for i := range v.Segments {
			v.Segments[i].Request.ReferenceVideoFPS = legacyReferenceVideoFPS(v.Segments[i].Request.ReferenceVideoFPS)
		}
		s.longVideos = append(s.longVideos, v)
	}
	return rows.Err()
}

func (s *Server) longVideoSettings() (LongVideoSettings, error) {
	c := defaultLongVideoSettings()
	var raw []byte
	err := s.db.QueryRow("SELECT value FROM metadata WHERE key='long_video_settings'").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(raw, &c)
	return c, err
}

func (s *Server) longVideoSettingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		c, err := s.longVideoSettings()
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, c)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, nil)
		return
	}
	var c LongVideoSettings
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeJSON(w, 400, map[string]string{"error": "无效设置"})
		return
	}
	if err := c.validate(); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	raw, err := json.Marshal(c)
	if err == nil {
		_, err = s.db.Exec("INSERT INTO metadata(key,value) VALUES('long_video_settings',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", string(raw))
	}
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, c)
}
