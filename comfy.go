package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type workflowManifest struct {
	Modes map[string]workflowMode `json:"modes"`
}

type workflowMode struct {
	GroupID        int                             `json:"group_id"`
	GroupIndex     int                             `json:"group_index"`
	PromptNode     int                             `json:"prompt_node"`
	DurationNode   int                             `json:"duration_node"`
	ResolutionNode int                             `json:"resolution_node"`
	UNetNode       int                             `json:"unet_node"`
	UNetModel      string                          `json:"unet_model"`
	AssetNodes     []int                           `json:"asset_nodes"`
	Accelerations  map[string]workflowAcceleration `json:"accelerations"`
}

type workflowAcceleration struct {
	Label       string `json:"label"`
	LoraName    string `json:"lora_name"`
	Steps       int    `json:"steps"`
	TESpeedMode string `json:"te_speed_mode"`
}

type canvasWorkflow struct {
	Nodes  []canvasNode  `json:"nodes"`
	Groups []canvasGroup `json:"groups"`
	Links  [][]any       `json:"links"`
}

type canvasNode struct {
	ID           int           `json:"id"`
	Type         string        `json:"type"`
	Title        string        `json:"title"`
	Pos          []float64     `json:"pos"`
	Inputs       []canvasInput `json:"inputs"`
	WidgetValues []any         `json:"widgets_values"`
}

type canvasInput struct {
	Name   string `json:"name"`
	Link   any    `json:"link"`
	Widget any    `json:"widget"`
}

type canvasGroup struct {
	ID       int       `json:"id"`
	Bounding []float64 `json:"bounding"`
}

type comfyPrompt struct {
	Prompt   map[string]comfyNode `json:"prompt"`
	ClientID string               `json:"client_id"`
}

type comfyNode struct {
	ClassType string         `json:"class_type"`
	Inputs    map[string]any `json:"inputs"`
	Meta      map[string]any `json:"_meta,omitempty"`
}

type compiledAsset struct {
	ID       string
	Filename string
	Kind     string
}

