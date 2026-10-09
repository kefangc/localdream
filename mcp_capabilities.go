package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	comfyclient "localdream/internal/comfy"
	"localdream/internal/workflow"
)

type mcpModeCapability struct {
	Mode                  string                   `json:"mode"`
	Assets                workflow.AssetCapability `json:"assets"`
	PromptSections        []string                 `json:"prompt_sections"`
	RuntimeStatus         string                   `json:"runtime_status"`
	ReferenceLimitsSource string                   `json:"reference_limits_source"`
	Warnings              []string                 `json:"warnings"`
}
type mcpCapabilitiesOutput struct {
	Modes                        []mcpModeCapability `json:"modes"`
	DurationSeconds              [2]float64          `json:"duration_seconds"`
	DurationBehavior             string              `json:"duration_behavior"`
	Resolutions                  []string            `json:"resolutions"`
	CustomResolutionMPRange      [2]float64          `json:"custom_resolution_mp_range"`
	CustomResolutionFormat       string              `json:"custom_resolution_format"`
	AspectRatios                 []string            `json:"aspect_ratios"`
	Steps                        [2]int              `json:"steps"`
	DefaultSteps                 int                 `json:"default_steps"`
	DefaultReferenceVideoFPS     float64             `json:"default_reference_video_fps"`
	ReferenceVideoFPSRange       [2]float64          `json:"reference_video_fps_range"`
	ReferenceVideoSourceFPSValue int                 `json:"reference_video_source_fps_value"`
	OutputFPS                    int                 `json:"output_fps"`
	CheckedAt                    time.Time           `json:"checked_at"`
	Limitations                  []string            `json:"limitations"`
}

