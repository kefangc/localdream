package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sequenceTestServer(t *testing.T) *Server {
	t.Helper()
	root := t.TempDir()
	t.Setenv("LOCALDREAM_DB", filepath.Join(root, "test.sqlite"))
	s := &Server{root: root, ctx: context.Background(), httpClient: &http.Client{Timeout: time.Second}, assets: []Asset{}, jobs: []Generation{}}
	if err := s.openStore(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.db.Close() })
	return s
}

func TestLongVideoTimingAndNamedWidgets(t *testing.T) {
	c := defaultLongVideoSettings()
	first, err := calculateClipTiming(8, 1, c, 0)
	if err != nil || first.InputFrames != 192 || first.EffectiveSeconds != 8 {
		t.Fatalf("first: %+v %v", first, err)
	}
	second, err := calculateClipTiming(7, 2, c, first.InputFrames)
	if err != nil || second.InputFrames != 192 || second.TrimFrames != 22 || second.EffectiveSeconds != 170.0/24 {
		t.Fatalf("second: %+v %v", second, err)
	}
	eight, _ := calculateClipTiming(8, 2, c, 192)
	if eight.InputFrames != 226 || eight.EffectiveSeconds != 8.5 {
		t.Fatalf("H3 grid ignored: %+v", eight)
	}
	if _, err = calculateClipTiming(200, 2, c, 192); err == nil {
		t.Fatal("native node frame limit ignored")
	}
	s := &Server{}
	inputs, err := s.compileNodeInputs(canvasNode{Inputs: []canvasInput{{Name: "trim_frames", Link: 514}}, NamedWidgets: map[string]any{"trim_frames": 0, "fps": 24, "match_tail": true}}, map[string][]any{"514": {514, 371, 1}})
	if err != nil || inputs["fps"] != 24 || inputs["match_tail"] != true {
		t.Fatalf("optional widgets lost: %#v %v", inputs, err)
	}
	if _, ok := inputs["trim_frames"].([]any); !ok {
		t.Fatal("trim link overwritten by widget")
	}
}