func (s *Server) compilePrompt(payload generationRequest) (comfyPrompt, error) {
	workflowPath := filepath.Join(s.root, "templates", "h3", "workflow.json")
	manifestPath := filepath.Join(s.root, "templates", "h3", "manifest.json")

	var doc canvasWorkflow
	if err := readJSON(workflowPath, &doc); err != nil {
		return comfyPrompt{}, fmt.Errorf("read H3 workflow: %w", err)
	}
	var manifest workflowManifest
	if err := readJSON(manifestPath, &manifest); err != nil {
		return comfyPrompt{}, fmt.Errorf("read H3 manifest: %w", err)
	}
	mode, ok := manifest.Modes[payload.Mode]
	if !ok {
		return comfyPrompt{}, fmt.Errorf("unsupported H3 mode: %s", payload.Mode)
	}
	if mode.Accelerations == nil {
		mode.Accelerations = map[string]workflowAcceleration{}
	}

	group := findGroup(doc.Groups, mode)
	if group == nil {
		return comfyPrompt{}, fmt.Errorf("H3 workflow has no group for mode: %s", payload.Mode)
	}
	nodes := make(map[string]canvasNode, len(doc.Nodes))
	for _, node := range doc.Nodes {
		nodes[strconv.Itoa(node.ID)] = node
	}
	links := make(map[string][]any, len(doc.Links))
	for _, link := range doc.Links {
		if len(link) > 2 {
			links[asString(link[0])] = link
		}
	}
	selected := map[string]bool{}
	for id, node := range nodes {
		if !ignoredCanvasNode(node.Type) && nodeInsideGroup(node, *group) {
			selected[id] = true
		}
	}
	if len(selected) == 0 {
		return comfyPrompt{}, fmt.Errorf("H3 workflow group has no executable nodes: %s", payload.Mode)
	}
	for changed := true; changed; {
		changed = false
		for id := range selected {
			for _, input := range nodes[id].Inputs {
				if link, ok := links[asString(input.Link)]; ok && len(link) > 1 {
					source := asString(link[1])
					if node, exists := nodes[source]; exists && !selected[source] && !ignoredCanvasNode(node.Type) {
						selected[source] = true
						changed = true
					}
				}
			}
		}
	}

	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return atoi(ids[i]) < atoi(ids[j]) })
	graph := make(map[string]comfyNode, len(ids))
	for _, id := range ids {
		node := nodes[id]
		inputs, err := s.compileNodeInputs(node, links)
		if err != nil {
			return comfyPrompt{}, fmt.Errorf("compile node %s (%s): %w", id, node.Type, err)
		}
		graph[id] = comfyNode{ClassType: node.Type, Inputs: inputs, Meta: map[string]any{"title": fallback(node.Title, node.Type)}}
	}
	if err := s.normalizeModelInputs(graph); err != nil {
		return comfyPrompt{}, err
	}

	promptID := strconv.Itoa(mode.PromptNode)
	if node, ok := graph[promptID]; ok {
		node.Inputs["prompt"] = payload.Prompt
		graph[promptID] = node
	} else {
		return comfyPrompt{}, fmt.Errorf("H3 prompt node is missing: %d", mode.PromptNode)
	}
	if mode.UNetModel != "" {
		if node, ok := graph[strconv.Itoa(mode.UNetNode)]; ok {
			node.Inputs["unet_name"] = mode.UNetModel
			node.Inputs["weight_dtype"] = "default"
			graph[strconv.Itoa(mode.UNetNode)] = node
		}
	}
	if node, ok := graph[strconv.Itoa(mode.DurationNode)]; ok {
		node.Inputs["value"] = payload.Duration
		graph[strconv.Itoa(mode.DurationNode)] = node
	}
	if node, ok := graph[strconv.Itoa(mode.ResolutionNode)]; ok {
		aspect, megapixels := resolutionInputs(payload.AspectRatio, payload.Resolution)
		node.Inputs["aspect_ratio"] = aspect
		node.Inputs["megapixels"] = megapixels
		node.Inputs["multiple"] = 32
		graph[strconv.Itoa(mode.ResolutionNode)] = node
	}
	if err := applyAcceleration(graph, mode, payload); err != nil {
		return comfyPrompt{}, err
	}
	assets, err := s.copyAssets(payload.AssetIDs)
	if err != nil {
		return comfyPrompt{}, err
	}
	if err := attachAssets(graph, mode, payload.Mode, assets); err != nil {
		return comfyPrompt{}, err
	}
	return comfyPrompt{Prompt: graph, ClientID: "localdream-" + newID()}, nil
}

func (s *Server) compileNodeInputs(node canvasNode, links map[string][]any) (map[string]any, error) {
	inputs := map[string]any{}
	for _, input := range node.Inputs {
		if link, ok := links[asString(input.Link)]; ok && len(link) > 2 {
			inputs[input.Name] = []any{asString(link[1]), int(asFloat(link[2]))}
		}
	}
	widgets := append([]any(nil), node.WidgetValues...)
	for _, input := range node.Inputs {
		if input.Widget != nil && len(widgets) > 0 {
			value := widgets[0]
			widgets = widgets[1:]
			if _, linked := inputs[input.Name]; !linked {
				inputs[input.Name] = unwrapWidget(value)
			}
		}
	}
	required, err := s.requiredInputs(node.Type)
	if err != nil {
		return nil, err
	}
	for _, name := range required {
		if _, exists := inputs[name]; exists || len(widgets) == 0 {
			continue
		}
		inputs[name] = unwrapWidget(widgets[0])
		widgets = widgets[1:]
	}
	return inputs, nil
}

