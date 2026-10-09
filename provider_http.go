package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

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
	intent, err := parseAgentJSON(out.Choices[0].Message.Content)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": "Agent 返回的不是有效 JSON"})
		return
	}
	writeJSON(w, 200, intent)
}

func parseAgentJSON(content string) (any, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		firstNewline := strings.IndexByte(content, '\n')
		if firstNewline >= 0 {
			content = content[firstNewline+1:]
		}
		content = strings.TrimSpace(strings.TrimSuffix(content, "```"))
	}
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object in Agent response")
	}
	var intent map[string]any
	if err := json.Unmarshal([]byte(content[start:end+1]), &intent); err != nil {
		return nil, err
	}
	return intent, nil
}
