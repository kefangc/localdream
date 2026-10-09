export type Asset = {
  id: string;
  name: string;
  type: string;
  url: string;
  size: number;
  project_id?: string;
};
export type Mode =
  | 'text_to_video'
  | 'image_to_video'
  | 'first_last_frame'
  | 'multi_reference_video';
export type View = 'create' | 'assets' | 'history';
export type Job = {
  reference_video_fps?: number;
  continuity?: { sequence_id: string; index: number; timing: ClipTiming };
  id: string;
  mode: Mode;
  status: string;
  prompt: string;
  duration: number;
  aspect_ratio: string;
  resolution: string;
  steps: number;
  lora?: string;
  lora_strength?: number;
  progress: number;
  progress_known?: boolean;
  progress_connected?: boolean;
  progress_stage?: string;
  progress_value?: number;
  progress_max?: number;
  created_at?: string;
  started_at?: string;
  finished_at?: string;
  elapsed_ms?: number;
  error?: string;
  output_url?: string;
  project_id?: string;
  archived?: boolean;
  deleted?: boolean;
  superseded?: boolean;
  asset_ids: string[];
};
export type GenerationOptions = {
  reference_video_fps: number;
  project_id?: string;
  strategy?: 'auto' | 'normal' | 'long';
  mode: Mode;
  duration: number;
  aspect_ratio: string;
  resolution: string;
  steps: number;
  lora: string;
  lora_strength: number;
  use_agent: boolean;
};
export type Provider = { base_url: string; api_key: string; model: string };
export type Project = {
  archived?: boolean;
  deleted?: boolean;
  id: string;
  name: string;
  description?: string;
  created_at: string;
  updated_at: string;
};
export type Intent = Partial<GenerationOptions> & {
  prompt?: {
    subject?: string;
    motion?: string;
    camera?: string;
    environment?: string;
    constraints?: string;
  };
};
export type Notice = { kind: 'error' | 'success'; message: string } | null;

export type LongVideoSettings = {
  use_te_speed: boolean;
  strategy: 'auto' | 'normal' | 'long';
  threshold: number;
  reference_threshold: number;
  reference_count: number;
  max_input_seconds: number;
  default_seconds: number;
  default_segments: number;
  context_frames: number;
  audio_context_frames: number;
  fps: number;
  match_audio_tail: boolean;
};
export type ClipTiming = {
  requested_seconds: number;
  input_frames: number;
  trim_frames: number;
  input_seconds: number;
  effective_seconds: number;
};
export type SegmentRequest = GenerationOptions & { prompt: string; asset_ids: string[] };
export type LongVideo = {
  project_id?: string;
  error?: string;
  id: string;
  title: string;
  aspect_ratio: string;
  resolution: string;
  settings: LongVideoSettings;
  segments: { request: SegmentRequest; job_id?: string; timing: ClipTiming }[];
  paused: boolean;
  revision: number;
  status: string;
  completed: number;
  actual_seconds: number;
};
export type SequenceInput = {
  project_id?: string;
  title: string;
  aspect_ratio: string;
  resolution: string;
  segments: SegmentRequest[];
  revision?: number;
  start: boolean;
};
