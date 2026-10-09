package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"localdream/internal/responses"
	"localdream/internal/skills"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type agentTurnInput struct {
	ClientTurnID    string   `json:"client_turn_id"`
	Prompt          string   `json:"prompt"`
	ProjectID       string   `json:"project_id"`
	AssetIDs        []string `json:"asset_ids"`
	ExecutionPolicy string   `json:"execution_policy"`
	SkillPolicy     string   `json:"skill_policy"`
}

var clientTurnIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)

func agentRequestIdentity(input agentTurnInput) (string, string) {
	identity := sha256.Sum256([]byte(input.ProjectID + ":" + input.ClientTurnID))
	serialized, _ := json.Marshal(input)
	fingerprint := sha256.Sum256(serialized)
	return "job_agent_" + hex.EncodeToString(identity[:12]), hex.EncodeToString(fingerprint[:])
}

type agentTurnResult struct {
	Text       string             `json:"text"`
	Skills     []string           `json:"skills"`
	Steps      []string           `json:"steps"`
	Draft      *generationRequest `json:"draft,omitempty"`
	Generation *Generation        `json:"generation,omitempty"`
	Error      string             `json:"error,omitempty"`
}

type prepareGenerationArgs struct {
	ReferenceVideoFPS *float64 `json:"reference_video_fps,omitempty"`
	Mode              string   `json:"mode"`
	Prompt            string   `json:"prompt"`
	Duration          float64  `json:"duration"`
	AspectRatio       string   `json:"aspect_ratio"`
	Resolution        string   `json:"resolution"`
	Steps             int      `json:"steps"`
	AssetIDs          []string `json:"asset_ids"`
}

func agentTool(name, description string, properties map[string]any, required []string) responses.Tool {
	return responses.Tool{Type: "function", Name: name, Description: description, Strict: true,
		Parameters: map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}}
}

func agentTools() []responses.Tool {
	stringProp := map[string]any{"type": "string"}
	return []responses.Tool{
		agentTool("get_capabilities", "Read executable H3 video modes and parameter limits. Call before preparing video.", map[string]any{}, []string{}),
		agentTool("list_assets", "List the assets selected for this request. Only selected assets may be used.", map[string]any{}, []string{}),
		agentTool("load_skill", "Load a skill's complete SKILL.md instructions by name. Use h3-prompt-writing before preparing H3 video unless skills are disabled.", map[string]any{"name": stringProp}, []string{"name"}),
		agentTool("read_skill_resource", "Read a reference document inside a loaded skill, such as references/base-en.txt or references/ref-en.txt.", map[string]any{"name": stringProp, "resource": stringProp}, []string{"name", "resource"}),
		agentTool("prepare_generation", "Validate and save one H3 generation draft with the complete mode-specific prompt. This does not start video generation.", map[string]any{
			"mode": stringProp, "prompt": stringProp, "duration": map[string]any{"type": "number"}, "aspect_ratio": stringProp,
			"resolution": stringProp, "steps": map[string]any{"type": "integer"}, "asset_ids": map[string]any{"type": "array", "items": stringProp},
			"reference_video_fps": map[string]any{"type": []string{"number", "null"}, "description": "Reference decoding FPS: null defaults to 7, 1–60 fps or 0 for source rate. H3 output stays 24 fps."},
		}, []string{"mode", "prompt", "duration", "aspect_ratio", "resolution", "steps", "asset_ids", "reference_video_fps"}),
		agentTool("submit_generation", "Submit the validated draft to local ComfyUI only when execution_policy is auto. Repeated calls return the same task.", map[string]any{}, []string{}),
		agentTool("get_generation", "Read a generation task in the current project by ID.", map[string]any{"id": stringProp}, []string{"id"}),
	}
}

