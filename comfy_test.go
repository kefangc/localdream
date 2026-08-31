package main

import (
	"net/http"
	"os"
	"testing"
	"time"
)

func TestResolutionInputs(t *testing.T) {
	aspect, megapixels := resolutionInputs("9:16", "480p")
	if aspect != "9:16 (Portrait Widescreen)" || megapixels != 0.4 {
		t.Fatalf("unexpected 480p portrait resolution: %q, %v", aspect, megapixels)
	}
	_, megapixels = resolutionInputs("unknown", "720p")
	if megapixels != 0.9 {
		t.Fatalf("unexpected default megapixels: %v", megapixels)
	}
}

func TestParseAgentJSON(t *testing.T) {
	for _, response := range []string{
		`{"mode":"text_to_video","prompt":{"subject":"kite"}}`,
		"```json\n{\"mode\":\"text_to_video\",\"prompt\":{\"subject\":\"kite\"}}\n```",
		"Here is the structured plan:\n{\"mode\":\"text_to_video\",\"prompt\":{\"subject\":\"kite\"}}",
	} {
		intent, err := parseAgentJSON(response)
		if err != nil {
			t.Fatal(err)
		}
		if intent.(map[string]any)["mode"] != "text_to_video" {
			t.Fatalf("unexpected Agent result: %#v", intent)
		}
	}
	if _, err := parseAgentJSON("not JSON"); err == nil {
		t.Fatal("expected invalid Agent text to fail")
	}
}

func TestPreferredMiniMaxTextEncoder(t *testing.T) {
	candidate := preferredMiniMaxTextEncoder([]string{"qwen3vl_32b_h3_generation_tail_50_63_int8_convrot.safetensors", "qwen3vl_32b_minimax_h3_nvfp4_awq.safetensors"})
	if candidate != "qwen3vl_32b_minimax_h3_nvfp4_awq.safetensors" {
		t.Fatalf("unexpected candidate: %s", candidate)
	}
}

func TestCompileTextToVideoAgainstComfyUI(t *testing.T) {
	if os.Getenv("COMFYUI_INTEGRATION") != "1" {
		t.Skip("set COMFYUI_INTEGRATION=1 to validate the local ComfyUI workflow compiler")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		root:       root,
		comfyURL:   env("COMFYUI_URL", "http://127.0.0.1:8188"),
		comfyInput: env("COMFYUI_INPUT", `D:\Comfy-Desktop\ComfyUI-Shared\input`),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	compiled, err := s.compilePrompt(generationRequest{
		Mode:        "text_to_video",
		Prompt:      "A paper kite moves through a quiet blue sky.",
		Duration:    5,
		AspectRatio: "16:9",
		Resolution:  "480p",
		Steps:       4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Prompt) == 0 || compiled.Prompt["194"].Inputs["prompt"] == "" {
		t.Fatal("compiled prompt has no configured text-to-video prompt node")
	}
}

func TestCompileAllModesAgainstComfyUI(t *testing.T) {
	if os.Getenv("COMFYUI_INTEGRATION") != "1" {
		t.Skip("set COMFYUI_INTEGRATION=1 to validate the local ComfyUI workflow compiler")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		root:       root,
		comfyURL:   env("COMFYUI_URL", "http://127.0.0.1:8188"),
		comfyInput: env("COMFYUI_INPUT", `D:\Comfy-Desktop\ComfyUI-Shared\input`),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	tests := []struct {
		name    string
		request generationRequest
	}{
		{
			name: "text-to-video",
			request: generationRequest{
				Mode: "text_to_video", Prompt: "A paper kite moves through a quiet blue sky.", Duration: 5, AspectRatio: "16:9", Resolution: "480p", Steps: 4,
			},
		},
		{
			name: "image-to-video",
			request: generationRequest{
				Mode: "image_to_video", Prompt: "The subject looks toward the camera.", Duration: 5, AspectRatio: "16:9", Resolution: "480p", Steps: 4, AssetIDs: []string{"asset_1"},
			},
		},
		{
			name: "first-last-frame",
			request: generationRequest{
				Mode: "first_last_frame", Prompt: "A smooth camera movement between the two frames.", Duration: 5, AspectRatio: "16:9", Resolution: "480p", Steps: 4, AssetIDs: []string{"asset_1", "asset_2"},
			},
		},
		{
			name: "multi-reference",
			request: generationRequest{
				Mode: "multi_reference_video", Prompt: "Use the reference image as the visual subject.", Duration: 5, AspectRatio: "16:9", Resolution: "480p", Steps: 4, AssetIDs: []string{"asset_1"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := s.compilePrompt(test.request)
			if err != nil {
				t.Fatal(err)
			}
			if len(compiled.Prompt) == 0 {
				t.Fatal("compiled workflow is empty")
			}
			if node, exists := compiled.Prompt["369"]; exists {
				if node.Inputs["processing_control_value"] != 0.08 || node.Inputs["processing_percent_1"] != 0.1 || node.Inputs["processing_percent_2"] != 0.9 || node.Inputs["mcs"] != float64(2) || node.Inputs["device"] != "auto" {
					t.Fatalf("TESpeedMiniMaxH3 widget values are mapped incorrectly: %#v", node.Inputs)
				}
			}
		})
	}
}