func (s *Server) requiredInputs(classType string) ([]string, error) {
	var objectInfo map[string]struct {
		Input struct {
			Required map[string]any `json:"required"`
		} `json:"input"`
		InputOrder struct {
			Required []string `json:"required"`
		} `json:"input_order"`
	}
	if err := s.comfyJSON(http.MethodGet, "/object_info/"+classType, nil, &objectInfo); err != nil {
		return nil, err
	}
	entry, ok := objectInfo[classType]
	if !ok {
		return nil, fmt.Errorf("ComfyUI has no object info for %s", classType)
	}
	if len(entry.InputOrder.Required) > 0 {
		return entry.InputOrder.Required, nil
	}
	keys := make([]string, 0, len(entry.Input.Required))
	for key := range entry.Input.Required {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *Server) normalizeModelInputs(graph map[string]comfyNode) error {
	for id, node := range graph {
		if node.ClassType != "CLIPLoader" {
			continue
		}
		clipName, _ := node.Inputs["clip_name"].(string)
		allowed, err := s.allowedValues("CLIPLoader", "clip_name")
		if err != nil {
			return err
		}
		if contains(allowed, clipName) {
			continue
		}
		candidate := preferredMiniMaxTextEncoder(allowed)
		if candidate == "" {
			return fmt.Errorf("node %s requires a CLIP text encoder, but the template model %q is unavailable and no MiniMax H3 compatible replacement is installed", id, clipName)
		}
		node.Inputs["clip_name"] = candidate
		graph[id] = node
	}
	return nil
}

func (s *Server) allowedValues(classType, inputName string) ([]string, error) {
	var objectInfo map[string]struct {
		Input struct {
			Required map[string][]any `json:"required"`
		} `json:"input"`
	}
	if err := s.comfyJSON(http.MethodGet, "/object_info/"+classType, nil, &objectInfo); err != nil {
		return nil, err
	}
	input := objectInfo[classType].Input.Required[inputName]
	if len(input) == 0 {
		return nil, fmt.Errorf("ComfyUI has no required input %s.%s", classType, inputName)
	}
	values, ok := input[0].([]any)
	if !ok {
		return nil, fmt.Errorf("ComfyUI input %s.%s is not a selectable model list", classType, inputName)
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, asString(value))
	}
	return result, nil
}

func preferredMiniMaxTextEncoder(candidates []string) string {
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), "minimax_h3") {
			return candidate
		}
	}
	for _, candidate := range candidates {
		if strings.Contains(strings.ToLower(candidate), "h3") {
			return candidate
		}
	}
	return ""
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (s *Server) copyAssets(assetIDs []string) ([]compiledAsset, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}
	if err := os.MkdirAll(s.comfyInput, 0755); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.root, "data", "assets"))
	if err != nil {
		return nil, fmt.Errorf("read local assets: %w", err)
	}
	assets := make([]compiledAsset, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		prefix := assetID + "."
		filename := ""
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
				filename = entry.Name()
				break
			}
		}
		if filename == "" {
			return nil, fmt.Errorf("asset not found: %s", assetID)
		}
		ext := strings.ToLower(filepath.Ext(filename))
		kind := ""
		if imageExtensions[ext] {
			kind = "image"
		} else if videoExtensions[ext] {
			kind = "video"
		} else {
			return nil, fmt.Errorf("unsupported asset type: %s", filename)
		}
		if err := copyFile(filepath.Join(s.root, "data", "assets", filename), filepath.Join(s.comfyInput, filename)); err != nil {
			return nil, err
		}
		assets = append(assets, compiledAsset{ID: assetID, Filename: filename, Kind: kind})
	}
	return assets, nil
}

