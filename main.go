package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Asset struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	URL       string    `json:"url"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}
type Generation struct {
	ID           string    `json:"id"`
	Mode         string    `json:"mode"`
	Status       string    `json:"status"`
	Prompt       string    `json:"prompt"`
	Error        string    `json:"error,omitempty"`
	OutputURL    string    `json:"output_url,omitempty"`
	Duration     float64   `json:"duration"`
	AspectRatio  string    `json:"aspect_ratio"`
	Resolution   string    `json:"resolution"`
	Steps        int       `json:"steps"`
	Lora         string    `json:"lora,omitempty"`
	LoraStrength float64   `json:"lora_strength,omitempty"`
	AssetIDs     []string  `json:"asset_ids"`
	Progress     int       `json:"progress"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type ProviderConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}
type Server struct {
	mu       sync.RWMutex
	assets   []Asset
	jobs     []Generation
	seq      int
	root     string
	comfyURL string
}

func main() {
	root, _ := os.Getwd()
	data := filepath.Join(root, "data")
	_ = os.MkdirAll(filepath.Join(data, "assets"), 0755)
	s := &Server{root: root, comfyURL: env("LOCALDREAM_ADAPTER_URL", "http://127.0.0.1:8791"), assets: []Asset{}, jobs: []Generation{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/assets", s.assetsHandler)
	mux.HandleFunc("/api/assets/", s.deleteAssetHandler)
	mux.HandleFunc("/api/generations", s.generationsHandler)
	mux.HandleFunc("/api/generations/", s.generationHandler)
	mux.HandleFunc("/api/providers/models", s.modelsHandler)
	mux.HandleFunc("/api/agent/interpret", s.agentHandler)
	mux.HandleFunc("/api/config/provider", s.providerConfigHandler)
	mux.HandleFunc("/api/workflows/import", s.importWorkflowHandler)
	mux.Handle("/media/", http.StripPrefix("/media/", http.FileServer(http.Dir(filepath.Join(data, "assets")))))
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(root, "frontend", "dist"))))
	addr := env("LOCALDREAM_ADDR", "127.0.0.1:8790")
	log.Printf("Local Dream listening at http://%s", addr)
	log.Fatal(http.ListenAndServe(addr, cors(mux)))
}

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
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
	if err := r.ParseMultipartForm(256 << 20); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	file, head, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "file is required"})
		return
	}
	defer file.Close()
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("asset_%d", s.seq)
	s.mu.Unlock()
	ext := filepath.Ext(head.Filename)
	if ext == "" {
		ext = ".bin"
	}
	name := id + ext
	dstPath := filepath.Join(s.root, "data", "assets", name)
	dst, err := os.Create(dstPath)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	size, err := io.Copy(dst, file)
	dst.Close()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	t := "image"
	if strings.HasPrefix(head.Header.Get("Content-Type"), "video/") || strings.HasSuffix(strings.ToLower(ext), ".mp4") || strings.HasSuffix(strings.ToLower(ext), ".mov") {
		t = "video"
	}
	a := Asset{ID: id, Name: head.Filename, Type: t, URL: "/media/" + name, Size: size, CreatedAt: time.Now()}
	s.mu.Lock()
	s.assets = append([]Asset{a}, s.assets...)
	s.mu.Unlock()
	writeJSON(w, 201, a)
}

func (s *Server) deleteAssetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/assets/")
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
		filename := filepath.Base(asset.URL)
		_ = os.Remove(filepath.Join(s.root, "data", "assets", filename))
		s.assets = append(s.assets[:i], s.assets[i+1:]...)
		writeJSON(w, http.StatusNoContent, nil)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "asset not found"})
}

var mentionRE = regexp.MustCompile(`@([\p{Han}A-Za-z0-9_-]+)`)

