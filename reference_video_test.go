package main

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fpsValue(v float64) *float64 { return &v }

func TestReferenceVideoFPSValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      *float64
		want    float64
		invalid bool
	}{
		{"missing", nil, 7, false}, {"source", fpsValue(0), 0, false},
		{"minimum", fpsValue(1), 1, false}, {"fractional", fpsValue(7.5), 7.5, false},
		{"maximum", fpsValue(60), 60, false}, {"negative", fpsValue(-1), 0, true},
		{"below minimum", fpsValue(0.5), 0, true}, {"over maximum", fpsValue(60.1), 0, true},
		{"nan", fpsValue(math.NaN()), 0, true}, {"infinity", fpsValue(math.Inf(1)), 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := generationRequest{ReferenceVideoFPS: tc.in}
			err := normalizeReferenceVideoFPS(&in)
			if (err != nil) != tc.invalid {
				t.Fatalf("validation: %v", err)
			}
			if !tc.invalid && (in.ReferenceVideoFPS == nil || *in.ReferenceVideoFPS != tc.want) {
				t.Fatalf("got %v, want %v", in.ReferenceVideoFPS, tc.want)
			}
		})
	}
}

func TestReferenceVideoFPSAPIAndPersistence(t *testing.T) {
	s := sequenceTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // exercise HTTP and persistence without submitting any inference
	s.ctx = ctx
	for _, fps := range []*float64{nil, fpsValue(0), fpsValue(7), fpsValue(12.5), fpsValue(60)} {
		in := generationRequest{Mode: "text_to_video", Prompt: "A quiet coastline", Strategy: "normal", Duration: 5, Resolution: "480p", Steps: 24, ReferenceVideoFPS: fps}
		w := projectRequest(t, s.generationsHandler, "POST", "/api/generations", in, 202)
		s.workers.Wait()
		var g Generation
		if err := json.Unmarshal(w.Body.Bytes(), &g); err != nil {
			t.Fatal(err)
		}
		want := defaultReferenceVideoFPS
		if fps != nil {
			want = *fps
		}
		if g.ReferenceVideoFPS == nil || *g.ReferenceVideoFPS != want {
			t.Fatalf("response lost FPS: %+v", g)
		}
		var raw string
		if err := s.db.QueryRow("SELECT payload FROM generations WHERE id=?", g.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var restored Generation
		if err := json.Unmarshal([]byte(raw), &restored); err != nil {
			t.Fatal(err)
		}
		if *requestFromJob(restored).ReferenceVideoFPS != want {
			t.Fatal("recovered/retried job lost FPS")
		}
		w = projectRequest(t, s.generationHandler, "GET", "/api/generations/"+g.ID, nil, 200)
		if err := json.Unmarshal(w.Body.Bytes(), &restored); err != nil || *restored.ReferenceVideoFPS != want {
			t.Fatal("history lost FPS", err)
		}
	}
	before := len(s.jobs)
	for _, fps := range []float64{-1, 0.5, 61} {
		projectRequest(t, s.generationsHandler, "POST", "/api/generations", generationRequest{ReferenceVideoFPS: fpsValue(fps)}, 400)
		projectRequest(t, s.longVideosHandler, "POST", "/api/long-videos", sequenceInput{AspectRatio: "16:9", Resolution: "480p", Segments: []generationRequest{{Prompt: "Coast", Duration: 5, ReferenceVideoFPS: fpsValue(fps)}}}, 400)
	}
	if len(s.jobs) != before {
		t.Fatal("invalid FPS created a job")
	}
}

func TestReferenceVideoFPSAllReferenceLoaders(t *testing.T) {
	for _, mode := range []string{"multi_reference_video", "long_video"} {
		for _, fps := range []float64{0, 1, 7, 24, 60} {
			graph := map[string]comfyNode{
				"316": {ClassType: "MiniMaxH3ReferenceToVideo", Inputs: map[string]any{}},
				"400": {ClassType: "VHS_VideoCombine", Inputs: map[string]any{"frame_rate": 24}},
			}
			assets := []compiledAsset{{Filename: "a.mp4", Kind: "video"}, {Filename: "b.mp4", Kind: "video"}, {Filename: "c.mp4", Kind: "video"}}
			if err := attachAssets(graph, workflowMode{PromptNode: 316}, mode, assets, fps); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"ref_videos.ref_video_0", "ref_videos.ref_video_1", "ref_videos.ref_video_2"} {
				link := graph["316"].Inputs[key].([]any)
				loader := graph[asString(link[0])]
				if loader.ClassType != "VHS_LoadVideo" || loader.Inputs["force_rate"] != fps {
					t.Fatalf("%s: incorrect loader %#v", key, loader)
				}
			}
			if graph["400"].Inputs["frame_rate"] != 24 {
				t.Fatal("reference FPS changed output FPS")
			}
		}
	}
}