func (s *Server) mcpCapabilities(ctx context.Context, _ emptyToolInput) (mcpCapabilitiesOutput, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out := mcpCapabilitiesOutput{
		Modes: []mcpModeCapability{}, DurationSeconds: [2]float64{1, 15}, Resolutions: []string{"480p", "720p"}, AspectRatios: []string{"16:9", "9:16", "1:1", "4:3"},
		Steps: [2]int{1, 100}, DefaultSteps: 50, DefaultReferenceVideoFPS: 7, ReferenceVideoFPSRange: [2]float64{1, 60}, OutputFPS: 24, CheckedAt: time.Now().UTC(),
		DurationBehavior:        "Pass the user's requested duration unchanged. The bundled ordinary template rounds duration*24 to frames, then aligns upward to 17n+5. For 7 seconds this is 175/24 = 7.2917 seconds. This is frame alignment, not long-video continuation padding. Custom templates may differ.",
		CustomResolutionMPRange: [2]float64{0.1, 1.9}, CustomResolutionFormat: "<decimal>MP, e.g. 0.75MP or 1.2MP; decimal precision is preserved. Resolutions lists only presets. Actual pixel dimensions are aligned by the workflow.",
		Limitations: []string{
			"Runtime probe checks only prompt nodes and reference slots. Full graph, model files and GPU execution are validated by the existing submission worker.",
			"MCP v1 covers ordinary H3 generation; long-video orchestration, LoRA selection and media upload are not exposed.",
			"Reference video audio is not automatically reused. Add an explicit audio asset when needed.",
			"Duration 1–15 seconds is the application validator range; the template's authoring recommendation is 5–15 seconds.",
		},
	}
	base := []string{"integrated_multimodal_description:", "overall_soundscape:", "non_diegetic_music:"}
	ref := []string{"subject_definitions:", "summary:", "retention_analysis:", "detailed_description:", "overall_soundscape:", "non_diegetic_music:"}
	modes := []mcpModeCapability{
		{Mode: "text_to_video", PromptSections: base},
		{Mode: "image_to_video", Assets: workflow.AssetCapability{Images: workflow.CountRange{Min: 1, Max: 1}, MinTotal: 1}, PromptSections: base},
		{Mode: "first_last_frame", Assets: workflow.AssetCapability{Images: workflow.CountRange{Min: 2, Max: 2}, MinTotal: 2}, PromptSections: base},
		{Mode: "multi_reference_video", Assets: workflow.AssetCapability{Images: workflow.CountRange{Max: 9}, Videos: workflow.CountRange{Max: 3}, Audios: workflow.CountRange{Max: 1}, MinTotal: 1}, PromptSections: ref},
	}
	root := env("H3_TEMPLATE_DIR", filepath.Join(s.root, "templates", "h3"))
	var manifest workflowManifest
	var doc canvasWorkflow
	manifestErr := readJSON(filepath.Join(root, "manifest.json"), &manifest)
	docErr := readJSON(filepath.Join(root, "workflow.json"), &doc)
	client := comfyclient.NewClient(s.comfyURL, s.httpClient)
	type probe struct {
		raw []byte
		err error
	}
	cache := map[string]probe{}
	for _, mode := range modes {
		mode.Warnings = []string{}
		mode.ReferenceLimitsSource = "application"
		mode.RuntimeStatus = "unknown"
		definition, exists := manifest.Modes[mode.Mode]
		if manifestErr != nil || docErr != nil || !exists {
			mode.RuntimeStatus = "unavailable"
			mode.Warnings = append(mode.Warnings, "Local workflow template or mode definition is unavailable.")
			out.Modes = append(out.Modes, mode)
			continue
		}
		nodeType := ""
		for _, node := range doc.Nodes {
			if node.ID == definition.PromptNode {
				nodeType = node.Type
				break
			}
		}
		if nodeType == "" {
			mode.RuntimeStatus = "unavailable"
			mode.Warnings = append(mode.Warnings, "Template prompt node is missing.")
			out.Modes = append(out.Modes, mode)
			continue
		}
		p, ok := cache[nodeType]
		if !ok {
			p.raw, p.err = client.ObjectInfo(ctx, nodeType)
			cache[nodeType] = p
		}
		if p.err != nil {
			mode.Warnings = append(mode.Warnings, "ComfyUI probe failed or timed out. Application limits are a fallback, not evidence that generation is available.")
		} else {
			var info map[string]json.RawMessage
			if json.Unmarshal(p.raw, &info) != nil || len(info[nodeType]) == 0 || string(info[nodeType]) == "null" {
				mode.RuntimeStatus = "unavailable"
				mode.Warnings = append(mode.Warnings, "Required prompt node is absent from ComfyUI object_info.")
			} else {
				mode.RuntimeStatus = "prompt_node_available"
				if mode.Mode == "multi_reference_video" {
					slots, err := workflow.ParseRuntimeAssetSlots(p.raw, nodeType)
					if err != nil {
						mode.Warnings = append(mode.Warnings, "Runtime reference limits could not be parsed; using application limits.")
					} else {
						mode.ReferenceLimitsSource = "application_intersect_runtime"
						mode.Assets.Images = intersectAssetRange(mode.Assets.Images, slots.ReferenceImages)
						mode.Assets.Videos = intersectAssetRange(mode.Assets.Videos, slots.ReferenceVideos)
						mode.Assets.Audios = intersectAssetRange(mode.Assets.Audios, slots.ReferenceAudios)
						if mode.Assets.Images.Min > mode.Assets.Images.Max || mode.Assets.Videos.Min > mode.Assets.Videos.Max || mode.Assets.Audios.Min > mode.Assets.Audios.Max || mode.Assets.Images.Max+mode.Assets.Videos.Max+mode.Assets.Audios.Max < mode.Assets.MinTotal {
							mode.RuntimeStatus = "unavailable"
							mode.Warnings = append(mode.Warnings, "Runtime reference limits are incompatible with this compiler.")
						}
					}
				}
			}
		}
		out.Modes = append(out.Modes, mode)
	}
	return out, nil
}

func intersectAssetRange(application, runtime workflow.CountRange) workflow.CountRange {
	return workflow.CountRange{Min: max(application.Min, runtime.Min), Max: min(application.Max, runtime.Max)}
}
