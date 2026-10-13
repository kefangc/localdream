package app

import (
	"encoding/json"
	"errors"
	"log"
	"net/url"
	"time"
)

func (s *Server) job(id string) (Generation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, g := range s.jobs {
		if g.ID == id {
			return g, true
		}
	}
	return Generation{}, false
}

func (s *Server) bindPrompt(id, promptID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.jobs {
		if g.ID != id {
			continue
		}
		g.PromptID = promptID
		g.Phase = "submitted"
		g.UpdatedAt = time.Now()
		if err := s.saveRecord("generations", id, g); err != nil {
			return err
		}
		s.jobs[i] = g
		return nil
	}
	return errors.New("job not found")
}

func (s *Server) launchWatch(id string) {
	s.startProgress(id)
	s.workers.Add(1)
	go func() { defer s.workers.Done(); s.watchJob(id) }()
}

func (s *Server) recoverJobs() {
	s.mu.RLock()
	jobs := append([]Generation(nil), s.jobs...)
	s.mu.RUnlock()
	for _, g := range jobs {
		if g.Status != "queued" && g.Status != "running" {
			continue
		}
		if g.Phase == "created" && g.Seed != nil {
			in := requestFromJob(g)
			s.workers.Add(1)
			go func(id string, in generationRequest) { defer s.workers.Done(); s.runJob(id, in) }(g.ID, in)
		} else {
			s.launchWatch(g.ID)
		}
	}
}

// Locate tasks by durable prompt_id, or by the unique client_id written before POST.
func (s *Server) remoteJob(g Generation) (promptID, status string, err error) {
	var queue struct {
		Running [][]json.RawMessage `json:"queue_running"`
		Pending [][]json.RawMessage `json:"queue_pending"`
	}
	if err = s.comfyJSON("GET", "/queue", nil, &queue); err != nil {
		return
	}
	find := func(items [][]json.RawMessage) string {
		for _, item := range items {
			if len(item) < 4 {
				continue
			}
			var id string
			var extra struct {
				ClientID string `json:"client_id"`
			}
			if json.Unmarshal(item[1], &id) != nil || json.Unmarshal(item[3], &extra) != nil {
				continue
			}
			if (g.PromptID != "" && id == g.PromptID) || (g.ClientID != "" && extra.ClientID == g.ClientID) {
				return id
			}
		}
		return ""
	}
	if id := find(queue.Running); id != "" {
		return id, "running", nil
	}
	if id := find(queue.Pending); id != "" {
		return id, "queued", nil
	}
	var history map[string]struct {
		Prompt []json.RawMessage `json:"prompt"`
	}
	path := "/history"
	if g.PromptID != "" {
		path += "/" + url.PathEscape(g.PromptID)
	}
	if err = s.comfyJSON("GET", path, nil, &history); err != nil {
		return
	}
	for id, h := range history {
		if g.PromptID != "" && id == g.PromptID {
			return id, "history", nil
		}
		if len(h.Prompt) < 4 || g.ClientID == "" {
			continue
		}
		var extra struct {
			ClientID string `json:"client_id"`
		}
		if json.Unmarshal(h.Prompt[3], &extra) == nil && extra.ClientID == g.ClientID {
			return id, "history", nil
		}
	}
	return "", "missing", nil
}

func (s *Server) watchJob(id string) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	missing := 0
	for {
		if s.ctx.Err() != nil {
			return
		}
		g, ok := s.job(id)
		if !ok || (g.Status != "running" && g.Status != "queued") {
			return
		}
		promptID, status, err := s.remoteJob(g)
		if err == nil {
			if status == "missing" {
				missing++
				if missing >= 3 {
					if err := s.updateJob(id, "failed", g.Progress, "未在 ComfyUI 队列或历史中找到原任务；提交结果未知或后端历史已丢失。为避免重复生成，未自动重提，请核对输出后再决定。"); err == nil {
						return
					}
				}
			} else {
				missing = 0
				if g.PromptID != promptID {
					if err := s.bindPrompt(id, promptID); err != nil {
						log.Printf("恢复 prompt_id 失败: %v", err)
						return
					}
				}
				if status == "history" {
					state, done, failed, output, err := s.comfyResult(promptID)
					if err == nil && failed {
						s.captureHistoryEnd(id, promptID)
						if err := s.updateJob(id, "failed", g.Progress, "ComfyUI 生成失败: "+state); err == nil {
							return
						}
					}
					if err == nil && done {
						s.captureHistoryEnd(id, promptID)
						if err := s.completeJob(id, output); err == nil {
							return
						}
					}
				} else if g.Status != status {
					s.updateJob(id, status, g.Progress, "")
				}
			}
		}
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) completeJob(id, outputURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			g := s.jobs[i]
			if g.Status == "cancelled" {
				return nil
			}
			g.Status = "completed"
			g.Progress = 100
			g.OutputURL = outputURL
			g.UpdatedAt = time.Now()
			g.Phase = "finished"
			finishTiming(&g)
			g.ProgressConnected = false
			if err := s.saveRecord("generations", id, g); err != nil {
				log.Printf("保存完成状态失败: %v", err)
				return err
			}
			s.jobs[i] = g
			s.pruneSequenceLatents(g.Continuity)
			return nil
		}
	}
	return errors.New("job not found")
}

func (s *Server) jobCancelled(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, job := range s.jobs {
		if job.ID == id {
			return job.Status == "cancelled"
		}
	}
	return false
}

func (s *Server) updateJob(id, status string, progress int, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			g := s.jobs[i]
			if g.Status == "cancelled" || g.Status == "completed" || g.Status == "failed" {
				return nil
			}
			g.Status = status
			if status == "running" {
				markStarted(&g, 0)
			}
			// Queue/history polling must not overwrite a newer WebSocket percentage.
			if status != "running" && status != "queued" {
				g.Progress = progress
			}
			g.Error = errMsg
			g.UpdatedAt = time.Now()
			if status == "failed" || status == "cancelled" {
				finishTiming(&g)
				g.ProgressConnected = false
			}
			if err := s.saveRecord("generations", id, g); err != nil {
				log.Printf("保存任务状态失败: %v", err)
				return err
			}
			s.jobs[i] = g
			return nil
		}
	}
	return errors.New("job not found")
}

// Keep the internal ComfyUI URL in storage; expose only a same-origin URL.
// This also fixes existing records without rewriting the database.
func publicGeneration(g Generation) Generation {
	g.ReferenceVideoFPS = legacyReferenceVideoFPS(g.ReferenceVideoFPS)
	if g.OutputURL != "" {
		g.OutputURL = "/api/generations/" + url.PathEscape(g.ID) + "/video"
	}
	return g
}
