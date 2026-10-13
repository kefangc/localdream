package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	_ "modernc.org/sqlite"
	"path/filepath"
)

// The database stores versioned JSON records inside transactional SQLite tables.
func (s *Server) openStore() error {
	path := env("LOCALDREAM_DB", filepath.Join(s.root, "data", "localdream.sqlite"))
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	s.db = db
	_, err = db.Exec(`PRAGMA busy_timeout=5000;
PRAGMA journal_mode=WAL;
PRAGMA synchronous=FULL;
CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS assets (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS projects (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS generations (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS long_videos (id TEXT PRIMARY KEY, payload TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS generation_drafts (id TEXT PRIMARY KEY, fingerprint TEXT NOT NULL, payload TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS legacy_imports (source TEXT PRIMARY KEY, payload TEXT NOT NULL);
INSERT OR IGNORE INTO metadata VALUES ('schema_version','1');`)
	if err != nil {
		db.Close()
		return err
	}
	var version string
	if err = db.QueryRow("SELECT value FROM metadata WHERE key='schema_version'").Scan(&version); err != nil || version != "1" {
		db.Close()
		return fmt.Errorf("unsupported database schema %q: %v", version, err)
	}
	load := func(table string, consume func([]byte) error) error {
		rows, err := db.Query("SELECT payload FROM " + table + " ORDER BY json_extract(payload,'$.created_at') DESC")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			if err := consume(raw); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	err = load("assets", func(raw []byte) error {
		var a Asset
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		s.assets = append(s.assets, a)
		return nil
	})
	if err == nil {
		err = load("projects", func(raw []byte) error {
			var p Project
			if err := json.Unmarshal(raw, &p); err != nil {
				return err
			}
			s.projects = append(s.projects, p)
			return nil
		})
	}
	if err == nil {
		err = load("generations", func(raw []byte) error {
			var g Generation
			if err := json.Unmarshal(raw, &g); err != nil {
				return err
			}
			s.jobs = append(s.jobs, g)
			return nil
		})
	}
	if err != nil {
		db.Close()
		return err
	}
	log.Printf("SQLite: %s (%d assets, %d jobs)", path, len(s.assets), len(s.jobs))
	return s.loadLongVideos()
}

func (s *Server) saveRecord(table, id string, record any) error {
	if s.db == nil {
		return errors.New("database is not initialized")
	}
	if table != "assets" && table != "projects" && table != "generations" && table != "long_videos" {
		return errors.New("invalid record table")
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO "+table+" (id,payload) VALUES (?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", id, string(raw))
	return err
}
