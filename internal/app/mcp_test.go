package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectTestMCP(t *testing.T, s *Server) *mcp.ClientSession {
	t.Helper()
	t.Setenv("LOCALDREAM_MCP_TOKEN", "")
	mux := http.NewServeMux()
	mux.Handle("/mcp", s.mcpHandler())
	httpServer := httptest.NewServer(cors(mux))
	t.Cleanup(httpServer.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL + "/mcp", DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
func callTestMCP[T any](t *testing.T, session *mcp.ClientSession, name string, args any) (toolReply[T], bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var reply toolReply[T]
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &reply); err != nil {
		t.Fatalf("%s: %s: %v", name, raw, err)
	}
	return reply, result.IsError
}
func validMCPPrepare() map[string]any {
	return map[string]any{"project_id": "", "request_id": "mcp-draft-001", "mode": "text_to_video", "prompt": "integrated_multimodal_description: Rain falls.\noverall_soundscape: Rain.\nnon_diegetic_music: N/A", "duration": 5, "aspect_ratio": "16:9", "resolution": "720p", "asset_ids": []string{}}
}

func TestMCPStreamableHTTPDraftLifecycle(t *testing.T) {
	s := sequenceTestServer(t)
	// Prevent GPU work: this test covers the durable application boundary only.
	stopped, cancel := context.WithCancel(context.Background())
	cancel()
	s.ctx = stopped
	session := connectTestMCP(t, s)
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 9 {
		t.Fatalf("tools = %d", len(listed.Tools))
	}
	for _, tool := range listed.Tools {
		if tool.InputSchema == nil || tool.OutputSchema == nil || tool.Annotations == nil {
			t.Fatalf("missing contract: %s", tool.Name)
		}
	}
	prepared, failed := callTestMCP[draftToolOutput](t, session, "prepare_generation", validMCPPrepare())
	if failed || !prepared.OK || prepared.Data == nil {
		t.Fatalf("prepare: %+v", prepared)
	}
	draft := prepared.Data.Draft
	if prepared.Data.Submitted || len(s.jobs) != 0 {
		t.Fatal("preparation submitted work")
	}
	again, _ := callTestMCP[draftToolOutput](t, session, "prepare_generation", validMCPPrepare())
	if again.Data.Draft.ID != draft.ID {
		t.Fatal("prepare not idempotent")
	}
	changed := validMCPPrepare()
	changed["duration"] = 6
	conflict, failed := callTestMCP[draftToolOutput](t, session, "prepare_generation", changed)
	if !failed || conflict.Error == nil || conflict.Error.Code != "request_id_conflict" {
		t.Fatalf("conflict: %+v", conflict)
	}
	wrong, failed := callTestMCP[draftToolOutput](t, session, "get_generation_draft", draftToolInput{ProjectID: "other", DraftID: draft.ID})
	if !failed || wrong.OK {
		t.Fatal("cross-project draft visible")
	}
	badRevision, failed := callTestMCP[generationToolOutput](t, session, "submit_generation", submitToolInput{DraftID: draft.ID, Revision: 2})
	if !failed || badRevision.OK {
		t.Fatal("wrong revision accepted")
	}
	// A fresh client connection reads the same draft; transport sessions own no state.
	session2 := connectTestMCP(t, s)
	submitted, failed := callTestMCP[generationToolOutput](t, session2, "submit_generation", submitToolInput{DraftID: draft.ID, Revision: 1})
	if failed || submitted.Data == nil || submitted.Data.ID != draft.GenerationID || submitted.Data.Status != "queued" {
		t.Fatalf("submit: %+v", submitted)
	}
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g, err := s.submitGenerationDraft(context.Background(), draft.ID, "", 1)
			if err != nil || g.ID != draft.GenerationID {
				t.Errorf("concurrent submit: %s %v", g.ID, err)
			}
		}()
	}
	wg.Wait()
	s.workers.Wait()
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM generations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("durable task count %d: %v", count, err)
	}
	wrongJob, failed := callTestMCP[generationToolOutput](t, session, "get_generation", generationToolInput{ProjectID: "other", GenerationID: draft.GenerationID})
	if !failed || wrongJob.OK {
		t.Fatal("cross-project task visible")
	}
	for i := 0; i < 2; i++ {
		cancelled, failed := callTestMCP[generationToolOutput](t, session, "cancel_generation", generationToolInput{GenerationID: draft.GenerationID})
		if failed || cancelled.Data.Status != "cancelled" {
			t.Fatalf("cancel: %+v", cancelled)
		}
	}
	// Reopen the same database to demonstrate restart persistence.
	reopened := &Server{root: s.root, ctx: stopped}
	if err := reopened.openStore(); err != nil {
		t.Fatal(err)
	}
	defer reopened.db.Close()
	loaded, _, err := reopened.loadGenerationDraft(context.Background(), draft.ID, "")
	if err != nil || loaded.ID != draft.ID {
		t.Fatalf("reopen draft: %v", err)
	}
	g, err := reopened.submitGenerationDraft(context.Background(), draft.ID, "", 1)
	if err != nil || g.ID != draft.GenerationID || g.Status != "cancelled" {
		t.Fatalf("restart dedup: %+v %v", g, err)
	}
	reopened.mu.Lock()
	reopened.jobs[0].ProjectID = "moved-project"
	reopened.mu.Unlock()
	if _, err := reopened.submitGenerationDraft(context.Background(), draft.ID, "", 1); err == nil {
		t.Fatal("resubmission leaked a task moved to another project")
	}
}

