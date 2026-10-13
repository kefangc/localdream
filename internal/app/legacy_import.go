package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
	"time"
)

func (s *Server) importLegacy(base string) error {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		return errors.New("legacy import must use a local HTTP server")
	}
	if len(s.assets) != 0 || len(s.jobs) != 0 {
		return errors.New("数据库已有数据，不覆盖；旧服务迁移只能对空库执行")
	}
	get := func(path string, out any) error {
		resp, err := s.httpClient.Get(strings.TrimRight(base, "/") + path)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("legacy %s: %s", path, resp.Status)
		}
		return json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out)
	}
	var assets []Asset
	var jobs []Generation
	if err := get("/api/assets", &assets); err != nil {
		return err
	}
	if err := get("/api/generations", &jobs); err != nil {
		return err
	}
	// Archive exactly what the legacy API reported, without claiming its graph used these assets.
	original, err := json.Marshal(map[string]any{"assets": assets, "generations": jobs, "captured_at": time.Now()})
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, a := range assets {
		if !strings.HasPrefix(a.URL, "/media/") {
			return fmt.Errorf("invalid legacy asset URL: %s", a.ID)
		}
		a.Filename = strings.TrimPrefix(a.URL, "/media/")
		if _, err := s.assetPath(a); err != nil {
			return err
		}
		raw, err := json.Marshal(a)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO assets VALUES (?,?)", a.ID, string(raw)); err != nil {
			return err
		}
	}
	for _, g := range jobs {
		g.Phase = "legacy" // Seed/prompt_id are unknown, do not invent them.
		raw, err := json.Marshal(g)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO generations VALUES (?,?)", g.ID, string(raw)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("INSERT INTO legacy_imports VALUES (?,?)", base, string(original)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	log.Printf("已迁移 %d 个素材、%d 条生成记录；旧文件保留，未推测旧任务的实际输入", len(assets), len(jobs))
	return nil
}
