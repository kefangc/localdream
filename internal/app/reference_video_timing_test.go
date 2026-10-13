package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestDedicatedServerGuardOnlyReadsAndRejectsBusyQueue(t *testing.T) {
	t.Setenv("LOCALDREAM_REQUIRE_IDLE_COMFY", "1")
	busy := true
	reads := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/queue" {
			t.Errorf("guard attempted a mutation: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		reads++
		if busy {
			_, _ = w.Write([]byte(`{"queue_running":[[1,"other-job"]],"queue_pending":[]}`))
		} else {
			_, _ = w.Write([]byte(`{"queue_running":[],"queue_pending":[]}`))
		}
	}))
	defer backend.Close()
	s := &Server{comfyURL: backend.URL, httpClient: backend.Client()}
	if err := s.requireIdleComfy(); err == nil {
		t.Fatal("dedicated server accepted another producer's running task")
	}
	busy = false
	if err := s.requireIdleComfy(); err != nil || reads != 2 {
		t.Fatal("empty queue was not accepted through read-only checks", err, reads)
	}
}

func TestReferenceTimingOptionPersistsWithoutChangingLegacy(t *testing.T) {
	s := sequenceTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.ctx = ctx // Persist through the API; never allow a submission worker.
	in := generationRequest{Mode: "text_to_video", Prompt: "Moving clouds", Strategy: "normal", Duration: 3, Resolution: "480p", ReferenceVideoPreserveDuration: true}
	w := projectRequest(t, s.generationsHandler, "POST", "/api/generations", in, 202)
	s.workers.Wait()
	var job Generation
	if err := json.Unmarshal(w.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if !job.ReferenceVideoPreserveDuration || job.Steps != 50 {
		t.Fatal("new API task did not retain timing option / default 50 steps")
	}
	var raw string
	if err := s.db.QueryRow("SELECT payload FROM generations WHERE id=?", job.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		t.Fatal(err)
	}
	if !requestFromJob(job).ReferenceVideoPreserveDuration {
		t.Fatal("retry/recovery lost opt-in timing semantics")
	}
	var old Generation
	if err := json.Unmarshal([]byte(`{"id":"legacy","steps":24,"reference_video_fps":7}`), &old); err != nil {
		t.Fatal(err)
	}
	if requestFromJob(old).ReferenceVideoPreserveDuration || requestFromJob(old).Steps != 24 {
		t.Fatal("legacy task changed timing or steps")
	}
}

// CPU FFmpeg + read-only ComfyUI object_info only. This never POSTs /prompt.
func TestReferenceDurationPreservationAgainstRealTools(t *testing.T) {
	if os.Getenv("LOCALDREAM_TIMING_INTEGRATION") != "1" {
		t.Skip("set LOCALDREAM_TIMING_INTEGRATION=1")
	}
	app := testProjectRoot(t)
	t.Setenv("H3_TEMPLATE_DIR", filepath.Join(app, "templates", "h3"))
	root := t.TempDir()
	assetDir, inputDir := filepath.Join(root, "data", "assets"), filepath.Join(root, "input")
	for _, dir := range []string{assetDir, inputDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	tool, err := referenceTool("LOCALDREAM_FFMPEG", "ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(assetDir, "motion.mp4")
	cmd := exec.Command(tool, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-f", "lavfi", "-i", "testsrc2=size=160x96:rate=24:duration=3", "-an", "-c:v", "libx264", "-crf", "12", "-pix_fmt", "yuv420p", source)
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(err, string(raw))
	}
	assets := []Asset{}
	for _, name := range []string{"identity.png", "first.png"} {
		if err := os.WriteFile(filepath.Join(assetDir, name), []byte("compile-only"), 0600); err != nil {
			t.Fatal(err)
		}
		assets = append(assets, Asset{ID: name, Filename: name, Type: "image"})
	}
	assets = append(assets, Asset{ID: "motion.mp4", Filename: "motion.mp4", Type: "video"})
	s := &Server{root: root, comfyInput: inputDir, comfyURL: "http://127.0.0.1:8188", httpClient: &http.Client{Timeout: 30 * time.Second}, assets: assets}
	results := map[string]any{}
	for _, fps := range []float64{7, 24} {
		in := generationRequest{Mode: "multi_reference_video", Prompt: "Picture 1 identity. Picture 2 initial composition. Video 1 full real-time motion. Continue movement without returning to the start.", Duration: 3, AspectRatio: "16:9", Resolution: "0.7MP", Steps: 50, Lora: "", LoraStrength: 0, ReferenceVideoFPS: fpsValue(fps), ReferenceVideoPreserveDuration: true, AssetIDs: []string{"identity.png", "first.png", "motion.mp4"}}
		compiled, err := s.compilePrompt(in)
		if err != nil {
			t.Fatal(err)
		}
		loaders, schedulers := 0, 0
		for _, node := range compiled.Prompt {
			if strings.Contains(strings.ToLower(node.ClassType), "lora") || node.ClassType == "MiniMaxH3AddGuide" {
				t.Fatal("native production graph retained unwanted LoRA / endpoint anchor")
			}
			if node.ClassType == "BasicScheduler" {
				schedulers++
				if asFloat(node.Inputs["steps"]) != 50 || node.Inputs["scheduler"] != "simple" || asFloat(node.Inputs["denoise"]) != 1 {
					t.Fatal("incorrect actual sampling schedule", node.Inputs)
				}
			}
			if node.ClassType == "VHS_LoadVideo" {
				loaders++
				if asFloat(node.Inputs["force_rate"]) != 24 {
					t.Fatal("sparse frames still reached H3 without preserved real time")
				}
				p := filepath.Join(inputDir, asString(node.Inputs["video"]))
				duration, frames, err := probeReferenceVideo(p)
				if err != nil || frames != 73 || duration < 3 || duration > 3.05 {
					t.Fatal("3-second action duration was shortened", duration, frames, err)
				}
			}
		}
		if loaders != 1 || schedulers != 1 {
			t.Fatal("missing scheduler/reference loader")
		}
		results["sampling_"+strconv.FormatFloat(fps, 'g', -1, 64)] = compiled.Prompt
	}
	// I2V takes one true first-frame image and does not anchor the end.
	i2v, err := s.compilePrompt(generationRequest{Mode: "image_to_video", Prompt: "Continue visible movement from this first frame.", Duration: 3, AspectRatio: "16:9", Resolution: "0.7MP", Steps: 50, AssetIDs: []string{"first.png"}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, node := range i2v.Prompt {
		if node.ClassType == "MiniMaxH3ImageToVideo" {
			found = node.Inputs["first_frame"] != nil && node.Inputs["last_frame"] == nil
		}
	}
	if !found {
		t.Fatal("I2V did not bind only the initial frame")
	}
	results["image_to_video"] = i2v.Prompt
	if validationDir := os.Getenv("LOCALDREAM_TIMING_VALIDATION_DIR"); validationDir != "" {
		if err := os.MkdirAll(validationDir, 0755); err != nil {
			t.Fatal(err)
		}
		records := []map[string]any{}
		for _, name := range []string{"S01_walking_body_lineart_original.mp4", "S04_hand_open_lineart_original.mp4"} {
			source := filepath.Join(`E:\idea\frieren-op\production\sources`, name)
			for _, fps := range []float64{7, 24} {
				filename, err := prepareReferenceVideoFile(source, validationDir, fps)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(validationDir, filename)
				var timing referenceTimingRecord
				if err := readJSON(path+".timing.json", &timing); err != nil {
					t.Fatal(err)
				}
				records = append(records, map[string]any{"source": source, "prepared": path, "timing": timing})
			}
		}
		raw, _ := json.MarshalIndent(records, "", "  ")
		if err := os.WriteFile(filepath.Join(validationDir, "real-reference-timing-validation.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if output := os.Getenv("LOCALDREAM_TIMING_AUDIT_OUTPUT"); output != "" {
		raw, _ := json.MarshalIndent(results, "", "  ")
		if err := os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