func (s *Server) generationsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.mu.RLock()
		defer s.mu.RUnlock()
		writeJSON(w, 200, s.jobs)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	var in struct {
		Mode         string   `json:"mode"`
		Prompt       string   `json:"prompt"`
		AspectRatio  string   `json:"aspect_ratio"`
		Resolution   string   `json:"resolution"`
		Duration     float64  `json:"duration"`
		Steps        int      `json:"steps"`
		Lora         string   `json:"lora"`
		LoraStrength float64  `json:"lora_strength"`
		AssetIDs     []string `json:"asset_ids"`
		UseAgent     bool     `json:"use_agent"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid JSON"})
		return
	}
	if in.Mode == "first_last_frame" && len(in.AssetIDs) < 2 {
		writeJSON(w, 400, map[string]string{"error": "首尾帧模式至少需要两个素材"})
		return
	}
	if in.Prompt == "" && !in.UseAgent {
		writeJSON(w, 400, map[string]string{"error": "提示词不能为空"})
		return
	}
	if in.Duration < 5 || in.Duration > 15 {
		writeJSON(w, 400, map[string]string{"error": "时长必须在 5 到 15 秒之间"})
		return
	}
	if in.Steps == 0 {
		in.Steps = 4
	}
	if in.Steps < 1 || in.Steps > 100 {
		writeJSON(w, 400, map[string]string{"error": "步数必须在 1 到 100 之间"})
		return
	}
	if in.LoraStrength < 0 || in.LoraStrength > 1 {
		writeJSON(w, 400, map[string]string{"error": "LoRA 强度必须在 0 到 1 之间"})
		return
	}
	s.mu.Lock()
	s.seq++
	id := fmt.Sprintf("job_%d", s.seq)
	now := time.Now()
	g := Generation{ID: id, Mode: in.Mode, Status: "queued", Prompt: in.Prompt, Duration: in.Duration, AspectRatio: in.AspectRatio, Resolution: in.Resolution, Steps: in.Steps, Lora: in.Lora, LoraStrength: in.LoraStrength, AssetIDs: in.AssetIDs, Progress: 0, CreatedAt: now, UpdatedAt: now}
	s.jobs = append([]Generation{g}, s.jobs...)
	s.mu.Unlock()
	go s.runJob(id, in.Mode, in.Prompt, in.Duration, in.AspectRatio, in.Resolution, in.Steps, in.Lora, in.LoraStrength, in.AssetIDs)
	writeJSON(w, 202, g)
}
func (s *Server) runJob(id, mode, prompt string, duration float64, ratio, resolution string, steps int, lora string, loraStrength float64, assetIDs []string) {
	for _, p := range []int{8, 18} {
		time.Sleep(250 * time.Millisecond)
		if s.jobCancelled(id) {
			return
		}
		s.updateJob(id, "running", p, "")
	}
	if s.jobCancelled(id) {
		return
	}
	payload := map[string]any{"prompt": prompt, "mode": mode, "duration": duration, "aspect_ratio": ratio, "resolution": resolution, "steps": steps, "lora": lora, "lora_strength": loraStrength, "asset_ids": assetIDs}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, s.comfyURL+"/api/localdream/submit", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if s.jobCancelled(id) {
			return
		}
		s.updateJob(id, "failed", 0, "ComfyUI 未连接："+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		if s.jobCancelled(id) {
			return
		}
		body, _ := io.ReadAll(resp.Body)
		s.updateJob(id, "failed", 0, "适配层错误："+string(body))
		return
	}
	var out struct {
		PromptID  string `json:"prompt_id"`
		Status    string `json:"status"`
		OutputURL string `json:"output_url"`
		Error     string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.PromptID == "" {
		s.updateJob(id, "failed", 18, "适配层未返回 ComfyUI prompt_id")
		return
	}
	s.updateJob(id, "running", 20, "")
	deadline := time.Now().Add(30 * time.Minute)
	for time.Now().Before(deadline) {
		if s.jobCancelled(id) {
			return
		}
		statusReq, err := http.NewRequest(http.MethodGet, s.comfyURL+"/api/localdream/status/"+out.PromptID, nil)
		if err == nil {
			statusResp, statusErr := http.DefaultClient.Do(statusReq)
			if statusErr == nil {
				var state struct {
					Status    string `json:"status"`
					Progress  int    `json:"progress"`
					OutputURL string `json:"output_url"`
					Error     string `json:"error"`
				}
				_ = json.NewDecoder(statusResp.Body).Decode(&state)
				statusResp.Body.Close()
				if state.Status == "failed" {
					s.updateJob(id, "failed", state.Progress, "ComfyUI 生成失败："+state.Error)
					return
				}
				if state.Status == "completed" {
					s.completeJob(id, state.OutputURL)
					return
				}
				progress := state.Progress
				if progress < 20 {
					progress = 20
				}
				if progress > 99 {
					progress = 99
				}
				s.updateJob(id, "running", progress, "")
			}
		}
		time.Sleep(time.Second)
	}
	s.updateJob(id, "failed", 20, "等待 ComfyUI 结果超时")
}

func (s *Server) completeJob(id, outputURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			s.jobs[i].Status = "completed"
			s.jobs[i].Progress = 100
			s.jobs[i].OutputURL = outputURL
			s.jobs[i].UpdatedAt = time.Now()
		}
	}
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
func (s *Server) updateJob(id, status string, progress int, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.jobs {
		if s.jobs[i].ID == id {
			s.jobs[i].Status = status
			s.jobs[i].Progress = progress
			s.jobs[i].Error = errMsg
			s.jobs[i].UpdatedAt = time.Now()
		}
	}
}
func (s *Server) generationHandler(w http.ResponseWriter, r *http.Request) {
	pathID := strings.TrimPrefix(r.URL.Path, "/api/generations/")
	isCancel := r.Method == http.MethodPost && strings.HasSuffix(pathID, "/cancel")
	id := strings.TrimSuffix(pathID, "/cancel")
	if id == "" {
		writeJSON(w, 404, nil)
		return
	}
	if r.Method == http.MethodGet {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, j := range s.jobs {
			if j.ID == id {
				writeJSON(w, 200, j)
				return
			}
		}
		writeJSON(w, 404, map[string]string{"error": "not found"})
		return
	}
	if isCancel {
		s.updateJob(id, "cancelled", 0, "")
		writeJSON(w, 200, map[string]string{"status": "cancelled"})
		return
	}
	writeJSON(w, 405, nil)
}
func (s *Server) modelsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, 405, nil)
		return
	}
	var c ProviderConfig
	if json.NewDecoder(r.Body).Decode(&c) != nil || c.BaseURL == "" {
		writeJSON(w, 400, map[string]string{"error": "base_url is required"})
		return
	}
	req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/models", nil)
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		writeJSON(w, resp.StatusCode, map[string]string{"error": "provider returned " + resp.Status})
		return
	}
	var raw struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&raw) != nil {
		writeJSON(w, 502, map[string]string{"error": "invalid models response"})
		return
	}
	out := []string{}
	for _, m := range raw.Data {
		out = append(out, m.ID)
	}
	writeJSON(w, 200, map[string]any{"models": out})
}

func (s *Server) providerConfigPath() string { return filepath.Join(s.root, "data", "provider.json") }

func (s *Server) providerConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var c ProviderConfig
		if b, err := os.ReadFile(s.providerConfigPath()); err == nil {
			_ = json.Unmarshal(b, &c)
		}
		c.APIKey = ""
		writeJSON(w, 200, c)
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, 405, nil)
		return
	}
	var c ProviderConfig
	if json.NewDecoder(r.Body).Decode(&c) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid config"})
		return
	}
	if old, err := os.ReadFile(s.providerConfigPath()); err == nil && c.APIKey == "" {
		var prev ProviderConfig
		_ = json.Unmarshal(old, &prev)
		c.APIKey = prev.APIKey
	}
	_ = os.MkdirAll(filepath.Dir(s.providerConfigPath()), 0755)
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(s.providerConfigPath(), b, 0600); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	c.APIKey = ""
	writeJSON(w, 200, c)
}

func (s *Server) agentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, nil)
		return
	}
	var in struct {
		Prompt  string           `json:"prompt"`
		Assets  []map[string]any `json:"assets"`
		BaseURL string           `json:"base_url"`
		APIKey  string           `json:"api_key"`
		Model   string           `json:"model"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Prompt) == "" {
		writeJSON(w, 400, map[string]string{"error": "prompt is required"})
		return
	}
	if in.BaseURL == "" {
		var c ProviderConfig
		if b, err := os.ReadFile(s.providerConfigPath()); err == nil {
			_ = json.Unmarshal(b, &c)
			in.BaseURL, in.APIKey, in.Model = c.BaseURL, c.APIKey, c.Model
		}
	}
	if in.BaseURL == "" || in.Model == "" {
		writeJSON(w, 400, map[string]string{"error": "请先配置 Agent Base URL 和模型"})
		return
	}
	bodyObj := map[string]any{"model": in.Model, "temperature": 0.4, "response_format": map[string]string{"type": "json_object"}, "messages": []any{map[string]any{"role": "system", "content": "你是本地视频创作助手。只输出 JSON，字段为 mode, duration, aspect_ratio, resolution, prompt（含 subject,motion,camera,environment,constraints）, assets（含 id,role）。不要创建或修改 ComfyUI 节点。"}, map[string]any{"role": "user", "content": fmt.Sprintf("用户需求：%s\n素材：%v", in.Prompt, in.Assets)}}}
	b, _ := json.Marshal(bodyObj)
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(in.BaseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+in.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		writeJSON(w, resp.StatusCode, map[string]string{"error": string(raw)})
		return
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &out) != nil || len(out.Choices) == 0 {
		writeJSON(w, 502, map[string]string{"error": "invalid chat completion response"})
		return
	}
	var intent any
	if json.Unmarshal([]byte(out.Choices[0].Message.Content), &intent) != nil {
		writeJSON(w, 502, map[string]string{"error": "Agent 返回的不是有效 JSON"})
		return
	}
	writeJSON(w, 200, intent)
}
func (s *Server) importWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, nil)
		return
	}
	var in struct {
		Workflow json.RawMessage `json:"workflow"`
		Name     string          `json:"name"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid workflow"})
		return
	}
	var doc struct {
		Nodes []struct {
			ID      int             `json:"id"`
			Type    string          `json:"type"`
			Title   string          `json:"title"`
			Widgets json.RawMessage `json:"widgets_values"`
		} `json:"nodes"`
	}
	workflowBytes := in.Workflow
	if len(workflowBytes) > 0 && workflowBytes[0] == '"' {
		var encoded string
		if json.Unmarshal(workflowBytes, &encoded) == nil {
			workflowBytes = []byte(encoded)
		}
	}
	if json.Unmarshal(workflowBytes, &doc) != nil {
		writeJSON(w, 400, map[string]string{"error": "workflow must be API JSON or exported JSON"})
		return
	}
	candidates := []map[string]any{}
	for _, n := range doc.Nodes {
		t := strings.ToLower(n.Type + " " + n.Title)
		role := ""
		if strings.Contains(t, "text") || strings.Contains(t, "prompt") || strings.Contains(t, "clip") {
			role = "prompt"
		}
		if strings.Contains(t, "loadimage") || strings.Contains(t, "image") {
			role = "image"
		}
		if strings.Contains(t, "video") {
			role = "video"
		}
		if strings.Contains(t, "audio") {
			role = "audio"
		}
		if strings.Contains(t, "duration") || strings.Contains(t, "length") {
			role = "duration"
		}
		if role != "" {
			candidates = append(candidates, map[string]any{"node_id": n.ID, "node_type": n.Type, "role": role, "confidence": 0.55})
		}
	}
	writeJSON(w, 200, map[string]any{"name": in.Name, "candidates": candidates, "warnings": []string{"自动推断结果需要人工确认后才能执行"}})
}