func TestLongVideoSequenceClaimsRecoveryAndContinuation(t *testing.T) {
	s := sequenceTestServer(t)
	v := LongVideo{ID: "sequence_test", AspectRatio: "16:9", Resolution: "480p", Settings: defaultLongVideoSettings(), Revision: 1, Segments: []VideoSegment{{Request: generationRequest{Prompt: "A lighthouse at dawn", Duration: 8}}, {Request: generationRequest{Duration: 7}}}}
	if err := s.prepareSequence(&v); err != nil {
		t.Fatal(err)
	}
	if err := s.saveRecord("long_videos", v.ID, v); err != nil {
		t.Fatal(err)
	}
	s.longVideos = []LongVideo{v}
	first, err := s.claimSequenceClip(0)
	if err != nil || first == nil {
		t.Fatalf("claim: %v", err)
	}
	if duplicate, err := s.claimSequenceClip(0); err != nil || duplicate != nil {
		t.Fatal("submitted first clip twice")
	}
	restored := &Server{db: s.db}
	if err = restored.loadLongVideos(); err != nil {
		t.Fatal(err)
	}
	if restored.longVideos[0].Segments[0].JobID != first.ID {
		t.Fatal("durable sequence link missing")
	}
	var raw []byte
	if err = s.db.QueryRow("SELECT payload FROM generations WHERE id=?", first.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var g Generation
	if err = json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	if requestFromJob(g).Continuity.Index != 1 {
		t.Fatal("recovery lost continuity parameters")
	}
	if err = s.completeJob(first.ID, "http://example.invalid/first.mp4"); err != nil {
		t.Fatal(err)
	}
	if next, err := s.claimSequenceClip(0); err != nil || next != nil {
		t.Fatal("empty next clip must wait")
	}
	if s.sequenceStatus(s.longVideos[0]) != "waiting" {
		t.Fatal("expected waiting for prompt")
	}
	s.longVideos[0].Segments[1].Request.Prompt = "The camera follows a passing sailboat"
	second, err := s.claimSequenceClip(0)
	if err != nil || second == nil {
		t.Fatalf("continuation: %v", err)
	}
	if second.Continuity.Index != 2 || second.Continuity.Timing.TrimFrames != 22 || second.Continuity.SequenceID != v.ID {
		t.Fatalf("bad continuity: %+v", second.Continuity)
	}
	if err = s.updateJob(second.ID, "failed", 0, "compile failed"); err != nil {
		t.Fatal(err)
	}
	if next, _ := s.claimSequenceClip(0); next != nil {
		t.Fatal("advanced past failure")
	}
	if err = s.retrySequence(&s.longVideos[0]); err != nil {
		t.Fatal(err)
	}
	retry, err := s.claimSequenceClip(0)
	if err != nil || retry == nil || retry.ID == second.ID || retry.Continuity.Index != 2 {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	if s.longVideos[0].Segments[0].JobID != first.ID {
		t.Fatal("retry destroyed successful first segment")
	}
}

func TestRegenerateSequenceResetsSuffix(t *testing.T) {
	s := sequenceTestServer(t)
	v := LongVideo{ID: "regen", AspectRatio: "16:9", Resolution: "480p", Settings: defaultLongVideoSettings(), Revision: 1, Segments: []VideoSegment{{JobID: "a"}, {JobID: "b"}, {JobID: "c"}}}
	s.longVideos = []LongVideo{v}
	for _, id := range []string{"a", "b", "c"} {
		s.jobs = append(s.jobs, Generation{ID: id, Status: "completed"})
	}
	if err := s.regenerateSequence(&s.longVideos[0], 2); err != nil {
		t.Fatal(err)
	}
	if s.longVideos[0].Segments[0].JobID != "a" || s.longVideos[0].Segments[1].JobID != "" || s.longVideos[0].Segments[2].JobID != "" {
		t.Fatal("suffix not reset")
	}
	if !s.jobs[1].Superseded || !s.jobs[2].Superseded {
		t.Fatal("old jobs not marked superseded")
	}
}

func TestRegenerateSecondSegmentKeepsFirstLatent(t *testing.T) {
	s := sequenceTestServer(t)
	folder := filepath.Join(s.root, "output", "h3_context", "localdream", "regen2")
	if err := os.MkdirAll(folder, 0755); err != nil {
		t.Fatal(err)
	}
	for _, slot := range []string{"clip_00001.safetensors", "clip_00002.safetensors", "clip_00003.safetensors"} {
		if err := os.WriteFile(filepath.Join(folder, slot), []byte("latent"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s.comfyInput = filepath.Join(s.root, "input")
	v := LongVideo{ID: "regen2", AspectRatio: "16:9", Resolution: "480p", Settings: defaultLongVideoSettings(), Segments: []VideoSegment{{JobID: "a"}, {JobID: "b"}, {JobID: "c"}}}
	s.longVideos = []LongVideo{v}
	for _, id := range []string{"a", "b", "c"} {
		s.jobs = append(s.jobs, Generation{ID: id, Status: "completed"})
	}
	if err := s.regenerateSequence(&s.longVideos[0], 2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, "clip_00001.safetensors")); err != nil {
		t.Fatal("previous continuity latent was deleted")
	}
	if _, err := os.Stat(filepath.Join(folder, "clip_00002.safetensors")); err != nil {
		t.Fatal("latent history was deleted")
	}
}

func TestRegenerateEarlierSegmentRebuildsWhenPredecessorWasPruned(t *testing.T) {
	s := sequenceTestServer(t)
	s.comfyInput = filepath.Join(s.root, "input")
	v := LongVideo{ID: "rebuild", Settings: defaultLongVideoSettings(), Segments: []VideoSegment{{JobID: "a"}, {JobID: "b"}}}
	s.longVideos = []LongVideo{v}
	s.jobs = []Generation{{ID: "a", Status: "completed"}, {ID: "b", Status: "completed"}}
	if err := s.regenerateSequence(&s.longVideos[0], 2); err != nil {
		t.Fatal(err)
	}
	if s.longVideos[0].Segments[0].JobID != "a" || s.longVideos[0].Segments[1].JobID != "" {
		t.Fatal("direct reroll changed the completed prefix")
	}
}

func TestMiddleRerollCreatesForkAndPreservesOriginal(t *testing.T) {
	s := sequenceTestServer(t)
	s.comfyInput = filepath.Join(s.root, "input")
	src := filepath.Join(s.root, "output", "h3_context", "localdream", "source")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"clip_00001.safetensors", "clip_00002.safetensors"} {
		if err := os.WriteFile(filepath.Join(src, n), []byte(n), 0600); err != nil {
			t.Fatal(err)
		}
	}
	v := LongVideo{ID: "source", Title: "起床", AspectRatio: "16:9", Resolution: "480p", Settings: defaultLongVideoSettings(), Revision: 4, Segments: []VideoSegment{{JobID: "a"}, {JobID: "b"}, {JobID: "c"}}}
	s.longVideos = []LongVideo{v}
	s.jobs = []Generation{{ID: "a", Status: "completed"}, {ID: "b", Status: "completed"}, {ID: "c", Status: "completed"}}
	fork, err := s.regenerateOrFork(&s.longVideos[0], 2)
	if err != nil {
		t.Fatal(err)
	}
	if fork.ID == v.ID || len(s.longVideos) != 2 {
		t.Fatal("fork was not created")
	}
	if s.longVideos[1].Segments[1].JobID != "b" {
		t.Fatal("original changed")
	}
	if fork.Segments[0].JobID != "a" || fork.Segments[1].JobID != "" || fork.Segments[2].JobID != "" {
		t.Fatal("fork prefix/suffix incorrect")
	}
	dst := filepath.Join(s.root, "output", "h3_context", "localdream", fork.ID)
	if _, err := os.Stat(filepath.Join(dst, "clip_00001.safetensors")); err != nil {
		t.Fatal("fork did not copy predecessor latent")
	}
}

func TestLongVideoHTTPSettingsAndImmutableFormat(t *testing.T) {
	s := sequenceTestServer(t)
	call := func(method, path string, body any, handler http.HandlerFunc) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(raw))
		w := httptest.NewRecorder()
		handler(w, r)
		return w
	}
	in := sequenceInput{Title: "海边", AspectRatio: "16:9", Resolution: "720p", Segments: []generationRequest{{Prompt: "海边日出", Duration: 8}, {Duration: 7}}}
	w := call("POST", "/api/long-videos", in, s.longVideosHandler)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	var saved LongVideo
	json.Unmarshal(w.Body.Bytes(), &saved)
	cfg := defaultLongVideoSettings()
	cfg.ContextFrames = 5
	w = call("POST", "/api/config/long-video", cfg, s.longVideoSettingsHandler)
	if w.Code != 200 {
		t.Fatal(w.Body)
	}
	if s.longVideos[0].Settings.ContextFrames != 22 {
		t.Fatal("global settings changed existing sequence")
	}
	in.Revision = saved.Revision
	in.Resolution = "480p"
	w = call("PUT", "/api/long-videos/"+saved.ID, in, s.longVideoHandler)
	if w.Code != 400 {
		t.Fatal("allowed resolution change")
	}
	in.Resolution = "720p"
	in.Segments[1].Prompt = "镜头推向灯塔"
	in.Start = true
	w = call("PUT", "/api/long-videos/"+saved.ID, in, s.longVideoHandler)
	if w.Code != 200 {
		t.Fatalf("append: %s", w.Body)
	}
	w = call("PUT", "/api/long-videos/"+saved.ID, in, s.longVideoHandler)
	if w.Code != 409 {
		t.Fatal("stale editor overwrote newer plan")
	}
}

// Read-only compiler integration. Never POST /prompt or copy reference media.
func TestCompileLongVideoAgainstComfyUI(t *testing.T) {
	if os.Getenv("COMFYUI_INTEGRATION") != "1" {
		t.Skip("live read-only object_info validation is opt-in")
	}
	root := testProjectRoot(t)
	s := &Server{root: root, comfyURL: env("COMFYUI_URL", "http://127.0.0.1:8188"), httpClient: &http.Client{Timeout: 10 * time.Second}}
	c := defaultLongVideoSettings()
	for _, index := range []int{1, 2} {
		timing, _ := calculateClipTiming(8, index, c, 192)
		graph, err := s.compilePrompt(generationRequest{Prompt: "A lighthouse at dawn", Duration: 8, AspectRatio: "16:9", Resolution: "480p", Steps: 24, Continuity: &ClipContext{"sequence_compile_only", index, c, timing}})
		if err != nil {
			t.Fatal(err)
		}
		if graph.Prompt["373"].Inputs["clip_index"] != index-1 || graph.Prompt["374"].Inputs["clip_index"] != index {
			t.Fatal("wrong clip indices")
		}
		if graph.Prompt["316"].Inputs["length"] != timing.InputFrames {
			t.Fatal("wrong H3 frame count")
		}
		for _, n := range graph.Prompt {
			if n.ClassType == "LoadImage" {
				t.Fatal("template sample image leaked")
			}
		}
	}
}

func TestCompileLongVideoTemplateOffline(t *testing.T) {
	// Small object-info double: exercises the actual canvas compiler without a
	// running GPU service. Live schema validation remains a separate opt-in test.
	widgets := map[string][]string{
		"PrimitiveFloat": {"value"}, "ComfyMathExpression": {"expression"},
		"ResolutionSelector": {"aspect_ratio", "megapixels", "multiple"},
		"CreateVideo":        {"images", "audio", "fps"}, "SaveVideo": {"video", "filename_prefix", "format", "codec"},
		"VAEDecodeAudio": {"samples", "vae"}, "VAEDecode": {"samples", "vae"}, "VAELoader": {"vae_name"},
		"CLIPLoader": {"clip_name", "type", "device"}, "LoadImage": {"image"}, "RandomNoise": {"noise_seed"},
		"SamplerCustomAdvanced": {"noise", "guider", "sampler", "sigmas", "latent_image"}, "BasicGuider": {"model", "conditioning"},
		"LoraLoaderModelOnly": {"model", "lora_name", "strength_model"}, "KSamplerSelect": {"sampler_name"},
		"BasicScheduler": {"model", "scheduler", "steps", "denoise"}, "UNETLoader": {"unet_name", "weight_dtype"},
		"TESpeedMiniMaxH3": {"model", "mode"}, "MiniMaxH3MemoryEfficientSageAttentionPatch": {"model"},
		"MiniMaxH3ReferenceToVideo": {"clip", "prompt", "width", "height", "length", "ref_image_size"},
	}
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kind := strings.TrimPrefix(r.URL.Path, "/object_info/")
		order, ok := widgets[kind]
		if !ok {
			http.Error(w, "unexpected class "+kind, 400)
			return
		}
		required := map[string]any{}
		for _, key := range order {
			required[key] = []any{"STRING"}
		}
		if kind == "CLIPLoader" {
			required["clip_name"] = []any{[]string{"qwen3vl_32b_minimax_h3_nvfp4_awq.safetensors"}}
		}
		writeJSON(w, 200, map[string]any{kind: map[string]any{"input": map[string]any{"required": required}, "input_order": map[string]any{"required": order}}})
	}))
	defer remote.Close()
	root := testProjectRoot(t)
	s := &Server{root: root, comfyURL: remote.URL, httpClient: remote.Client()}
	c := defaultLongVideoSettings()
	for _, index := range []int{1, 2, 3} {
		timing, _ := calculateClipTiming(8, index, c, 192)
		compiled, err := s.compilePrompt(generationRequest{Prompt: "A lighthouse at dawn", Duration: 8, AspectRatio: "16:9", Resolution: "480p", Steps: 24, Continuity: &ClipContext{"sequence_offline", index, c, timing}})
		if err != nil {
			t.Fatal(err)
		}
		graph := compiled.Prompt
		if graph["373"].Inputs["clip_index"] != index-1 || graph["374"].Inputs["clip_index"] != index {
			t.Fatal("incorrect continuity index")
		}
		if graph["373"].Inputs["latent_path"] != "h3_context/localdream/sequence_offline" {
			t.Fatal("latent not isolated")
		}
		if graph["316"].Inputs["length"] != timing.InputFrames || graph["288"].Inputs["steps"] != 24 {
			t.Fatal("duration or step override lost")
		}
		if graph["372"].Inputs["match_tail"] != true {
			t.Fatal("audio tail option lost")
		}
		for _, node := range graph {
			if node.ClassType == "LoadImage" || node.ClassType == "TESpeedMiniMaxH3" {
				t.Fatal("unused template sample or optional wrapper retained")
			}
			for _, value := range node.Inputs {
				if link, ok := value.([]any); ok && len(link) == 2 {
					if _, exists := graph[asString(link[0])]; !exists {
						t.Fatalf("dangling link: %v", link)
					}
				}
			}
		}
	}
}

