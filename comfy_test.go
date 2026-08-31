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
		})
	}
}
