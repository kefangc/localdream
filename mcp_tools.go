package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"localdream/internal/skills"
)

type toolReply[T any] struct {
	OK    bool       `json:"ok"`
	Data  *T         `json:"data,omitempty"`
	Error *toolFault `json:"error,omitempty"`
}

func addMCPTool[In, Out any](server *mcp.Server, name, description string, readOnly, destructive bool, handler func(context.Context, In) (Out, error)) {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(err)
	} // all types are known at build time and exercised in tests
	for field, values := range map[string][]any{
		"mode":       {"text_to_video", "image_to_video", "first_last_frame", "multi_reference_video"},
		"resolution": {"480p", "720p"}, "aspect_ratio": {"16:9", "9:16", "1:1", "4:3"},
		"resource": {"SKILL.md", "references/base-en.txt", "references/ref-en.txt"},
		"type":     {"", "image", "video", "audio"},
	} {
		if p := schema.Properties[field]; p != nil {
			p.Enum = values
		}
	}
	for field, bounds := range map[string][2]float64{"duration": {1, 15}, "steps": {1, 100}, "limit": {1, 100}, "offset": {0, 1000000}, "revision": {1, 1}} {
		if p := schema.Properties[field]; p != nil {
			lo, hi := bounds[0], bounds[1]
			p.Minimum, p.Maximum = &lo, &hi
		}
	}
	if p := schema.Properties["asset_ids"]; p != nil {
		maximum := 13
		p.MaxItems = &maximum
		p.UniqueItems = true
	}
	if p := schema.Properties["request_id"]; p != nil {
		p.Pattern = clientTurnIDPattern.String()
	}
	if p := schema.Properties["prompt"]; p != nil {
		minimum, maximum := 1, 32000
		p.MinLength, p.MaxLength = &minimum, &maximum
	}
	closed := false
	mcp.AddTool(server, &mcp.Tool{Name: name, Description: description, InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, DestructiveHint: &destructive, IdempotentHint: true, OpenWorldHint: &closed}},
		func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, toolReply[Out], error) {
			data, err := handler(ctx, in)
			if err == nil {
				return nil, toolReply[Out]{OK: true, Data: &data}, nil
			}
			var problem *toolFault
			if !errors.As(err, &problem) {
				switch {
				case errors.Is(err, sql.ErrNoRows):
					problem = toolProblem("not_found", "Object not found in this project.", false)
				case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
					problem = toolProblem("request_interrupted", "Request interrupted. Read the existing draft/task before retrying a submission.", true)
				default:
					log.Printf("MCP %s: %v", name, err)
					problem = toolProblem("internal_error", "Local Dream could not complete this operation; inspect the server log. For submission, query the existing draft/task before retrying.", true)
				}
			}
			return &mcp.CallToolResult{IsError: true}, toolReply[Out]{OK: false, Error: problem}, nil
		})
}

type emptyToolInput struct{}
type projectListInput struct {
	Offset int `json:"offset,omitempty" jsonschema:"Pagination offset; default 0."`
	Limit  int `json:"limit,omitempty" jsonschema:"Page size; default 30, maximum 100."`
}
type assetListInput struct {
	ProjectID string `json:"project_id" jsonschema:"Exact project ID from list_projects. Empty string selects only the global workspace."`
	Type      string `json:"type,omitempty" jsonschema:"Optional image, video or audio filter."`
	Query     string `json:"query,omitempty" jsonschema:"Case-insensitive substring of asset name."`
	Offset    int    `json:"offset,omitempty"`
	Limit     int    `json:"limit,omitempty" jsonschema:"Page size; default 30, maximum 100."`
}
type toolPage[T any] struct {
	Items      []T  `json:"items"`
	Total      int  `json:"total"`
	NextOffset *int `json:"next_offset,omitempty"`
}

func paginateTools[T any](items []T, offset, limit int) toolPage[T] {
	if limit == 0 {
		limit = 30
	}
	start := min(offset, len(items))
	end := min(start+limit, len(items))
	page := toolPage[T]{Items: append([]T{}, items[start:end]...), Total: len(items)}
	if end < len(items) {
		page.NextOffset = &end
	}
	return page
}

