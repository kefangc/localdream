package app

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (s *Server) jobLocked(id string) (Generation, bool) {
	for _, g := range s.jobs {
		if g.ID == id {
			return g, true
		}
	}
	return Generation{}, false
}

func (s *Server) sequenceStatus(v LongVideo) string {
	if v.Error != "" {
		return "failed"
	}
	for _, seg := range v.Segments {
		if seg.JobID == "" {
			if v.Paused {
				return "paused"
			}
			if strings.TrimSpace(seg.Request.Prompt) == "" {
				return "waiting"
			}
			return "queued"
		}
		g, ok := s.jobLocked(seg.JobID)
		if !ok {
			return "failed"
		}
		if activeGeneration(g) {
			return g.Status
		}
		if g.Status != "completed" {
			return g.Status
		}
	}
	return "completed"
}

// Called under s.mu: all sequence edits and clip claims share this lock.
func (s *Server) prepareSequence(v *LongVideo) error {
	if len(v.Segments) == 0 {
		return fmt.Errorf("至少保留一个片段")
	}
	switch v.AspectRatio {
	case "16:9", "9:16", "1:1", "4:3":
	default:
		return fmt.Errorf("请选择有效画面比例")
	}
	if v.Resolution != "480p" && v.Resolution != "720p" {
		mp, err := strconv.ParseFloat(strings.TrimSuffix(v.Resolution, "MP"), 64)
		if !strings.HasSuffix(v.Resolution, "MP") || err != nil || !positive(mp) || mp < 0.1 || mp > 1.9 {
			return fmt.Errorf("请选择 480p、720p 或 0.1–1.9MP")
		}
		v.Resolution = fmt.Sprintf("%.1fMP", mp)
	}
	previousFrames := 0
	for i := range v.Segments {
		seg := &v.Segments[i]
		if seg.JobID != "" {
			previousFrames = seg.Timing.InputFrames
			continue
		}
		seg.Request.AspectRatio = v.AspectRatio
		seg.Request.ProjectID = v.ProjectID
		seg.Request.Resolution = v.Resolution
		if err := validateClipRequest(&seg.Request, true); err != nil {
			return fmt.Errorf("第 %d 段：%w", i+1, err)
		}
		timing, err := calculateClipTiming(seg.Request.Duration, i+1, v.Settings, previousFrames)
		if err != nil {
			return fmt.Errorf("第 %d 段：%w", i+1, err)
		}
		seg.Timing = timing
		previousFrames = timing.InputFrames
		seen := map[string]bool{}
		ids := []string{}
		counts := map[string]int{}
		for _, id := range seg.Request.AssetIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			found := false
			for _, a := range s.assets {
				if a.ID == id {
					if v.ProjectID != "" && a.ProjectID != v.ProjectID {
						return fmt.Errorf("第 %d 段只能引用当前项目的素材", i+1)
					}
					found = true
					counts[a.Type]++
					break
				}
			}
			if !found {
				return fmt.Errorf("第 %d 段素材不存在：%s", i+1, id)
			}
			ids = append(ids, id)
		}
		seg.Request.AssetIDs = ids
		if counts["image"] > 9 || counts["video"] > 3 || counts["audio"] > 1 {
			return fmt.Errorf("第 %d 段参考素材超过模板上限（9 图／3 视频／1 音频）", i+1)
		}
	}
	return nil
}

