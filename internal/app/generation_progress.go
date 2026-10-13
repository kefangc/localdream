package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

func activeGeneration(g Generation) bool { return g.Status == "queued" || g.Status == "running" }

func finishTiming(g *Generation) {
	if g.FinishedAt == nil {
		now := time.Now()
		g.FinishedAt = &now
	}
	ms := int64(0)
	if g.StartedAt != nil {
		ms = g.FinishedAt.Sub(*g.StartedAt).Milliseconds()
	}
	if ms < 0 {
		ms = 0
	}
	g.ElapsedMS = &ms
}

// Use ComfyUI's execution_start timestamp when available; observing the running
// queue is only a fallback, never use job creation (which includes queue time).
func markStarted(g *Generation, timestamp int64) {
	now := time.Now()
	if timestamp > 0 {
		started := time.UnixMilli(timestamp)
		if !started.Before(g.CreatedAt) && !started.After(now.Add(time.Minute)) {
			g.StartedAt = &started
			return
		}
	}
	if g.StartedAt == nil {
		g.StartedAt = &now
	}
}

func (s *Server) recordStart(id string, timestamp int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.jobs {
		if g.ID != id || !activeGeneration(g) {
			continue
		}
		markStarted(&g, timestamp)
		if err := s.saveRecord("generations", id, g); err == nil {
			s.jobs[i] = g
		} else {
			log.Printf("保存开始时间: %v", err)
		}
		return
	}
}

func (s *Server) captureHistoryEnd(id, promptID string) {
	var history map[string]struct {
		Status struct {
			Messages [][]json.RawMessage `json:"messages"`
		} `json:"status"`
	}
	if s.comfyJSON("GET", "/history/"+url.PathEscape(promptID), nil, &history) != nil {
		return
	}
	for _, message := range history[promptID].Status.Messages {
		if len(message) != 2 {
			continue
		}
		var kind string
		var data struct {
			Timestamp int64 `json:"timestamp"`
		}
		if json.Unmarshal(message[0], &kind) != nil || json.Unmarshal(message[1], &data) != nil {
			continue
		}
		if kind == "execution_start" && data.Timestamp > 0 {
			s.recordStart(id, data.Timestamp)
		}
		if kind == "execution_success" || kind == "execution_error" || kind == "execution_interrupted" {
			s.recordEnd(id, data.Timestamp)
			return
		}
	}
}

func (s *Server) recordEnd(id string, timestamp int64) {
	if timestamp <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.jobs {
		if g.ID != id || !activeGeneration(g) {
			continue
		}
		end := time.UnixMilli(timestamp)
		if end.Before(g.CreatedAt) || end.After(time.Now().Add(time.Minute)) {
			return
		}
		g.FinishedAt = &end
		if err := s.saveRecord("generations", id, g); err == nil {
			s.jobs[i] = g
		} else {
			log.Printf("保存结束时间: %v", err)
		}
		return
	}
}

func (s *Server) dialProgress(ctx context.Context, clientID string) (*websocket.Conn, error) {
	u, err := url.Parse(strings.TrimRight(s.comfyURL, "/") + "/ws")
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return nil, errors.New("invalid ComfyUI URL")
	}
	query := u.Query()
	query.Set("clientId", clientID)
	u.RawQuery = query.Encode()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, u.String(), nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	return conn, err
}

func (s *Server) setProgressConnection(id string, connected bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.jobs {
		if g.ID != id || !activeGeneration(g) || g.ProgressConnected == connected {
			continue
		}
		g.ProgressConnected = connected
		// Connection state does not invalidate the last measured node progress.
		if err := s.saveRecord("generations", id, g); err == nil {
			s.jobs[i] = g
		} else {
			log.Printf("保存进度连接状态: %v", err)
		}
	}
}

func (s *Server) startProgress(id string) {
	g, ok := s.job(id)
	if !ok || !activeGeneration(g) || g.ClientID == "" {
		return
	}
	s.mu.Lock()
	if s.progressStreams == nil {
		s.progressStreams = map[string]bool{}
	}
	if s.progressStreams[id] {
		s.mu.Unlock()
		return
	}
	s.progressStreams[id] = true
	s.workers.Add(1)
	s.mu.Unlock()
	// Restore the connection flag, but keep the last measured progress visible.
	s.setProgressConnection(id, false)
	conn, err := s.dialProgress(s.ctx, g.ClientID)
	if err != nil {
		log.Printf("%s 实时进度暂不可用，将重连: %v", id, err)
	}
	go func() {
		defer s.workers.Done()
		defer func() { s.mu.Lock(); delete(s.progressStreams, id); s.mu.Unlock() }()
		ctx, cancel := context.WithCancel(s.ctx)
		defer cancel()
		// End a blocked socket read as soon as the task terminates or app shuts down.
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					g, ok := s.job(id)
					if !ok || !activeGeneration(g) {
						cancel()
						return
					}
				}
			}
		}()
		backoff := time.Second
		for ctx.Err() == nil {
			if conn == nil {
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				conn, err = s.dialProgress(ctx, g.ClientID)
				if err != nil {
					if backoff < 15*time.Second {
						backoff *= 2
					}
					continue
				}
			}
			backoff = time.Second
			s.setProgressConnection(id, true)
			current := conn
			closeOnCancel := context.AfterFunc(ctx, func() { current.Close() })
			conn.SetReadLimit(64 << 20)
			conn.SetPongHandler(func(string) error {
				return current.SetReadDeadline(time.Now().Add(45 * time.Second))
			})
			heartbeatDone := make(chan struct{})
			go func() {
				ticker := time.NewTicker(15 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-heartbeatDone:
						return
					case <-ticker.C:
						if err := current.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
							current.Close()
							return
						}
					}
				}
			}()
			for ctx.Err() == nil {
				// Pongs keep an idle, healthy connection alive without new progress.
				conn.SetReadDeadline(time.Now().Add(45 * time.Second))
				kind, reader, readErr := conn.NextReader()
				if readErr != nil {
					break
				}
				if kind != websocket.TextMessage {
					if _, err := io.Copy(io.Discard, reader); err != nil {
						break
					}
					continue
				}
				raw, readErr := io.ReadAll(io.LimitReader(reader, 2<<20))
				if readErr != nil {
					break
				}
				if err := s.applyProgressEvent(id, raw); err != nil {
					log.Printf("%s 进度事件: %v", id, err)
				}
			}
			close(heartbeatDone)
			closeOnCancel()
			conn.Close()
			conn = nil
			s.setProgressConnection(id, false)
		}
		if conn != nil {
			conn.Close()
		}
	}()
}

