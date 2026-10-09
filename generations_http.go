package main

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var mentionRE = regexp.MustCompile(`@([\p{Han}A-Za-z0-9_-]+)`)

func (s *Server) generationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.RLock()
		defer s.mu.RUnlock()
		jobs := make([]Generation, 0, len(s.jobs))
		for _, g := range s.jobs {
			if g.Deleted {
				continue
			}
			jobs = append(jobs, publicGeneration(g))
		}
		writeJSON(w, 200, jobs)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	var in generationRequest
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	g, status, err := s.createGeneration(in)
	if err != nil {
		code := ""
		if status == http.StatusConflict {
			code = "long_video_required"
		}
		writeJSON(w, status, map[string]string{"error": err.Error(), "code": code})
		return
	}
	writeJSON(w, http.StatusAccepted, g)
}

// createGeneration is shared by the form and the Agent. It persists the task
// before starting the existing ComfyUI submission worker.
func (s *Server) createGeneration(in generationRequest) (Generation, int, error) {
	if err := normalizeReferenceVideoFPS(&in); err != nil {
		return Generation{}, http.StatusBadRequest, err
	}
	if in.Mode == "first_last_frame" && len(in.AssetIDs) < 2 {
		return Generation{}, 400, fmt.Errorf("首尾帧模式至少需要两个素材")
	}
	settings, err := s.longVideoSettings()
	if err != nil {
		return Generation{}, 500, err
	}
	strategy := in.Strategy
	if strategy == "" {
		strategy = settings.Strategy
	}
	uniqueAssets := map[string]bool{}
	for _, id := range in.AssetIDs {
		uniqueAssets[id] = true
	}
	if strategy == "long" || strategy == "auto" && (in.Duration >= settings.Threshold || in.Duration >= settings.ReferenceThreshold && len(uniqueAssets) >= settings.ReferenceCount) {
		return Generation{}, 409, fmt.Errorf("已达到长视频条件，请在长视频编辑器中填写分段，或明确选择普通模式")
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return Generation{}, 400, fmt.Errorf("提示词不能为空")
	}
	if in.Duration < 1 || in.Duration > 15 {
		return Generation{}, 400, fmt.Errorf("时长必须在 1 到 15 秒之间")
	}
	if strings.HasSuffix(in.Resolution, "MP") {
		mp, err := strconv.ParseFloat(strings.TrimSuffix(in.Resolution, "MP"), 64)
		if err != nil || math.IsNaN(mp) || math.IsInf(mp, 0) || mp < 0.1 || mp > 1.9 {
			return Generation{}, 400, fmt.Errorf("自定义分辨率必须在 0.1 到 1.9 MP 之间")
		}
		in.Resolution = fmt.Sprintf("%.1fMP", mp)
	} else if in.Resolution != "480p" && in.Resolution != "720p" {
		return Generation{}, 400, fmt.Errorf("请选择 480p、720p 或自定义 MP 分辨率")
	}
	if in.Steps == 0 {
		in.Steps = 50
	}
	if in.Steps < 1 || in.Steps > 100 {
		return Generation{}, 400, fmt.Errorf("步数必须在 1 到 100 之间")
	}
	if in.LoraStrength < 0 || in.LoraStrength > 1 {
		return Generation{}, 400, fmt.Errorf("LoRA 强度必须在 0 到 1 之间")
	}
	id, err := uniqueID("job")
	if err != nil {
		return Generation{}, 500, err
	}
	if in.AgentJobID != "" {
		id = in.AgentJobID
	}
	seed, err := randomSeed()
	if err != nil {
		return Generation{}, 500, err
	}
	s.mu.Lock()
	for _, existing := range s.jobs {
		if existing.ID != id {
			continue
		}
		s.mu.Unlock()
		if in.AgentFingerprint != "" && existing.AgentRequestFingerprint == in.AgentFingerprint {
			return existing, http.StatusAccepted, nil
		}
		return Generation{}, http.StatusConflict, fmt.Errorf("此 Agent 请求 ID 已用于另一项生成任务")
	}
	if !s.validProjectLocked(in.ProjectID) {
		s.mu.Unlock()
		return Generation{}, 400, fmt.Errorf("项目不存在或已归档")
	}
	snapshot := make([]Asset, 0, len(in.AssetIDs))
	for _, assetID := range in.AssetIDs {
		found := false
		for _, a := range s.assets {
			if a.ID != assetID {
				continue
			}
			if (in.ProjectID != "" || in.AgentJobID != "") && a.ProjectID != in.ProjectID {
				s.mu.Unlock()
				return Generation{}, 400, fmt.Errorf("只能引用当前项目的素材")
			}
			if _, err := s.assetPath(a); err != nil {
				s.mu.Unlock()
				return Generation{}, 400, err
			}
			snapshot = append(snapshot, a)
			found = true
			break
		}
		if !found {
			s.mu.Unlock()
			return Generation{}, 400, fmt.Errorf("素材不存在: %s", assetID)
		}
	}
	now := time.Now()
	g := Generation{ID: id, Seed: &seed, ClientID: "localdream-" + id, Phase: "created", AssetSnapshot: snapshot, Mode: in.Mode, Status: "queued", Prompt: in.Prompt, Duration: in.Duration, AspectRatio: in.AspectRatio, Resolution: in.Resolution, Steps: in.Steps, Lora: in.Lora, LoraStrength: in.LoraStrength, AssetIDs: in.AssetIDs, Progress: 0, CreatedAt: now, UpdatedAt: now}
	g.ProjectID = in.ProjectID
	g.ReferenceVideoFPS = in.ReferenceVideoFPS
	g.ReferenceVideoPreserveDuration = in.ReferenceVideoPreserveDuration
	g.AgentRequestFingerprint = in.AgentFingerprint
	if err := s.saveRecord("generations", id, g); err != nil {
		s.mu.Unlock()
		return Generation{}, 500, err
	}
	s.jobs = append([]Generation{g}, s.jobs...)
	s.workers.Add(1)
	s.mu.Unlock()
	go func() { defer s.workers.Done(); s.runJob(id, in) }()
	return g, http.StatusAccepted, nil
}