func TestReferenceVideoFPSLegacyAndSequence(t *testing.T) {
	var old Generation
	if err := json.Unmarshal([]byte(`{"id":"old","mode":"multi_reference_video"}`), &old); err != nil {
		t.Fatal(err)
	}
	if *requestFromJob(old).ReferenceVideoFPS != 0 || *publicGeneration(old).ReferenceVideoFPS != 0 {
		t.Fatal("legacy job changed from source FPS")
	}
	s := sequenceTestServer(t)
	v := LongVideo{ID: "sequence_fps", AspectRatio: "16:9", Resolution: "480p", Settings: defaultLongVideoSettings(), Revision: 1, Segments: []VideoSegment{{Request: generationRequest{Prompt: "Coast", Duration: 5, ReferenceVideoFPS: fpsValue(12)}}, {Request: generationRequest{Prompt: "Sea", Duration: 5}}}}
	if err := s.prepareSequence(&v); err != nil {
		t.Fatal(err)
	}
	if *v.Segments[1].Request.ReferenceVideoFPS != 7 {
		t.Fatal("new sequence did not default to 7")
	}
	if err := s.saveRecord("long_videos", v.ID, v); err != nil {
		t.Fatal(err)
	}
	s.longVideos = []LongVideo{v}
	g, err := s.claimSequenceClip(0)
	if err != nil || g == nil || *g.ReferenceVideoFPS != 12 || *requestFromJob(*g).ReferenceVideoFPS != 12 {
		t.Fatalf("sequence lost FPS: %+v %v", g, err)
	}
	oldPlan := LongVideo{ID: "legacy_fps", Segments: []VideoSegment{{Request: generationRequest{Prompt: "Legacy", Duration: 5}}}}
	if err := s.saveRecord("long_videos", oldPlan.ID, oldPlan); err != nil {
		t.Fatal(err)
	}
	s.longVideos = nil
	if err := s.loadLongVideos(); err != nil {
		t.Fatal(err)
	}
	for _, plan := range s.longVideos {
		if plan.ID == oldPlan.ID && *plan.Segments[0].Request.ReferenceVideoFPS != 0 {
			t.Fatal("legacy plan changed cadence")
		}
	}
}

// Uses live object_info and real templates, but never POSTs /prompt.
func TestCompileReferenceVideoFPSAgainstComfyUI(t *testing.T) {
	if os.Getenv("COMFYUI_INTEGRATION") != "1" {
		t.Skip("set COMFYUI_INTEGRATION=1")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "templates", "h3"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"workflow.json", "workflow-longvideo-motion-context.json", "manifest.json"} {
		if err := copyFile(filepath.Join("templates", "h3", name), filepath.Join(root, "templates", "h3", name)); err != nil {
			t.Fatal(err)
		}
	}
	assetDir, inputDir := filepath.Join(root, "data", "assets"), filepath.Join(root, "input")
	for _, dir := range []string{assetDir, inputDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	assets := []Asset{}
	for _, name := range []string{"S01_walking_body_lineart_original.mp4", "S04_hand_open_lineart_original.mp4"} {
		if err := os.WriteFile(filepath.Join(assetDir, name), []byte("compile-only"), 0600); err != nil {
			t.Fatal(err)
		}
		assets = append(assets, Asset{ID: name, Filename: name, Type: "video"})
	}
	s := &Server{root: root, comfyURL: env("COMFYUI_URL", "http://127.0.0.1:8188"), comfyInput: inputDir, httpClient: &http.Client{Timeout: 30 * time.Second}, assets: assets}
	graphs := map[string]any{}
	for _, fps := range []float64{7, 24} {
		for _, long := range []bool{false, true} {
			in := generationRequest{Mode: "multi_reference_video", Prompt: "Preserve the reference motion", Duration: 5, AspectRatio: "16:9", Resolution: "480p", Steps: 24, ReferenceVideoFPS: fpsValue(fps), AssetIDs: []string{assets[0].ID, assets[1].ID}}
			if long {
				timing, _ := calculateClipTiming(5, 1, defaultLongVideoSettings(), 0)
				in.Continuity = &ClipContext{SequenceID: "fps_audit", Index: 1, Settings: defaultLongVideoSettings(), Timing: timing}
			}
			compiled, err := s.compilePrompt(in)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			outputCount := 0
			for _, node := range compiled.Prompt {
				if node.ClassType == "VHS_LoadVideo" && node.Meta["title"] == "dynamic reference video" {
					count++
					if node.Inputs["force_rate"] != fps {
						t.Fatalf("compiled loader ignores FPS: %#v", node)
					}
				}
				if node.ClassType == "VHS_VideoCombine" && asFloat(node.Inputs["frame_rate"]) != 24 {
					t.Fatalf("output FPS changed: %#v", node)
				}
				if node.ClassType == "CreateVideo" {
					outputCount++
					if asFloat(node.Inputs["fps"]) != 24 {
						t.Fatalf("CreateVideo output FPS changed: %#v", node)
					}
				}
				if node.ClassType == "LoraLoaderModelOnly" {
					t.Fatal("no-LoRA request retained a LoRA node")
				}
				if node.ClassType == "BasicScheduler" && asFloat(node.Inputs["steps"]) != 24 {
					t.Fatal("sampling steps changed")
				}
			}
			if count != 2 {
				t.Fatalf("got %d video loaders, want 2", count)
			}
			if outputCount != 1 {
				t.Fatalf("got %d CreateVideo outputs, want 1", outputCount)
			}
			key := "normal"
			if long {
				key = "long"
			}
			if fps == 7 {
				key += "_7fps"
			} else {
				key += "_24fps"
			}
			graphs[key] = compiled
			t.Logf("%s: 2 reference loaders force_rate=%g, output 24 fps", key, fps)
		}
	}
	if path := os.Getenv("REFERENCE_FPS_AUDIT_PATH"); path != "" {
		raw, err := json.MarshalIndent(graphs, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