func attachAssets(graph map[string]comfyNode, mode workflowMode, modeName string, assets []compiledAsset) error {
	images, videos := []string{}, []string{}
	for _, asset := range assets {
		if asset.Kind == "image" {
			images = append(images, asset.Filename)
		} else {
			videos = append(videos, asset.Filename)
		}
	}
	switch modeName {
	case "text_to_video":
		if len(assets) > 0 {
			return fmt.Errorf("text-to-video does not accept reference assets")
		}
		return nil
	case "image_to_video":
		if len(videos) > 0 || len(images) != 1 {
			return fmt.Errorf("image-to-video requires exactly one image")
		}
	case "first_last_frame":
		if len(videos) > 0 || len(images) != 2 {
			return fmt.Errorf("first-last-frame requires exactly two images")
		}
	case "multi_reference_video":
		if len(assets) == 0 || len(images) > 9 || len(videos) > 3 {
			return fmt.Errorf("multi-reference requires 1-9 images and up to 3 videos")
		}
	default:
		return fmt.Errorf("unknown mode: %s", modeName)
	}
	if modeName != "multi_reference_video" {
		for index, nodeID := range mode.AssetNodes {
			if index >= len(images) {
				break
			}
			node, ok := graph[strconv.Itoa(nodeID)]
			if !ok || node.ClassType != "LoadImage" {
				return fmt.Errorf("workflow image input node is missing: %d", nodeID)
			}
			node.Inputs["image"] = images[index]
			graph[strconv.Itoa(nodeID)] = node
		}
		return nil
	}
	targetID := strconv.Itoa(mode.PromptNode)
	target, ok := graph[targetID]
	if !ok {
		return fmt.Errorf("workflow prompt node is missing: %d", mode.PromptNode)
	}
	for key := range target.Inputs {
		if strings.HasPrefix(key, "ref_images.") || strings.HasPrefix(key, "ref_videos.") || strings.HasPrefix(key, "ref_video_audios.") || strings.HasPrefix(key, "ref_audios.") {
			delete(target.Inputs, key)
		}
	}
	nextID := maxNodeID(graph) + 1
	for index, filename := range images {
		nodeID := strconv.Itoa(nextID)
		nextID++
		graph[nodeID] = comfyNode{ClassType: "LoadImage", Inputs: map[string]any{"image": filename}, Meta: map[string]any{"title": "dynamic reference image"}}
		target.Inputs[fmt.Sprintf("ref_images.ref_image_%d", index)] = []any{nodeID, 0}
	}
	for index, filename := range videos {
		nodeID := strconv.Itoa(nextID)
		nextID++
		graph[nodeID] = comfyNode{ClassType: "VHS_LoadVideo", Inputs: map[string]any{"video": filename, "force_rate": 0, "custom_width": 0, "custom_height": 0, "frame_load_cap": 0, "skip_first_frames": 0, "select_every_nth": 1}, Meta: map[string]any{"title": "dynamic reference video"}}
		target.Inputs[fmt.Sprintf("ref_videos.ref_video_%d", index)] = []any{nodeID, 0}
	}
	graph[targetID] = target
	return nil
}

func applyAcceleration(graph map[string]comfyNode, mode workflowMode, payload generationRequest) error {
	selection := strings.TrimSpace(payload.Lora)
	option, ok := mode.Accelerations[selection]
	if selection != "" && !ok {
		return fmt.Errorf("the selected mode does not support acceleration: %s", selection)
	}
	steps := payload.Steps
	if steps < 1 {
		steps = 4
	}
	if steps > 100 {
		steps = 100
	}
	if selection != "" && steps != option.Steps {
		return fmt.Errorf("%s requires %d steps", option.Label, option.Steps)
	}
	bypass := map[string]any{}
	for id, node := range graph {
		switch node.ClassType {
		case "BasicScheduler":
			node.Inputs["scheduler"] = "simple"
			node.Inputs["steps"] = steps
			node.Inputs["denoise"] = 1
			graph[id] = node
		case "TESpeedMiniMaxH3":
			node.Inputs["mode"] = option.TESpeedMode
			if selection == "" {
				node.Inputs["mode"] = "standard"
			}
			graph[id] = node
		case "LoraLoaderModelOnly":
			if selection != "" {
				node.Inputs["lora_name"] = option.LoraName
				node.Inputs["strength_model"] = clamp(payload.LoraStrength, 0, 1)
				graph[id] = node
			} else if model, linked := node.Inputs["model"].([]any); linked && len(model) > 1 {
				bypass[id] = model
			}
		}
	}
	for id, node := range graph {
		for key, value := range node.Inputs {
			if linked, ok := value.([]any); ok && len(linked) > 1 {
				if replacement, exists := bypass[asString(linked[0])]; exists {
					node.Inputs[key] = replacement
				}
			}
		}
		graph[id] = node
	}
	for id := range bypass {
		delete(graph, id)
	}
	return nil
}

