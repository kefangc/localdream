package app

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"
)

type Asset struct {
	ProjectID string    `json:"project_id,omitempty"`
	Filename  string    `json:"filename,omitempty"`
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	URL       string    `json:"url"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type Generation struct {
	ReferenceVideoPreserveDuration bool              `json:"reference_video_preserve_duration,omitempty"`
	ReferenceVideoFPS              *float64          `json:"reference_video_fps,omitempty"`
	AgentRequestFingerprint        string            `json:"agent_request_fingerprint,omitempty"`
	ProjectID                      string            `json:"project_id,omitempty"`
	Archived                       bool              `json:"archived,omitempty"`
	Deleted                        bool              `json:"deleted,omitempty"`
	Superseded                     bool              `json:"superseded,omitempty"`
	Continuity                     *ClipContext      `json:"continuity,omitempty"`
	StartedAt                      *time.Time        `json:"started_at,omitempty"`
	FinishedAt                     *time.Time        `json:"finished_at,omitempty"`
	ElapsedMS                      *int64            `json:"elapsed_ms,omitempty"`
	ProgressNode                   string            `json:"progress_node,omitempty"`
	ProgressStage                  string            `json:"progress_stage,omitempty"`
	ProgressValue                  float64           `json:"progress_value,omitempty"`
	ProgressMax                    float64           `json:"progress_max,omitempty"`
	ProgressKnown                  bool              `json:"progress_known"`
	ProgressConnected              bool              `json:"progress_connected"`
	NodeTypes                      map[string]string `json:"node_types,omitempty"`
	RecoveryNote                   string            `json:"recovery_note,omitempty"`
	Seed                           *int64            `json:"seed,omitempty"`
	PromptID                       string            `json:"prompt_id,omitempty"`
	ClientID                       string            `json:"client_id,omitempty"`
	Phase                          string            `json:"phase,omitempty"`
	AssetSnapshot                  []Asset           `json:"asset_snapshot,omitempty"`
	ID                             string            `json:"id"`
	Mode                           string            `json:"mode"`
	Status                         string            `json:"status"`
	Prompt                         string            `json:"prompt"`
	Error                          string            `json:"error,omitempty"`
	OutputURL                      string            `json:"output_url,omitempty"`
	Duration                       float64           `json:"duration"`
	AspectRatio                    string            `json:"aspect_ratio"`
	Resolution                     string            `json:"resolution"`
	Steps                          int               `json:"steps"`
	Lora                           string            `json:"lora,omitempty"`
	LoraStrength                   float64           `json:"lora_strength,omitempty"`
	AssetIDs                       []string          `json:"asset_ids"`
	Progress                       int               `json:"progress"`
	CreatedAt                      time.Time         `json:"created_at"`
	UpdatedAt                      time.Time         `json:"updated_at"`
}

type ProviderConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

type generationRequest struct {
	ReferenceVideoPreserveDuration bool         `json:"reference_video_preserve_duration,omitempty"`
	ReferenceVideoFPS              *float64     `json:"reference_video_fps,omitempty"`
	AgentJobID                     string       `json:"-"`
	AgentFingerprint               string       `json:"-"`
	ProjectID                      string       `json:"project_id,omitempty"`
	Strategy                       string       `json:"strategy,omitempty"`
	Continuity                     *ClipContext `json:"-"`
	Mode                           string       `json:"mode"`
	Prompt                         string       `json:"prompt"`
	AspectRatio                    string       `json:"aspect_ratio"`
	Resolution                     string       `json:"resolution"`
	Duration                       float64      `json:"duration"`
	Steps                          int          `json:"steps"`
	Lora                           string       `json:"lora"`
	LoraStrength                   float64      `json:"lora_strength"`
	AssetIDs                       []string     `json:"asset_ids"`
	UseAgent                       bool         `json:"use_agent"`
}

type Server struct {
	projects        []Project
	longVideos      []LongVideo
	progressStreams map[string]bool
	db              *sql.DB
	ctx             context.Context
	workers         sync.WaitGroup
	submitMu        sync.Mutex
	mu              sync.RWMutex
	assets          []Asset
	jobs            []Generation
	root            string
	comfyURL        string
	comfyInput      string
	httpClient      *http.Client
}