type prepareToolInput struct {
	ProjectID         string   `json:"project_id" jsonschema:"Exact project ID; empty string is the global workspace."`
	RequestID         string   `json:"request_id" jsonschema:"Client-generated 8–128 character idempotency key. Reuse for retries; use a new key for changed parameters."`
	Mode              string   `json:"mode" jsonschema:"H3 generation mode. Check get_capabilities first."`
	Prompt            string   `json:"prompt" jsonschema:"Complete structured H3 prompt. Read read_prompt_guide for required sections."`
	Duration          float64  `json:"duration" jsonschema:"Requested video duration in seconds, 1–15."`
	AspectRatio       string   `json:"aspect_ratio"`
	Resolution        string   `json:"resolution"`
	Steps             int      `json:"steps,omitempty" jsonschema:"Sampling steps, 1–100; omitted defaults to 50."`
	AssetIDs          []string `json:"asset_ids" jsonschema:"Ordered asset IDs from this project. For first_last_frame: first image then last image. Text-to-video requires an empty array."`
	ReferenceVideoFPS *float64 `json:"reference_video_fps,omitempty" jsonschema:"Reference decoding FPS: omit for 7; 0 uses source FPS; otherwise 1–60. Output remains 24 FPS."`
}
type draftToolInput struct {
	ProjectID string `json:"project_id" jsonschema:"Project scope; empty string selects global workspace."`
	DraftID   string `json:"draft_id"`
}
type submitToolInput struct {
	ProjectID string `json:"project_id"`
	DraftID   string `json:"draft_id"`
	Revision  int    `json:"revision" jsonschema:"Exact revision returned by prepare_generation or get_generation_draft."`
}
type generationToolInput struct {
	ProjectID    string `json:"project_id" jsonschema:"Must match the generation's project; empty string selects global workspace."`
	GenerationID string `json:"generation_id"`
}
type guideToolInput struct {
	Resource string `json:"resource" jsonschema:"SKILL.md for overview; base-en.txt for text/image/first-last-frame; ref-en.txt for multi-reference."`
}
type guideToolOutput struct {
	Content string `json:"content"`
}
type draftToolOutput struct {
	Draft      generationDraft `json:"draft"`
	Submitted  bool            `json:"submitted"`
	Validation string          `json:"validation"`
}
type generationToolOutput struct {
	ID               string    `json:"generation_id"`
	ProjectID        string    `json:"project_id"`
	Status           string    `json:"status"`
	Phase            string    `json:"phase"`
	Mode             string    `json:"mode"`
	Duration         float64   `json:"duration"`
	Resolution       string    `json:"resolution"`
	AspectRatio      string    `json:"aspect_ratio"`
	Progress         int       `json:"progress"`
	ProgressKnown    bool      `json:"progress_known"`
	OutputURL        string    `json:"output_url,omitempty" jsonschema:"Path relative to the Local Dream server; present only after completion."`
	Error            string    `json:"error,omitempty"`
	UpdatedAt        time.Time `json:"updated_at"`
	PollAfterSeconds int       `json:"poll_after_seconds"`
}

func generationToolView(g Generation) generationToolOutput {
	public := publicGeneration(g)
	view := generationToolOutput{ID: g.ID, ProjectID: g.ProjectID, Status: g.Status, Phase: g.Phase, Mode: g.Mode, Duration: g.Duration, Resolution: g.Resolution, AspectRatio: g.AspectRatio, Progress: g.Progress, ProgressKnown: g.ProgressKnown, Error: g.Error, UpdatedAt: g.UpdatedAt}
	if g.Status == "completed" {
		view.OutputURL = public.OutputURL
	}
	if activeGeneration(g) {
		view.PollAfterSeconds = 5
	}
	return view
}
func (s *Server) scopedGeneration(in generationToolInput) (Generation, error) {
	g, ok := s.job(in.GenerationID)
	if !ok || g.Deleted || g.ProjectID != in.ProjectID {
		return Generation{}, toolProblem("generation_not_found", "Generation not found in this project.", false)
	}
	return g, nil
}
func (s *Server) draftToolView(draft generationDraft) draftToolOutput {
	_, submitted := s.job(draft.GenerationID)
	return draftToolOutput{Draft: draft, Submitted: submitted, Validation: "Parameters and asset bindings validated. No GPU work is started by preparing. Runtime graph/model validation occurs during submission; preparation does not guarantee execution success."}
}

