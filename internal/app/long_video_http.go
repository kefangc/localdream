package app

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type sequenceInput struct {
	ProjectID   string              `json:"project_id,omitempty"`
	Title       string              `json:"title"`
	AspectRatio string              `json:"aspect_ratio"`
	Resolution  string              `json:"resolution"`
	Segments    []generationRequest `json:"segments"`
	Revision    int                 `json:"revision"`
	Start       bool                `json:"start"`
}

type sequenceView struct {
	LongVideo
	Status        string  `json:"status"`
	Completed     int     `json:"completed"`
	ActualSeconds float64 `json:"actual_seconds"`
}

func (s *Server) viewSequence(v LongVideo) sequenceView {
	out := sequenceView{LongVideo: v, Status: s.sequenceStatus(v)}
	for _, seg := range v.Segments {
		out.ActualSeconds += seg.Timing.EffectiveSeconds
		if g, ok := s.jobLocked(seg.JobID); ok && g.Status == "completed" {
			out.Completed++
		}
	}
	return out
}

func (s *Server) longVideosHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.RLock()
		defer s.mu.RUnlock()
		out := []sequenceView{}
		for _, v := range s.longVideos {
			out = append(out, s.viewSequence(v))
		}
		writeJSON(w, 200, out)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, nil)
		return
	}
	var in sequenceInput
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		writeJSON(w, 400, map[string]string{"error": "无效分段计划"})
		return
	}
	c, err := s.longVideoSettings()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	id, err := uniqueID("sequence")
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	now := time.Now()
	v := LongVideo{ID: id, Title: strings.TrimSpace(in.Title), AspectRatio: in.AspectRatio, Resolution: in.Resolution, Settings: c, Paused: !in.Start, Revision: 1, CreatedAt: now, UpdatedAt: now}
	v.ProjectID = in.ProjectID
	if v.Title == "" {
		v.Title = "未命名长视频"
	}
	for _, req := range in.Segments {
		v.Segments = append(v.Segments, VideoSegment{Request: req})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.validProjectLocked(v.ProjectID) {
		writeJSON(w, 400, map[string]string{"error": "项目不存在或已归档"})
		return
	}
	if err = s.prepareSequence(&v); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err = s.saveRecord("long_videos", id, v); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.longVideos = append([]LongVideo{v}, s.longVideos...)
	writeJSON(w, 201, s.viewSequence(v))
}

func (s *Server) longVideoHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/long-videos/"), "/")
	id := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	var in sequenceInput
	if r.Method == http.MethodPut {
		if json.NewDecoder(r.Body).Decode(&in) != nil {
			writeJSON(w, 400, map[string]string{"error": "无效分段计划"})
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, stored := range s.longVideos {
		if stored.ID != id {
			continue
		}
		if r.Method == http.MethodGet {
			writeJSON(w, 200, s.viewSequence(stored))
			return
		}
		v := stored
		v.Error = ""
		v.Segments = append([]VideoSegment(nil), stored.Segments...)
		var err error
		switch {
		case r.Method == http.MethodPut && action == "":
			if in.Revision != v.Revision {
				writeJSON(w, 409, map[string]string{"error": "分段状态已更新，请重新载入后编辑（避免覆盖已开始的片段）"})
				return
			}
			if in.AspectRatio != v.AspectRatio || in.Resolution != v.Resolution {
				writeJSON(w, 400, map[string]string{"error": "当前 latent 续接要求比例与分辨率保持不变；请新建视频更换画幅"})
				return
			}
			next := make([]VideoSegment, len(in.Segments))
			for j, seg := range v.Segments {
				if seg.JobID != "" && j >= len(next) {
					err = fmt.Errorf("不能移除已提交的片段")
					break
				}
			}
			if err == nil {
				for j, req := range in.Segments {
					if j < len(v.Segments) && v.Segments[j].JobID != "" {
						next[j] = v.Segments[j]
					} else {
						next[j] = VideoSegment{Request: req}
					}
				}
				v.Segments = next
				v.Title = in.Title
				// Saving a draft only pauses future dispatch; a running clip continues.
				v.Paused = !in.Start
				err = s.prepareSequence(&v)
			}
		case r.Method == http.MethodPost && action == "pause":
			v.Paused = true
		case r.Method == http.MethodPost && action == "start":
			v.Paused = false
		case r.Method == http.MethodPost && action == "retry":
			err = s.retrySequence(&v)
		case r.Method == http.MethodPost && action == "regenerate":
			var body struct {
				Index   int                `json:"index"`
				Segment *generationRequest `json:"segment"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				err = fmt.Errorf("无效片段编号")
			} else {
				if body.Segment != nil {
					if body.Index < 1 || body.Index > len(v.Segments) {
						err = fmt.Errorf("片段编号无效")
					} else {
						current := &v.Segments[body.Index-1]
						if current.JobID != "" {
							if g, ok := s.jobLocked(current.JobID); ok && activeGeneration(g) {
								err = fmt.Errorf("当前片段仍在生成")
							}
						}
						if err == nil {
							current.Request = *body.Segment
							current.Request.ProjectID = v.ProjectID
							current.JobID = ""
							err = s.prepareSequence(&v)
						}
					}
				}
				var result *LongVideo
				result, err = s.regenerateOrFork(&v, body.Index)
				if err == nil {
					// A middle reroll returns a new sequence and must not overwrite
					// the original row. The response is written immediately below.
					if result.ID != stored.ID {
						writeJSON(w, 200, s.viewSequence(*result))
						return
					}
					v = *result
				}
			}
		default:
			writeJSON(w, 405, nil)
			return
		}
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		v.Revision++
		v.UpdatedAt = time.Now()
		if err = s.saveRecord("long_videos", v.ID, v); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		s.longVideos[i] = v
		writeJSON(w, 200, s.viewSequence(v))
		return
	}
	writeJSON(w, 404, map[string]string{"error": "长视频不存在"})
}

// Do not retry an ambiguous POST, or locally cancelled work that may still run
// remotely. This is the one retry boundary that prevents duplicate GPU jobs.
func (s *Server) retrySequence(v *LongVideo) error {
	for i, seg := range v.Segments {
		if seg.JobID == "" {
			return fmt.Errorf("没有失败片段需要重试")
		}
		g, ok := s.jobLocked(seg.JobID)
		if !ok {
			return fmt.Errorf("片段记录不存在")
		}
		if g.Status == "completed" {
			continue
		}
		if g.Status != "failed" {
			return fmt.Errorf("只能重试已确认失败的当前片段")
		}
		if g.Phase != "created" && g.Phase != "rejected" {
			promptID, status, err := s.remoteJob(g)
			if err != nil {
				return err
			}
			if status != "history" {
				return fmt.Errorf("远端结果尚未确认，请先核对 ComfyUI；未重复提交")
			}
			_, _, failed, _, err := s.comfyResult(promptID)
			if err != nil {
				return err
			}
			if !failed {
				return fmt.Errorf("远端未确认失败，不能重复生成")
			}
		}
		v.Segments[i].JobID = ""
		v.Paused = false
		return nil
	}
	return fmt.Errorf("没有失败片段需要重试")
}
