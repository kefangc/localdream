package main

import (
	"encoding/json"
	"errors"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) runJob(id string, in generationRequest) {
	// Serialize preparation/submission; ComfyUI owns execution queueing.
	s.submitMu.Lock()
	defer s.submitMu.Unlock()
	if s.ctx.Err() != nil || s.jobCancelled(id) {
		return
	}
	g, ok := s.job(id)
	if !ok {
		return
	}
	if g.Phase != "created" {
		s.launchWatch(id)
		return
	}
	if g.Seed == nil {
		s.updateJob(id, "failed", 0, "任务缺少已保存的 seed，未自动提交")
		return
	}
	if err := s.requireIdleComfy(); err != nil {
		s.updateJob(id, "failed", 0, err.Error())
		return
	}
	// Compile using the immutable asset snapshot, even if the user subsequently
	// removes an asset from the library. Files are retained on library deletion.
	compiler := &Server{root: s.root, comfyURL: s.comfyURL, comfyInput: s.comfyInput, httpClient: s.httpClient, assets: g.AssetSnapshot}
	compiled, err := compiler.compilePrompt(in)
	if err != nil {
		s.updateJob(id, "failed", 0, "工作流编译失败："+err.Error())
		return
	}
	noiseNodes := 0
	// Preserve the compiler and template: override only the submitted resolution
	// nodes when the saved request explicitly selects a custom megapixel value.
	if strings.HasSuffix(in.Resolution, "MP") {
		mp, err := strconv.ParseFloat(strings.TrimSuffix(in.Resolution, "MP"), 64)
		if err != nil || math.IsNaN(mp) || math.IsInf(mp, 0) || mp < 0.1 || mp > 1.9 {
			s.updateJob(id, "failed", 0, "自定义分辨率无效")
			return
		}
		resolutionNodes := 0
		for nodeID, node := range compiled.Prompt {
			if node.ClassType == "ResolutionSelector" {
				if node.Inputs == nil {
					node.Inputs = map[string]any{}
				}
				node.Inputs["megapixels"] = mp
				compiled.Prompt[nodeID] = node
				resolutionNodes++
			}
		}
		if resolutionNodes == 0 {
			s.updateJob(id, "failed", 0, "工作流没有分辨率节点，无法应用自定义分辨率")
			return
		}
	}
	for nodeID, node := range compiled.Prompt {
		if node.ClassType == "RandomNoise" {
			if node.Inputs == nil {
				node.Inputs = map[string]any{}
			}
			node.Inputs["noise_seed"] = *g.Seed
			compiled.Prompt[nodeID] = node
			noiseNodes++
		}
	}
	if noiseNodes == 0 {
		s.updateJob(id, "failed", 0, "工作流没有 RandomNoise，无法应用任务 seed")
		return
	}
	compiled.ClientID = g.ClientID
	if err := s.requireIdleComfy(); err != nil {
		s.updateJob(id, "failed", 0, err.Error())
		return
	}
	// Write the intent before POST. A crash after this point must reconcile, never resubmit.
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	for i := range s.jobs {
		if s.jobs[i].ID != id {
			continue
		}
		if s.jobs[i].Status == "cancelled" {
			s.mu.Unlock()
			return
		}
		g = s.jobs[i]
		g.NodeTypes = make(map[string]string, len(compiled.Prompt))
		for nodeID, node := range compiled.Prompt {
			g.NodeTypes[nodeID] = node.ClassType
		}
		g.Phase = "submitting"
		g.UpdatedAt = time.Now()
		// Atomic claim: a second process/recovery worker must not submit this row again.
		raw, err := json.Marshal(g)
		if err != nil {
			s.mu.Unlock()
			log.Printf("任务编码失败: %v", err)
			return
		}
		result, err := s.db.Exec("UPDATE generations SET payload=? WHERE id=? AND json_extract(payload,'$.phase')='created'", string(raw), id)
		if err != nil {
			s.mu.Unlock()
			log.Printf("任务未提交，持久化失败: %v", err)
			return
		}
		claimed, err := result.RowsAffected()
		if err != nil || claimed != 1 {
			s.mu.Unlock()
			s.launchWatch(id)
			return
		}
		s.jobs[i] = g
	}
	s.mu.Unlock()
	// Establish the matching client socket BEFORE POST, including fast/cache hits.
	s.startProgress(id)
	var out struct {
		PromptID string `json:"prompt_id"`
	}
	if err := s.comfyJSON(http.MethodPost, "/prompt", compiled, &out); err != nil {
		var responseError *comfyHTTPError
		if errors.As(err, &responseError) && responseError.Status == http.StatusBadRequest {
			// ComfyUI validation rejected the graph before queueing it. Preserve
			// the real node error and permit an explicit retry after correction.
			s.rejectSubmission(id, err.Error())
			return
		}
		log.Printf("%s 提交响应异常，核对队列而非重试: %v", id, err)
		s.launchWatch(id)
		return
	}
	if out.PromptID == "" {
		s.launchWatch(id)
		return
	}
	if err := s.bindPrompt(id, out.PromptID); err != nil {
		log.Printf("prompt_id 保存失败（可通过 client_id 恢复）: %v", err)
	}
	log.Printf("job=%s prompt=%s seed=%d", id, out.PromptID, *g.Seed)
	s.launchWatch(id)
}

func (s *Server) rejectSubmission(id, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.jobs {
		if g.ID != id || g.Status == "cancelled" {
			continue
		}
		g.Status, g.Phase, g.Error = "failed", "rejected", detail
		g.UpdatedAt = time.Now()
		g.ProgressConnected = false
		finishTiming(&g)
		if err := s.saveRecord("generations", id, g); err != nil {
			log.Printf("保存拒绝提交状态: %v", err)
			return
		}
		s.jobs[i] = g
		return
	}
}