func progressStage(classType, nodeID string) string {
	switch {
	case strings.Contains(classType, "Sampler"):
		return "采样"
	case classType == "VAEDecodeAudio":
		return "音频解码"
	case strings.Contains(classType, "VAEDecode"):
		return "视频解码"
	case classType == "CreateVideo" || strings.Contains(classType, "SaveVideo"):
		return "合成/保存视频"
	case strings.Contains(classType, "Loader"):
		return "加载模型/素材"
	case strings.Contains(classType, "MiniMaxH3"):
		return "准备参考条件"
	}
	if classType != "" {
		return classType
	}
	return "节点 " + nodeID
}

func (s *Server) applyProgressEvent(id string, raw []byte) error {
	var event struct {
		Type string `json:"type"`
		Data struct {
			PromptID         string  `json:"prompt_id"`
			Node             *string `json:"node"`
			Value            float64 `json:"value"`
			Max              float64 `json:"max"`
			Timestamp        int64   `json:"timestamp"`
			ExceptionMessage string  `json:"exception_message"`
			Nodes            map[string]struct {
				State     string  `json:"state"`
				Value     float64 `json:"value"`
				Max       float64 `json:"max"`
				DisplayID string  `json:"display_node_id"`
			} `json:"nodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return err
	}
	switch event.Type {
	case "execution_start", "executing", "progress", "progress_state", "execution_success", "execution_error", "execution_interrupted":
	default:
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, g := range s.jobs {
		if g.ID != id || !activeGeneration(g) {
			continue
		}
		d := event.Data
		if g.PromptID != "" && d.PromptID != "" && g.PromptID != d.PromptID {
			return nil
		}
		if g.PromptID == "" && d.PromptID != "" {
			g.PromptID = d.PromptID
		}
		setNode := func(node, display string, value, max float64, known bool) {
			if node == g.ProgressNode && g.ProgressKnown && (!known || max <= 0 || value < 0) {
				return // A generic same-node status is not a new measurement.
			}
			g.Status = "running"
			markStarted(&g, 0)
			g.ProgressNode = node
			class := g.NodeTypes[node]
			if class == "" {
				class = g.NodeTypes[display]
			}
			g.ProgressStage = progressStage(class, node)
			g.ProgressKnown = known && max > 0 && value >= 0
			g.ProgressValue = value
			g.ProgressMax = max
			g.Progress = 0
			if g.ProgressKnown {
				g.Progress = int(clamp(value/max*100, 0, 100))
			}
		}
		switch event.Type {
		case "execution_start":
			g.Status = "running"
			markStarted(&g, d.Timestamp)
			g.ProgressStage = "准备执行"
			g.ProgressKnown = false
		case "executing":
			if d.Node == nil {
				g.ProgressStage = "整理输出"
				g.ProgressKnown = false
			} else if *d.Node != g.ProgressNode {
				setNode(*d.Node, "", 0, 0, false)
			}
		case "progress":
			if d.Node != nil {
				setNode(*d.Node, "", d.Value, d.Max, true)
			}
		case "progress_state":
			keys := make([]string, 0, len(d.Nodes))
			for node, n := range d.Nodes {
				if n.State == "running" {
					keys = append(keys, node)
				}
			}
			sort.Strings(keys)
			selected := ""
			for _, node := range keys {
				if node == g.ProgressNode {
					selected = node
					break
				}
			}
			if selected == "" && len(keys) > 0 {
				selected = keys[0]
			}
			if selected != "" {
				n := d.Nodes[selected]
				setNode(selected, n.DisplayID, n.Value, n.Max, n.Max > 1 || n.Value > 0)
			}
		case "execution_success", "execution_error", "execution_interrupted":
			end := time.Now()
			if d.Timestamp > 0 {
				candidate := time.UnixMilli(d.Timestamp)
				if !candidate.Before(g.CreatedAt) && !candidate.After(end.Add(time.Minute)) {
					end = candidate
				}
			}
			g.FinishedAt = &end
			g.ProgressKnown = false
			if event.Type == "execution_success" {
				g.ProgressStage = "整理输出"
			} else {
				g.Status = "failed"
				g.Error = "ComfyUI 执行失败: " + d.ExceptionMessage
				if event.Type == "execution_interrupted" {
					g.Error = "ComfyUI 生成被中断"
				}
				finishTiming(&g)
				g.ProgressConnected = false
			}
		}
		g.UpdatedAt = time.Now()
		if err := s.saveRecord("generations", id, g); err != nil {
			return err
		}
		s.jobs[i] = g
		return nil
	}
	return nil
}