func (s *Server) agentTurnHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var input agentTurnInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}
	if input.ExecutionPolicy == "" {
		input.ExecutionPolicy = "plan_only"
	}
	if input.SkillPolicy == "" {
		input.SkillPolicy = "auto"
	}
	if (input.ExecutionPolicy != "plan_only" && input.ExecutionPolicy != "auto") || (input.SkillPolicy != "auto" && input.SkillPolicy != "none") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Agent policy"})
		return
	}
	if input.ExecutionPolicy == "auto" && !clientTurnIDPattern.MatchString(input.ClientTurnID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "自动执行需要 8–128 位 client_turn_id"})
		return
	}
	if input.ExecutionPolicy == "auto" {
		jobID, fingerprint := agentRequestIdentity(input)
		if previous, ok := s.job(jobID); ok {
			if previous.AgentRequestFingerprint != fingerprint {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "client_turn_id 已用于不同请求"})
				return
			}
			writeJSON(w, http.StatusOK, agentTurnResult{Skills: []string{}, Steps: []string{"返回已创建的视频任务"}, Generation: &previous})
			return
		}
	}
	if _, err := s.selectedAgentAssets(input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	var provider ProviderConfig
	if data, err := os.ReadFile(s.providerConfigPath()); err != nil || json.Unmarshal(data, &provider) != nil || provider.BaseURL == "" || provider.Model == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请先配置支持 /v1/responses 工具调用的 Agent 模型"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	result, err := s.runAgentTurn(ctx, input, provider, &responses.Client{})
	if err != nil {
		if result.Generation != nil {
			result.Error = err.Error()
			writeJSON(w, http.StatusOK, result)
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) selectedAgentAssets(input agentTurnInput) ([]Asset, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.validProjectLocked(input.ProjectID) {
		return nil, errors.New("项目不存在或已归档")
	}
	if len(input.AssetIDs) > 13 {
		return nil, errors.New("最多选择 13 个参考素材")
	}
	selected := make([]Asset, 0, len(input.AssetIDs))
	seen := map[string]bool{}
	for _, id := range input.AssetIDs {
		if seen[id] {
			return nil, errors.New("素材不能重复选择")
		}
		seen[id] = true
		found := false
		for _, asset := range s.assets {
			if asset.ID != id {
				continue
			}
			if asset.ProjectID != input.ProjectID {
				return nil, errors.New("只能引用当前项目的素材")
			}
			if _, err := s.assetPath(asset); err != nil {
				return nil, err
			}
			selected = append(selected, asset)
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf("素材不存在: %s", id)
		}
	}
	return selected, nil
}

func validateAgentDraft(draft generationRequest, selected []Asset) error {
	if strings.TrimSpace(draft.Prompt) == "" || draft.Duration < 1 || draft.Duration > 15 {
		return errors.New("提示词或时长无效")
	}
	if draft.AspectRatio != "16:9" && draft.AspectRatio != "9:16" && draft.AspectRatio != "1:1" && draft.AspectRatio != "4:3" {
		return errors.New("不支持的画面比例")
	}
	if draft.Resolution != "480p" && draft.Resolution != "720p" {
		return errors.New("当前 Agent 草稿只支持 480p 或 720p")
	}
	if draft.Steps < 1 || draft.Steps > 100 {
		return errors.New("步数必须在 1 到 100 之间")
	}
	fields := []string{"integrated_multimodal_description:", "overall_soundscape:", "non_diegetic_music:"}
	if draft.Mode == "multi_reference_video" {
		fields = []string{"subject_definitions:", "summary:", "retention_analysis:", "detailed_description:", "overall_soundscape:", "non_diegetic_music:"}
	}
	last := -1
	for _, field := range fields {
		at := strings.Index(draft.Prompt, field)
		if at <= last {
			return fmt.Errorf("H3 提示词缺少或乱序: %s", field)
		}
		last = at
	}
	available := map[string]Asset{}
	for _, asset := range selected {
		available[asset.ID] = asset
	}
	count := map[string]int{}
	seen := map[string]bool{}
	for _, id := range draft.AssetIDs {
		asset, ok := available[id]
		if !ok || seen[id] {
			return errors.New("草稿引用了未选择或重复的素材")
		}
		seen[id] = true
		count[asset.Type]++
	}
	switch draft.Mode {
	case "text_to_video":
		if len(draft.AssetIDs) != 0 {
			return errors.New("文生视频不能绑定参考素材")
		}
	case "image_to_video":
		if count["image"] != 1 || len(draft.AssetIDs) != 1 {
			return errors.New("图生视频需要一张图像")
		}
	case "first_last_frame":
		if count["image"] != 2 || len(draft.AssetIDs) != 2 {
			return errors.New("首尾帧需要两张有顺序的图像")
		}
	case "multi_reference_video":
		if len(draft.AssetIDs) == 0 || count["image"] > 9 || count["video"] > 3 || count["audio"] > 1 {
			return errors.New("多参考素材数量超出当前 H3 模板能力")
		}
	default:
		return errors.New("未知的 H3 生成模式")
	}
	return nil
}

func (s *Server) runAgentTurn(ctx context.Context, input agentTurnInput, provider ProviderConfig, client *responses.Client) (agentTurnResult, error) {
	selected, err := s.selectedAgentAssets(input)
	if err != nil {
		return agentTurnResult{}, err
	}
	result := agentTurnResult{Skills: []string{}, Steps: []string{}}
	loaded := map[string]bool{}
	outputs := map[string]string{}
	history := []any{map[string]any{"role": "user", "content": input.Prompt}}
	instructions := "You are Local Dream's creative assistant. Use tools for facts and actions. Only selected assets exist. Never claim a video is finished until get_generation reports completed. For H3 video creation, call get_capabilities, load h3-prompt-writing when skills are enabled, read its relevant reference, then prepare_generation with the full structured H3 prompt. Do not silently change user-specified model, resolution, dialogue, asset roles or duration. Ask in plain text when a material choice is missing. submit_generation is only available when execution_policy is auto; do not call it for plan_only."
	if input.SkillPolicy == "none" {
		instructions += " Skills are disabled for this request."
	} else {
		catalog, _ := json.Marshal(skills.List())
		instructions += " Available skills (name and description only): " + string(catalog)
	}
	for round := 0; round < 10; round++ {
		response, err := client.Create(ctx, provider.BaseURL, provider.APIKey, responses.Request{
			Model: provider.Model, Instructions: instructions, Input: history, Tools: agentTools(), ToolChoice: "auto", Store: false,
		})
		if err != nil {
			return result, err
		}
		for _, item := range response.Output {
			history = append(history, item)
		}
		calls, err := response.Calls()
		if err != nil {
			return result, err
		}
		if len(calls) == 0 {
			result.Text = response.Text()
			if result.Text == "" && result.Draft == nil && result.Generation == nil {
				return result, errors.New("Agent 返回了空结果")
			}
			return result, nil
		}
		for _, call := range calls {
			output, ok := outputs[call.CallID]
			if !ok {
				toolResult := s.runAgentTool(input, selected, &result, loaded, call)
				encoded, _ := json.Marshal(toolResult)
				output = string(encoded)
				outputs[call.CallID] = output
			}
			history = append(history, map[string]any{"type": "function_call_output", "call_id": call.CallID, "output": output})
		}
	}
	return result, errors.New("Agent 超过本轮工具调用上限")
}

func (s *Server) runAgentTool(input agentTurnInput, selected []Asset, result *agentTurnResult, loaded map[string]bool, call responses.FunctionCall) map[string]any {
	var args map[string]json.RawMessage
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return map[string]any{"ok": false, "error": "invalid tool arguments"}
	}
	readString := func(name string) string {
		var value string
		_ = json.Unmarshal(args[name], &value)
		return value
	}
	switch call.Name {
	case "get_capabilities":
		result.Steps = append(result.Steps, "读取 H3 生成能力")
		return map[string]any{"ok": true, "modes": []string{"text_to_video", "image_to_video", "first_last_frame", "multi_reference_video"}, "duration_seconds": []int{1, 15}, "resolutions": []string{"480p", "720p"}, "aspect_ratios": []string{"16:9", "9:16", "1:1", "4:3"}, "multi_reference_max": map[string]int{"images": 9, "videos": 3, "audios": 1}, "note": "These are current app limits; actual ComfyUI execution is checked at submission."}
	case "list_assets":
		assets := make([]map[string]string, 0, len(selected))
		for _, a := range selected {
			assets = append(assets, map[string]string{"id": a.ID, "name": a.Name, "type": a.Type})
		}
		return map[string]any{"ok": true, "assets": assets}
	case "load_skill":
		if input.SkillPolicy == "none" {
			return map[string]any{"ok": false, "error": "skills are disabled"}
		}
		name := readString("name")
		body, err := skills.Read(name, "SKILL.md")
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}
		}
		if !loaded[name] {
			result.Skills = append(result.Skills, name)
			result.Steps = append(result.Steps, "加载技能 "+name)
		}
		loaded[name] = true
		return map[string]any{"ok": true, "skill": name, "content": body}
	case "read_skill_resource":
		name := readString("name")
		if !loaded[name] {
			return map[string]any{"ok": false, "error": "load the skill first"}
		}
		body, err := skills.Read(name, readString("resource"))
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}
		}
		return map[string]any{"ok": true, "content": body}
	case "prepare_generation":
		if result.Generation != nil {
			return map[string]any{"ok": false, "error": "submitted generation cannot be edited in this turn"}
		}
		if input.SkillPolicy != "none" && !loaded["h3-prompt-writing"] {
			return map[string]any{"ok": false, "error": "load h3-prompt-writing before preparing H3 video"}
		}
		var proposed prepareGenerationArgs
		if err := json.Unmarshal([]byte(call.Arguments), &proposed); err != nil {
			return map[string]any{"ok": false, "error": "invalid generation draft"}
		}
		draft := generationRequest{ProjectID: input.ProjectID, Strategy: "normal", Mode: proposed.Mode, Prompt: proposed.Prompt,
			Duration: proposed.Duration, AspectRatio: proposed.AspectRatio, Resolution: proposed.Resolution, Steps: proposed.Steps, AssetIDs: proposed.AssetIDs}
		draft.ReferenceVideoFPS = proposed.ReferenceVideoFPS
		if err := normalizeReferenceVideoFPS(&draft); err != nil {
			return map[string]any{"ok": false, "error": err.Error()}
		}
		if input.ExecutionPolicy == "auto" {
			draft.AgentJobID, draft.AgentFingerprint = agentRequestIdentity(input)
		}
		if err := validateAgentDraft(draft, selected); err != nil {
			return map[string]any{"ok": false, "error": err.Error()}
		}
		result.Draft = &draft
		result.Steps = append(result.Steps, "准备并校验 H3 视频草稿")
		return map[string]any{"ok": true, "draft": draft, "status": "prepared_not_submitted"}
	case "submit_generation":
		if input.ExecutionPolicy != "auto" {
			return map[string]any{"ok": false, "error": "this request only authorizes planning"}
		}
		if result.Draft == nil {
			return map[string]any{"ok": false, "error": "prepare a valid draft first"}
		}
		if result.Generation != nil {
			return map[string]any{"ok": true, "generation_id": result.Generation.ID, "status": result.Generation.Status}
		}
		if _, err := s.selectedAgentAssets(input); err != nil {
			return map[string]any{"ok": false, "error": err.Error()}
		}
		generation, _, err := s.createGeneration(*result.Draft)
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}
		}
		result.Generation = &generation
		result.Steps = append(result.Steps, "已创建本地视频任务 "+generation.ID)
		return map[string]any{"ok": true, "generation_id": generation.ID, "status": generation.Status, "actual_mode": generation.Mode, "resolution": generation.Resolution}
	case "get_generation":
		generation, ok := s.job(readString("id"))
		if !ok || generation.ProjectID != input.ProjectID {
			return map[string]any{"ok": false, "error": "generation not found in current project"}
		}
		return map[string]any{"ok": true, "generation": publicGeneration(generation)}
	default:
		return map[string]any{"ok": false, "error": "unknown tool"}
	}
}
