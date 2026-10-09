package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type toolFault struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (e *toolFault) Error() string { return e.Message }
func toolProblem(code, message string, retryable bool) *toolFault {
	return &toolFault{Code: code, Message: message, Retryable: retryable}
}

// Drafts are immutable. A changed plan gets a new request_id and draft_id.
// One draft identifies exactly one generation, including after reconnect/restart.
type generationDraft struct {
	ID           string            `json:"draft_id"`
	GenerationID string            `json:"generation_id"`
	Revision     int               `json:"revision"`
	Request      generationRequest `json:"request"`
	CreatedAt    time.Time         `json:"created_at"`
}

func draftIdentity(projectID, requestID string) string {
	b, _ := json.Marshal([]string{projectID, requestID})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:16])
}

func (s *Server) prepareGenerationDraft(ctx context.Context, requestID string, in generationRequest) (generationDraft, error) {
	if !clientTurnIDPattern.MatchString(requestID) {
		return generationDraft{}, toolProblem("invalid_request_id", "request_id must contain 8–128 letters, digits, underscores or hyphens. Reuse it only for retries of the same draft.", false)
	}
	in.Strategy = "normal"
	if in.AssetIDs == nil {
		in.AssetIDs = []string{}
	}
	if err := normalizeReferenceVideoFPS(&in); err != nil {
		return generationDraft{}, toolProblem("invalid_reference_video_fps", err.Error(), false)
	}
	if err := normalizeGenerationResolution(&in); err != nil {
		return generationDraft{}, toolProblem("invalid_resolution", err.Error(), false)
	}
	if in.Steps == 0 {
		in.Steps = 50
	}
	// Fixed identity/fingerprint use normalized parameters, not model prose.
	raw, err := json.Marshal(in)
	if err != nil {
		return generationDraft{}, err
	}
	h := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(h[:])
	identity := draftIdentity(in.ProjectID, requestID)
	draft := generationDraft{ID: "draft_" + identity, GenerationID: "job_mcp_" + identity, Revision: 1, Request: in, CreatedAt: time.Now().UTC()}
	// Return a previous immutable draft even if a referenced asset later disappeared.
	previous, previousFingerprint, err := s.loadGenerationDraft(ctx, draft.ID, in.ProjectID)
	if err == nil {
		if previousFingerprint != fingerprint {
			return generationDraft{}, toolProblem("request_id_conflict", "request_id already belongs to different parameters; use a new request_id for a new draft.", false)
		}
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return generationDraft{}, err
	}
	selected, err := s.selectedAgentAssets(agentTurnInput{ProjectID: in.ProjectID, AssetIDs: in.AssetIDs})
	if err != nil {
		return generationDraft{}, toolProblem("invalid_assets_or_project", err.Error(), false)
	}
	if err := validateAgentDraft(in, selected); err != nil {
		return generationDraft{}, toolProblem("invalid_generation", err.Error(), false)
	}
	raw, err = json.Marshal(draft)
	if err != nil {
		return generationDraft{}, err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO generation_drafts(id,fingerprint,payload) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING", draft.ID, fingerprint, string(raw))
	if err != nil {
		return generationDraft{}, err
	}
	// Concurrent identical requests return the same durable record.
	stored, storedFingerprint, err := s.loadGenerationDraft(ctx, draft.ID, in.ProjectID)
	if err != nil {
		return generationDraft{}, err
	}
	if storedFingerprint != fingerprint {
		return generationDraft{}, toolProblem("request_id_conflict", "request_id already belongs to different parameters.", false)
	}
	return stored, nil
}

func (s *Server) loadGenerationDraft(ctx context.Context, id, projectID string) (generationDraft, string, error) {
	if s.db == nil {
		return generationDraft{}, "", fmt.Errorf("database is not initialized")
	}
	var raw, fingerprint string
	if err := s.db.QueryRowContext(ctx, "SELECT payload,fingerprint FROM generation_drafts WHERE id=?", id).Scan(&raw, &fingerprint); err != nil {
		return generationDraft{}, "", err
	}
	var draft generationDraft
	if err := json.Unmarshal([]byte(raw), &draft); err != nil {
		return generationDraft{}, "", err
	}
	if draft.Request.ProjectID != projectID {
		return generationDraft{}, "", sql.ErrNoRows
	}
	return draft, fingerprint, nil
}

func (s *Server) submitGenerationDraft(ctx context.Context, id, projectID string, revision int) (Generation, error) {
	draft, fingerprint, err := s.loadGenerationDraft(ctx, id, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return Generation{}, toolProblem("draft_not_found", "Draft not found in this project. Call prepare_generation first.", false)
	}
	if err != nil {
		return Generation{}, err
	}
	if revision != draft.Revision {
		return Generation{}, toolProblem("draft_revision_conflict", "Read the draft and submit its exact revision.", false)
	}
	if previous, ok := s.job(draft.GenerationID); ok {
		if previous.ProjectID != projectID {
			return Generation{}, toolProblem("generation_project_changed", "The task for this draft has moved to another project. Query it using its current project; this draft cannot submit another task.", false)
		}
		if previous.AgentRequestFingerprint != fingerprint {
			return Generation{}, toolProblem("generation_identity_conflict", "Existing generation does not match this draft.", false)
		}
		if previous.Deleted {
			return Generation{}, toolProblem("generation_deleted", "The task for this draft was deleted. Prepare a new draft to generate again.", false)
		}
		return previous, nil
	}
	selected, err := s.selectedAgentAssets(agentTurnInput{ProjectID: projectID, AssetIDs: draft.Request.AssetIDs})
	if err != nil {
		return Generation{}, toolProblem("invalid_assets_or_project", err.Error(), false)
	}
	if err := validateAgentDraft(draft.Request, selected); err != nil {
		return Generation{}, toolProblem("invalid_generation", err.Error(), false)
	}
	if err := ctx.Err(); err != nil {
		return Generation{}, err
	}
	in := draft.Request
	in.AgentJobID, in.AgentFingerprint = draft.GenerationID, fingerprint
	g, status, err := s.createGeneration(in)
	if err != nil {
		return Generation{}, toolProblem("generation_submission_failed", err.Error(), status >= 500)
	}
	return g, nil
}