// Atomically persist the child generation and its place in the sequence before
// any ComfyUI request. A restart can never lose the link and submit a duplicate.
func (s *Server) claimSequenceClip(vIndex int) (*Generation, error) {
	v := s.longVideos[vIndex]
	if v.Paused {
		return nil, nil
	}
	for index, seg := range v.Segments {
		if seg.JobID != "" {
			g, ok := s.jobLocked(seg.JobID)
			if !ok || g.Status != "completed" {
				return nil, nil
			}
			continue
		}
		if strings.TrimSpace(seg.Request.Prompt) == "" {
			return nil, nil
		}
		id, err := uniqueID("job")
		if err != nil {
			return nil, err
		}
		seed, err := randomSeed()
		if err != nil {
			return nil, err
		}
		in := seg.Request
		in.ReferenceVideoFPS = legacyReferenceVideoFPS(in.ReferenceVideoFPS)
		now := time.Now()
		snapshot := []Asset{}
		for _, assetID := range in.AssetIDs {
			for _, a := range s.assets {
				if a.ID == assetID {
					snapshot = append(snapshot, a)
					break
				}
			}
		}
		if len(snapshot) != len(in.AssetIDs) {
			return nil, fmt.Errorf("%s 第 %d 段引用素材已删除，请重新编辑该段", v.ID, index+1)
		}
		g := Generation{ID: id, Seed: &seed, ClientID: "localdream-" + id, Phase: "created", AssetSnapshot: snapshot, Mode: in.Mode, Status: "queued", Prompt: in.Prompt, Duration: in.Duration, AspectRatio: v.AspectRatio, Resolution: v.Resolution, Steps: in.Steps, Lora: in.Lora, LoraStrength: in.LoraStrength, AssetIDs: in.AssetIDs, CreatedAt: now, UpdatedAt: now, Continuity: &ClipContext{v.ID, index + 1, v.Settings, seg.Timing}}
		g.ProjectID = v.ProjectID
		g.ReferenceVideoFPS = in.ReferenceVideoFPS
		g.ReferenceVideoPreserveDuration = in.ReferenceVideoPreserveDuration
		v.Segments = append([]VideoSegment(nil), v.Segments...)
		v.Segments[index].JobID = id
		v.Revision++
		v.UpdatedAt = now
		jobRaw, err := json.Marshal(g)
		if err != nil {
			return nil, err
		}
		videoRaw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		tx, err := s.db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		if _, err = tx.Exec("INSERT INTO generations(id,payload) VALUES(?,?)", id, string(jobRaw)); err != nil {
			return nil, err
		}
		result, err := tx.Exec("UPDATE long_videos SET payload=? WHERE id=? AND json_extract(payload,'$.revision')=?", string(videoRaw), v.ID, v.Revision-1)
		if err != nil {
			return nil, err
		}
		claimed, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if claimed != 1 {
			_ = tx.Rollback()
			var raw []byte
			if err := s.db.QueryRow("SELECT payload FROM long_videos WHERE id=?", v.ID).Scan(&raw); err != nil {
				return nil, err
			}
			var current LongVideo
			if err := json.Unmarshal(raw, &current); err != nil {
				return nil, err
			}
			s.longVideos[vIndex] = current
			return nil, nil
		}
		if err = tx.Commit(); err != nil {
			return nil, err
		}
		s.longVideos[vIndex] = v
		s.jobs = append([]Generation{g}, s.jobs...)
		return &g, nil
	}
	return nil, nil
}

func requestFromJob(g Generation) generationRequest {
	return generationRequest{Mode: g.Mode, Prompt: g.Prompt, AspectRatio: g.AspectRatio, Resolution: g.Resolution, Duration: g.Duration, Steps: g.Steps, Lora: g.Lora, LoraStrength: g.LoraStrength, AssetIDs: g.AssetIDs, Continuity: g.Continuity, ReferenceVideoFPS: legacyReferenceVideoFPS(g.ReferenceVideoFPS), ReferenceVideoPreserveDuration: g.ReferenceVideoPreserveDuration}
}

func (s *Server) dispatchLongVideos() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return
	}
	for i := range s.longVideos {
		g, err := s.claimSequenceClip(i)
		if err != nil {
			log.Printf("长视频调度: %v", err)
			v := s.longVideos[i]
			v.Paused = true
			v.Error = err.Error()
			v.Revision++
			if s.saveRecord("long_videos", v.ID, v) == nil {
				s.longVideos[i] = v
			}
			continue
		}
		if g != nil {
			s.workers.Add(1)
			go func(job Generation) { defer s.workers.Done(); s.runJob(job.ID, requestFromJob(job)) }(*g)
		}
	}
}

// Keep exactly the newest successful latent slot. Older slots are never needed
// by the forward-only chain and can otherwise accumulate indefinitely.
func (s *Server) pruneSequenceLatents(c *ClipContext) {
	// Latent history is intentionally retained. Regenerating a middle segment
	// forks from the prefix and needs the predecessor's saved context.
}