func TestMCPInvalidArgumentsAndAssetScope(t *testing.T) {
	s := sequenceTestServer(t)
	session := connectTestMCP(t, s)
	for _, tc := range []struct {
		field string
		value any
	}{{"resolution", "4K"}, {"resolution", "0.09MP"}, {"resolution", "1.91MP"}, {"resolution", "NaNMP"}, {"reference_video_fps", -1}, {"reference_video_fps", 0.5}, {"reference_video_fps", 60.1}, {"duration", 16}, {"steps", -1}, {"asset_ids", []string{"a", "a"}}, {"unexpected", true}, {"prompt", ""}} {
		args := validMCPPrepare()
		args[tc.field] = tc.value
		_, failed := callTestMCP[draftToolOutput](t, session, "prepare_generation", args)
		if !failed {
			t.Errorf("accepted invalid %s", tc.field)
		}
	}
	args := validMCPPrepare()
	delete(args, "project_id")
	_, failed := callTestMCP[draftToolOutput](t, session, "prepare_generation", args)
	if !failed {
		t.Fatal("missing project scope accepted")
	}
	s.assets = []Asset{{ID: "foreign", ProjectID: "other", Name: "other project's image", Type: "image"}}
	args = validMCPPrepare()
	args["mode"] = "image_to_video"
	args["asset_ids"] = []string{"foreign"}
	r, failed := callTestMCP[draftToolOutput](t, session, "prepare_generation", args)
	if !failed || r.Error.Code != "invalid_assets_or_project" {
		t.Fatalf("foreign asset: %+v", r)
	}
	page, failed := callTestMCP[toolPage[Asset]](t, session, "list_assets", assetListInput{ProjectID: ""})
	if failed || len(page.Data.Items) != 0 {
		t.Fatal("global workspace leaked other assets")
	}
	guide, failed := callTestMCP[guideToolOutput](t, session, "read_prompt_guide", guideToolInput{Resource: "references/base-en.txt"})
	if failed || !strings.Contains(guide.Data.Content, "Video Prompt Writing Guide") {
		t.Fatal("guide unavailable")
	}
	_, failed = callTestMCP[guideToolOutput](t, session, "read_prompt_guide", guideToolInput{Resource: "../../provider.json"})
	if !failed {
		t.Fatal("guide path escaped allowlist")
	}
}

func TestMCPTransportAccessPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, token, remote, host, origin, auth, forwarded string
		want                                               int
	}{
		{name: "local", remote: "127.0.0.1:1234", host: "localhost:8790", want: 200},
		{name: "ipv6", remote: "[::1]:1234", host: "[::1]:8790", want: 200},
		{name: "rebind", remote: "127.0.0.1:1234", host: "evil.example", want: 403},
		{name: "remote without token", remote: "192.0.2.1:1234", host: "192.0.2.2:8790", want: 403},
		{name: "cross origin", remote: "127.0.0.1:1234", host: "localhost:8790", origin: "https://evil.example", want: 403},
		{name: "null origin", remote: "127.0.0.1:1234", host: "localhost:8790", origin: "null", want: 403},
		{name: "proxy without token", remote: "127.0.0.1:1234", host: "localhost:8790", forwarded: "192.0.2.1", want: 403},
		{name: "missing token", token: "test-secret", remote: "127.0.0.1:1234", host: "localhost:8790", want: 401},
		{name: "wrong token", token: "test-secret", auth: "Bearer wrong", remote: "127.0.0.1:1234", host: "localhost:8790", want: 401},
		{name: "valid token", token: "test-secret", auth: "Bearer test-secret", remote: "192.0.2.1:1234", host: "192.0.2.2:8790", want: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := cors(protectMCP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }), tc.token))
			r := httptest.NewRequest("POST", "http://localhost:8790/mcp", nil)
			r.RemoteAddr = tc.remote
			r.Host = tc.host
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Authorization", tc.auth)
			r.Header.Set("X-Forwarded-For", tc.forwarded)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
			if w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("MCP inherited wildcard CORS")
			}
		})
	}
}