func (s *Server) registerMCPTools(server *mcp.Server) {
	addMCPTool(server, "get_capabilities", "Read supported H3 modes, parameter limits and a read-only runtime probe. Runtime probe only checks prompt nodes and reference slots, not every model or the complete graph. No GPU work.", true, false, s.mcpCapabilities)
	addMCPTool(server, "list_projects", "List non-deleted projects. Archived projects cannot accept new tasks. Use project_id empty string for the global workspace.", true, false, func(_ context.Context, in projectListInput) (toolPage[Project], error) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		items := []Project{}
		for _, p := range s.projects {
			if !p.Deleted {
				items = append(items, p)
			}
		}
		return paginateTools(items, in.Offset, in.Limit), nil
	})
	addMCPTool(server, "list_assets", "List existing assets in exactly one project, with optional name/type filters and pagination. Does not inspect the image/video content. Use returned IDs in prepare_generation.", true, false, func(_ context.Context, in assetListInput) (toolPage[Asset], error) {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if !s.validProjectLocked(in.ProjectID) {
			return toolPage[Asset]{}, toolProblem("invalid_project", "Project does not exist or is archived.", false)
		}
		items := []Asset{}
		for _, a := range s.assets {
			if a.ProjectID == in.ProjectID && (in.Type == "" || a.Type == in.Type) && strings.Contains(strings.ToLower(a.Name), strings.ToLower(in.Query)) {
				a.Filename = ""
				items = append(items, a)
			}
		}
		return paginateTools(items, in.Offset, in.Limit), nil
	})
	addMCPTool(server, "read_prompt_guide", "Read the bundled H3 prompt-writing guide. Use SKILL.md first, then references/base-en.txt or references/ref-en.txt. Returns guidance, not generation capability.", true, false, func(_ context.Context, in guideToolInput) (guideToolOutput, error) {
		body, err := skills.Read("h3-prompt-writing", in.Resource)
		return guideToolOutput{Content: body}, err
	})
	addMCPTool(server, "prepare_generation", "Validate and persist an immutable H3 draft. Does not compile a graph, copy media or start generation. Repeated request_id with identical normalized parameters returns the same draft; changed parameters conflict. A new creative version needs a new request_id.", false, false, func(ctx context.Context, in prepareToolInput) (draftToolOutput, error) {
		draft, err := s.prepareGenerationDraft(ctx, in.RequestID, generationRequest{ProjectID: in.ProjectID, Mode: in.Mode, Prompt: in.Prompt, Duration: in.Duration, AspectRatio: in.AspectRatio, Resolution: in.Resolution, Steps: in.Steps, AssetIDs: in.AssetIDs, ReferenceVideoFPS: in.ReferenceVideoFPS})
		if err != nil {
			return draftToolOutput{}, err
		}
		return s.draftToolView(draft), nil
	})
	addMCPTool(server, "get_generation_draft", "Read a durable draft after reconnect or before submission. generation_id is reserved at preparation; submitted tells whether its task exists.", true, false, func(ctx context.Context, in draftToolInput) (draftToolOutput, error) {
		draft, _, err := s.loadGenerationDraft(ctx, in.DraftID, in.ProjectID)
		if err != nil {
			return draftToolOutput{}, err
		}
		return s.draftToolView(draft), nil
	})
	addMCPTool(server, "submit_generation", "Start GPU generation from an exact draft revision when the user has authorized generation. Returns a task immediately, NOT a completed video. Repeating the same draft returns its existing task, including after reconnect/restart. Poll get_generation; use a new draft for another attempt/version.", false, false, func(ctx context.Context, in submitToolInput) (generationToolOutput, error) {
		g, err := s.submitGenerationDraft(ctx, in.DraftID, in.ProjectID, in.Revision)
		return generationToolView(g), err
	})
	addMCPTool(server, "get_generation", "Read actual video task status and result. queued/running is unfinished; completed plus output_url is deliverable. Respect poll_after_seconds. Does not submit or retry jobs.", true, false, func(_ context.Context, in generationToolInput) (generationToolOutput, error) {
		g, err := s.scopedGeneration(in)
		return generationToolView(g), err
	})
	addMCPTool(server, "cancel_generation", "Cancel one queued/running generation in the specified project using existing task cancellation. May interrupt that generation. Already-cancelled tasks return their current state; completed/failed tasks cannot be cancelled.", false, true, func(_ context.Context, in generationToolInput) (generationToolOutput, error) {
		if _, err := s.scopedGeneration(in); err != nil {
			return generationToolOutput{}, err
		}
		g, _, err := s.cancelGeneration(in.GenerationID)
		return generationToolView(g), err
	})
}