func (s *Server) regenerateOrFork(v *LongVideo, index int) (*LongVideo, error) {
	if index < 1 || index > len(v.Segments) {
		return nil, fmt.Errorf("片段编号无效")
	}
	for i := index - 1; i < len(v.Segments); i++ {
		if v.Segments[i].JobID == "" {
			continue
		}
		g, ok := s.jobLocked(v.Segments[i].JobID)
		if ok && activeGeneration(g) {
			return nil, fmt.Errorf("片段 %d 仍在生成，不能重生成", i+1)
		}
	}
	if index == len(v.Segments) {
		g, ok := s.jobLocked(v.Segments[index-1].JobID)
		if ok {
			g.Superseded = true
			g.UpdatedAt = time.Now()
			_ = s.saveRecord("generations", g.ID, g)
			for j := range s.jobs {
				if s.jobs[j].ID == g.ID {
					s.jobs[j] = g
				}
			}
		}
		v.Segments[index-1].JobID = ""
		v.Paused = false
		v.Error = ""
		return v, nil
	}

	// A middle reroll is a new sequence: the original remains available for
	// comparison/recovery, while the fork owns a separate latent namespace.
	id, err := uniqueID("sequence")
	if err != nil {
		return nil, err
	}
	fork := *v
	fork.ID = id
	fork.Title = fmt.Sprintf("%s · 从片段 %d 继续", v.Title, index)
	fork.Revision = 1
	fork.CreatedAt = time.Now()
	fork.UpdatedAt = fork.CreatedAt
	fork.Paused = false
	fork.Error = ""
	fork.Segments = append([]VideoSegment(nil), v.Segments...)
	for i := index - 1; i < len(fork.Segments); i++ {
		fork.Segments[i].JobID = ""
	}
	if err := s.copySequenceLatents(v.ID, fork.ID, index-1); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(fork)
	if err != nil {
		return nil, err
	}
	if _, err = s.db.Exec("INSERT INTO long_videos(id,payload) VALUES(?,?)", fork.ID, string(raw)); err != nil {
		return nil, err
	}
	s.longVideos = append([]LongVideo{fork}, s.longVideos...)
	return &fork, nil
}

func (s *Server) copySequenceLatents(sourceID, targetID string, requiredSlot int) error {
	if s.comfyInput == "" {
		return fmt.Errorf("ComfyUI 输出目录未配置，无法创建连续视频 fork")
	}
	output := env("COMFYUI_OUTPUT", filepath.Join(filepath.Dir(s.comfyInput), "output"))
	src := filepath.Join(output, "h3_context", "localdream", sourceID)
	dst := filepath.Join(output, "h3_context", "localdream", targetID)
	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("原视频缺少 latent 历史，无法从片段 %d fork: %w", requiredSlot+1, err)
	}
	if err = os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	found := false
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "clip_") || !strings.HasSuffix(entry.Name(), ".safetensors") {
			continue
		}
		var slot int
		if _, err = fmt.Sscanf(entry.Name(), "clip_%05d.safetensors", &slot); err != nil {
			continue
		}
		if slot == requiredSlot {
			found = true
		}
		if err = copyFile(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("原视频缺少 clip_%05d.safetensors，无法从片段 %d fork", requiredSlot, requiredSlot+1)
	}
	return nil
}

func (s *Server) regenerateSequence(v *LongVideo, index int) error {
	if index < 1 || index > len(v.Segments) {
		return fmt.Errorf("片段编号无效")
	}
	for i := index - 1; i < len(v.Segments); i++ {
		seg := v.Segments[i]
		if seg.JobID == "" {
			continue
		}
		g, ok := s.jobLocked(seg.JobID)
		if ok && activeGeneration(g) {
			return fmt.Errorf("片段 %d 仍在生成，不能重生成", i+1)
		}
		if ok && !g.Superseded {
			g.Superseded = true
			g.UpdatedAt = time.Now()
			_ = s.saveRecord("generations", g.ID, g)
			for j := range s.jobs {
				if s.jobs[j].ID == g.ID {
					s.jobs[j] = g
				}
			}
		}
		v.Segments[i].JobID = ""
	}
	v.Paused = false
	v.Error = ""
	return nil
}

func (s *Server) startLongVideoDispatcher() {
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			s.dispatchLongVideos()
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
