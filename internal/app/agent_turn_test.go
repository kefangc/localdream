package app

import (
	"context"
	"encoding/json"
	"fmt"
	"localdream/internal/responses"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAgentTurnLoadsH3AndPreparesWithoutSubmitting(t *testing.T) {
	var requests atomic.Int32
	var sawSkillReference bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []json.RawMessage `json:"input"`
			Tools []responses.Tool  `json:"tools"`
			Store bool              `json:"store"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Store || len(body.Tools) < 5 {
			t.Errorf("missing tools or stateless setting: %#v", body)
		}
		for _, item := range body.Input {
			if strings.Contains(string(item), "Video Prompt Writing Guide") {
				sawSkillReference = true
			}
		}
		step := requests.Add(1)
		var output string
		switch step {
		case 1:
			output = `{"type":"function_call","call_id":"call_1","name":"load_skill","arguments":"{\"name\":\"h3-prompt-writing\"}"}`
		case 2:
			output = `{"type":"function_call","call_id":"call_2","name":"read_skill_resource","arguments":"{\"name\":\"h3-prompt-writing\",\"resource\":\"references/base-en.txt\"}"}`
		case 3:
			args, _ := json.Marshal(prepareGenerationArgs{Mode: "text_to_video", Prompt: "integrated_multimodal_description: [Shot 1] Rain falls.\n\noverall_soundscape: Rain.\n\nnon_diegetic_music: N/A", Duration: 5, AspectRatio: "16:9", Resolution: "720p", Steps: 24, AssetIDs: []string{}})
			item, _ := json.Marshal(map[string]string{"type": "function_call", "call_id": "call_3", "name": "prepare_generation", "arguments": string(args)})
			output = string(item)
		case 4:
			output = `{"type":"function_call","call_id":"call_4","name":"submit_generation","arguments":"{}"}`
		default:
			output = `{"type":"message","content":[{"type":"output_text","text":"草稿已完成，等待用户提交。"}]}`
		}
		_, _ = fmt.Fprintf(w, `{"id":"response_%d","status":"completed","output":[%s]}`, step, output)
	}))
	defer provider.Close()

	s := &Server{root: t.TempDir()}
	result, err := s.runAgentTurn(context.Background(), agentTurnInput{Prompt: "做一段下雨的视频", ExecutionPolicy: "plan_only", SkillPolicy: "auto"}, ProviderConfig{BaseURL: provider.URL, Model: "mock-model"}, &responses.Client{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Draft == nil || result.Generation != nil || len(result.Skills) != 1 || !sawSkillReference || result.Text == "" {
		t.Fatalf("incomplete planning result: %#v, skill reference seen: %v", result, sawSkillReference)
	}
}

func TestAgentGenerationRequestIDPreventsDuplicateTask(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	s := &Server{root: root, ctx: context.Background()}
	if err := s.openStore(); err != nil {
		t.Fatal(err)
	}
	defer s.db.Close()
	request := agentTurnInput{ClientTurnID: "same-turn-123", Prompt: "Rain on a quiet street", ExecutionPolicy: "auto", SkillPolicy: "auto"}
	jobID, fingerprint := agentRequestIdentity(request)
	draft := generationRequest{AgentJobID: jobID, AgentFingerprint: fingerprint, ProjectID: "", Strategy: "normal", Mode: "text_to_video", Prompt: "integrated_multimodal_description: Rain.\noverall_soundscape: Rain.\nnon_diegetic_music: N/A", Duration: 5, AspectRatio: "16:9", Resolution: "720p", Steps: 24}
	first, status, err := s.createGeneration(draft)
	if err != nil || status != 202 {
		t.Fatalf("first task: %v, %d", err, status)
	}
	second, _, err := s.createGeneration(draft)
	if err != nil || first.ID != second.ID {
		t.Fatalf("duplicate request created a new task: %v, %s, %s", err, first.ID, second.ID)
	}
	s.workers.Wait()
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM generations WHERE id=?", first.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("expected one durable task, got %d: %v", count, err)
	}
	s2 := &Server{root: root, ctx: context.Background()}
	if err := s2.openStore(); err != nil {
		t.Fatal(err)
	}
	defer s2.db.Close()
	if persisted, ok := s2.job(first.ID); !ok || persisted.AgentRequestFingerprint != fingerprint {
		t.Fatalf("task identity was not recovered: %#v, %v", persisted, ok)
	}
}

func TestAgentDraftRejectsUnsupportedMediaClaims(t *testing.T) {
	base := generationRequest{Mode: "multi_reference_video", Prompt: "subject_definitions: A\nsummary: B\nretention_analysis: C\ndetailed_description: D\noverall_soundscape: E\nnon_diegetic_music: F", Duration: 8, AspectRatio: "16:9", Resolution: "720p", Steps: 24}
	base.AssetIDs = []string{"video_1"}
	if err := validateAgentDraft(base, []Asset{{ID: "video_1", Type: "video"}}); err != nil {
		t.Fatal(err)
	}
	base.Resolution = "2K"
	if err := validateAgentDraft(base, []Asset{{ID: "video_1", Type: "video"}}); err == nil {
		t.Fatal("unsupported resolution was accepted")
	}
	base.Resolution = "720p"
	base.AssetIDs = []string{"video_1", "another_project_asset"}
	if err := validateAgentDraft(base, []Asset{{ID: "video_1", Type: "video"}}); err == nil {
		t.Fatal("unselected asset was accepted")
	}
}