func TestMCPUsesExistingComfySubmissionAndResult(t *testing.T) {
	s := sequenceTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	t.Cleanup(func() { cancel(); s.workers.Wait() })
	template := filepath.Join(s.root, "templates", "h3")
	if err := os.MkdirAll(template, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("H3_TEMPLATE_DIR", template)
	t.Setenv("LOCALDREAM_REQUIRE_IDLE_COMFY", "0")
	writeFixture := func(name string, v any) {
		raw, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(template, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture("manifest.json", workflowManifest{Modes: map[string]workflowMode{"text_to_video": {GroupID: 1, PromptNode: 1, DurationNode: 3, ResolutionNode: 4}}})
	writeFixture("workflow.json", canvasWorkflow{Groups: []canvasGroup{{ID: 1, Bounding: []float64{0, 0, 100, 100}}}, Nodes: []canvasNode{
		{ID: 1, Type: "TestPrompt", Pos: []float64{10, 10}, NamedWidgets: map[string]any{"prompt": ""}},
		{ID: 2, Type: "RandomNoise", Pos: []float64{20, 20}, NamedWidgets: map[string]any{"noise_seed": 1}},
		{ID: 3, Type: "PrimitiveFloat", Pos: []float64{30, 30}, NamedWidgets: map[string]any{"value": 5}},
		{ID: 4, Type: "ResolutionSelector", Pos: []float64{40, 40}, NamedWidgets: map[string]any{"megapixels": 0.9, "aspect_ratio": "16:9 (Widescreen)", "multiple": 32}},
	}})
	var posts atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/prompt":
			posts.Add(1)
			var body comfyPrompt
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.ClientID == "" || body.Prompt["1"].Inputs["prompt"] == "" {
				t.Error("existing compiler/submission contract lost")
			}
			if body.Prompt["3"].Inputs["value"] != float64(7) || body.Prompt["4"].Inputs["megapixels"] != 0.75 {
				t.Error("compiler changed requested duration or custom MP precision")
			}
			writeJSON(w, 200, map[string]string{"prompt_id": "fake-prompt"})
		case r.URL.Path == "/queue":
			writeJSON(w, 200, map[string]any{"queue_running": []any{}, "queue_pending": []any{}})
		case strings.HasPrefix(r.URL.Path, "/history"):
			writeJSON(w, 200, map[string]any{"fake-prompt": map[string]any{"status": map[string]any{"status_str": "success"}, "outputs": map[string]any{"save": map[string]any{"videos": []any{map[string]string{"filename": "test.mp4", "type": "output"}}}}}})
		case strings.HasPrefix(r.URL.Path, "/object_info/"):
			writeJSON(w, 200, map[string]any{"TestPrompt": map[string]any{"input": map[string]any{}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer backend.Close()
	s.comfyURL = backend.URL
	s.httpClient = backend.Client()
	session := connectTestMCP(t, s)
	capabilities, failed := callTestMCP[mcpCapabilitiesOutput](t, session, "get_capabilities", emptyToolInput{})
	if failed || capabilities.Data.Modes[0].RuntimeStatus != "prompt_node_available" {
		t.Fatalf("probe: %+v", capabilities)
	}
	if capabilities.Data.CustomResolutionMPRange != [2]float64{0.1, 1.9} || capabilities.Data.DefaultReferenceVideoFPS != 7 || capabilities.Data.OutputFPS != 24 {
		t.Fatalf("parameter capabilities: %+v", capabilities)
	}
	args := validMCPPrepare()
	args["duration"], args["resolution"], args["reference_video_fps"] = 7, "0.75MP", 12.5
	prepared, failed := callTestMCP[draftToolOutput](t, session, "prepare_generation", args)
	if failed {
		t.Fatalf("prepare: %+v", prepared)
	}
	if posts.Load() != 0 {
		t.Fatal("prepare submitted GPU work")
	}
	in := submitToolInput{DraftID: prepared.Data.Draft.ID, Revision: 1}
	_, failed = callTestMCP[generationToolOutput](t, session, "submit_generation", in)
	if failed {
		t.Fatal("submit failed")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := s.job(prepared.Data.Draft.GenerationID)
		if g.Status == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	result, failed := callTestMCP[generationToolOutput](t, session, "get_generation", generationToolInput{GenerationID: prepared.Data.Draft.GenerationID})
	if failed || result.Data.Status != "completed" || !strings.HasPrefix(result.Data.OutputURL, "/api/generations/") {
		t.Fatalf("result: %+v", result)
	}
	if result.Data.Duration != 7 || result.Data.Resolution != "0.75MP" || result.Data.ReferenceVideoFPS == nil || *result.Data.ReferenceVideoFPS != 12.5 {
		t.Fatalf("task result lost requested parameters: %+v", result)
	}
	_, failed = callTestMCP[generationToolOutput](t, session, "submit_generation", in)
	if failed || posts.Load() != 1 {
		t.Fatalf("POST /prompt count = %d", posts.Load())
	}
}

func TestMCPReferenceFPSAndCustomMPPersistence(t *testing.T) {
	s := sequenceTestServer(t)
	stopped, cancel := context.WithCancel(context.Background())
	cancel() // Persistence only; no real ComfyUI or GPU requests.
	s.ctx = stopped
	session := connectTestMCP(t, s)
	for i, fps := range []any{nil, float64(0), float64(1), 12.5, float64(60)} {
		args := validMCPPrepare()
		args["request_id"] = fmt.Sprintf("mcp-parameters-%d", i)
		args["duration"], args["resolution"], args["reference_video_fps"] = 7, "0.750MP", fps
		prepared, failed := callTestMCP[draftToolOutput](t, session, "prepare_generation", args)
		if failed || prepared.Data == nil {
			t.Fatalf("prepare fps %v: %+v", fps, prepared)
		}
		wantFPS := 7.0
		if fps != nil {
			wantFPS = fps.(float64)
		}
		draft := prepared.Data.Draft
		if draft.Request.Duration != 7 || draft.Request.Continuity != nil || draft.Request.Strategy != "normal" || draft.Request.Resolution != "0.75MP" || *draft.Request.ReferenceVideoFPS != wantFPS {
			t.Fatalf("draft changed parameters: %+v", draft.Request)
		}
		args["resolution"] = "0.75MP"
		again, failed := callTestMCP[draftToolOutput](t, session, "prepare_generation", args)
		if failed || again.Data.Draft.ID != draft.ID {
			t.Fatal("equivalent MP spelling must be idempotent")
		}
		result, failed := callTestMCP[generationToolOutput](t, session, "submit_generation", submitToolInput{DraftID: draft.ID, Revision: 1})
		if failed || result.Data.Resolution != "0.75MP" || result.Data.Duration != 7 || *result.Data.ReferenceVideoFPS != wantFPS {
			t.Fatalf("task lost parameters: %+v", result)
		}
		s.workers.Wait()
		var raw string
		if err := s.db.QueryRow("SELECT payload FROM generations WHERE id=?", draft.GenerationID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var stored Generation
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			t.Fatal(err)
		}
		recovered := requestFromJob(stored)
		if recovered.Duration != 7 || recovered.Resolution != "0.75MP" || *recovered.ReferenceVideoFPS != wantFPS {
			t.Fatalf("restart would change parameters: %+v", recovered)
		}
	}
}