func (s *Server) comfyJSON(method, path string, body any, output any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequest(method, strings.TrimRight(s.comfyURL, "/")+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("ComfyUI returned %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	if output == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(output)
}

func (s *Server) comfyResult(promptID string) (message string, done bool, failed bool, outputURL string, err error) {
	var history map[string]struct {
		Status struct {
			Status   string `json:"status_str"`
			Messages any    `json:"messages"`
		} `json:"status"`
		Outputs map[string]struct {
			Gifs   []comfyOutput `json:"gifs"`
			Videos []comfyOutput `json:"videos"`
			Images []comfyOutput `json:"images"`
		} `json:"outputs"`
	}
	if err = s.comfyJSON(http.MethodGet, "/history/"+promptID, nil, &history); err != nil {
		return "", false, false, "", err
	}
	item, exists := history[promptID]
	if !exists {
		return "", false, false, "", nil
	}
	if item.Status.Status == "error" {
		return fmt.Sprint(item.Status.Messages), false, true, "", nil
	}
	for _, nodeOutput := range item.Outputs {
		outputs := append(append(nodeOutput.Gifs, nodeOutput.Videos...), nodeOutput.Images...)
		for _, output := range outputs {
			if videoExtensions[strings.ToLower(filepath.Ext(output.Filename))] {
				return "", true, false, s.comfyViewURL(output), nil
			}
		}
	}
	if item.Outputs != nil {
		return "", true, false, "", nil
	}
	return "", false, false, "", nil
}

type comfyOutput struct {
	Filename  string `json:"filename"`
	Subfolder string `json:"subfolder"`
	Type      string `json:"type"`
}

func (s *Server) comfyViewURL(output comfyOutput) string {
	values := make(url.Values)
	values.Set("filename", output.Filename)
	values.Set("subfolder", output.Subfolder)
	values.Set("type", fallback(output.Type, "output"))
	return strings.TrimRight(s.comfyURL, "/") + "/view?" + values.Encode()
}

func readJSON(path string, output any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, output)
}

func findGroup(groups []canvasGroup, mode workflowMode) *canvasGroup {
	for index := range groups {
		if groups[index].ID == mode.GroupID {
			return &groups[index]
		}
	}
	if mode.GroupIndex >= 0 && mode.GroupIndex < len(groups) {
		return &groups[mode.GroupIndex]
	}
	return nil
}

func nodeInsideGroup(node canvasNode, group canvasGroup) bool {
	if len(node.Pos) < 2 || len(group.Bounding) < 4 {
		return false
	}
	x, y := node.Pos[0], node.Pos[1]
	gx, gy, width, height := group.Bounding[0], group.Bounding[1], group.Bounding[2], group.Bounding[3]
	return gx <= x && x <= gx+width && gy <= y && y <= gy+height
}

func ignoredCanvasNode(classType string) bool {
	return classType == "Note" || classType == "MarkdownNote" || classType == "Label (rgthree)"
}

func unwrapWidget(value any) any {
	if values, ok := value.([]any); ok && len(values) == 1 {
		return values[0]
	}
	return value
}

func resolutionInputs(ratio, resolution string) (string, float64) {
	ratios := map[string]string{"16:9": "16:9 (Widescreen)", "9:16": "9:16 (Portrait Widescreen)", "1:1": "1:1 (Square)", "4:3": "4:3 (Standard)"}
	megapixels := 0.9
	if resolution == "480p" {
		megapixels = 0.4
	}
	return fallback(ratios[ratio], "16:9 (Widescreen)"), megapixels
}

func maxNodeID(graph map[string]comfyNode) int {
	maxID := 0
	for id := range graph {
		if parsed := atoi(id); parsed > maxID {
			maxID = parsed
		}
	}
	return maxID
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(destination)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func asString(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case float64:
		return strconv.Itoa(int(value))
	case int:
		return strconv.Itoa(value)
	default:
		return ""
	}
}

func asFloat(value any) float64 {
	switch value := value.(type) {
	case float64:
		return value
	case int:
		return float64(value)
	case json.Number:
		parsed, _ := value.Float64()
		return parsed
	default:
		return 0
	}
}

func atoi(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func fallback(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func clamp(value, minimum, maximum float64) float64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func newID() string {
	return strconv.FormatInt(timeNow().UnixNano(), 36)
}

var timeNow = func() time.Time { return time.Now() }

var imageExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".bmp": true, ".gif": true}
var videoExtensions = map[string]bool{".mp4": true, ".mov": true, ".mkv": true, ".webm": true, ".avi": true, ".m4v": true}