// Isolated visual QA server: temp DB/assets, no ComfyUI startup, no dispatcher.
// Generation is deliberately rejected; this is not a simulated render result.
func TestLongVideoUIPreview(t *testing.T) {
	if os.Getenv("LOCALDREAM_UI_PREVIEW") != "1" {
		t.Skip("interactive visual QA only")
	}
	s := sequenceTestServer(t)
	root := testProjectRoot(t)
	if err := os.MkdirAll(filepath.Join(s.root, "data", "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	// Only the preview copies read-only record metadata for realistic card layout.
	// Changes made in the preview stay in its temporary database.
	if os.Getenv("LOCALDREAM_UI_RECORDS") == "1" {
		client := &http.Client{Timeout: 3 * time.Second}
		if resp, err := client.Get("http://127.0.0.1:8790/api/generations"); err == nil {
			var records []Generation
			_ = json.NewDecoder(resp.Body).Decode(&records)
			resp.Body.Close()
			for _, g := range records {
				if g.Status == "completed" && len(s.jobs) < 3 {
					g.ProjectID = ""
					s.jobs = append(s.jobs, g)
				}
			}
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/assets", s.assetsHandler)
	mux.HandleFunc("/api/assets/", s.deleteAssetHandler)
	mux.HandleFunc("/api/projects", s.projectsHandler)
	mux.HandleFunc("/api/projects/", s.projectHandler)
	mux.HandleFunc("/api/generations", s.generationsHandler)
	mux.HandleFunc("/api/generations/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/video") && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			w.Header().Set("Location", "http://127.0.0.1:8790"+r.URL.Path)
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		s.generationHandler(w, r)
	})
	mux.HandleFunc("/api/config/long-video", s.longVideoSettingsHandler)
	mux.HandleFunc("/api/config/provider", s.providerConfigHandler)
	mux.HandleFunc("/api/long-videos", s.longVideosHandler)
	mux.HandleFunc("/api/long-videos/", s.longVideoHandler)
	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(root, "frontend", "dist"))))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" || r.Method == "PUT" {
			raw, _ := io.ReadAll(r.Body)
			r.Body.Close()
			var in struct {
				Start bool `json:"start"`
			}
			json.Unmarshal(raw, &in)
			if in.Start || r.URL.Path == "/api/generations" {
				writeJSON(w, 403, map[string]string{"error": "隔离预览不提交真实生成，请使用正式服务执行"})
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		mux.ServeHTTP(w, r)
	})
	t.Log("isolated preview: http://127.0.0.1:8793 (no real inference)")
	if err := http.ListenAndServe("127.0.0.1:8793", handler); err != nil {
		t.Fatal(err)
	}
}
