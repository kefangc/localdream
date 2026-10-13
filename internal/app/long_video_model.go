package app

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// LongVideo is a durable manual sequence, not a merged media file. Each clip
// keeps its own immutable generation record and can be downloaded independently.
type LongVideo struct {
	ProjectID   string            `json:"project_id,omitempty"`
	Error       string            `json:"error,omitempty"`
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	AspectRatio string            `json:"aspect_ratio"`
	Resolution  string            `json:"resolution"`
	Settings    LongVideoSettings `json:"settings"`
	Segments    []VideoSegment    `json:"segments"`
	Paused      bool              `json:"paused"`
	Revision    int               `json:"revision"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type VideoSegment struct {
	Request generationRequest `json:"request"`
	JobID   string            `json:"job_id,omitempty"`
	Timing  ClipTiming        `json:"timing"`
}

type ClipTiming struct {
	RequestedSeconds float64 `json:"requested_seconds"`
	InputFrames      int     `json:"input_frames"`
	TrimFrames       int     `json:"trim_frames"`
	InputSeconds     float64 `json:"input_seconds"`
	EffectiveSeconds float64 `json:"effective_seconds"`
}

type ClipContext struct {
	SequenceID string            `json:"sequence_id"`
	Index      int               `json:"index"`
	Settings   LongVideoSettings `json:"settings"`
	Timing     ClipTiming        `json:"timing"`
}

type LongVideoSettings struct {
	UseTESpeed         bool    `json:"use_te_speed"`
	Strategy           string  `json:"strategy"`
	Threshold          float64 `json:"threshold"`
	ReferenceThreshold float64 `json:"reference_threshold"`
	ReferenceCount     int     `json:"reference_count"`
	MaxInputSeconds    float64 `json:"max_input_seconds"`
	DefaultSeconds     float64 `json:"default_seconds"`
	DefaultSegments    int     `json:"default_segments"`
	ContextFrames      int     `json:"context_frames"`
	AudioContextFrames int     `json:"audio_context_frames"`
	FPS                int     `json:"fps"`
	MatchAudioTail     bool    `json:"match_audio_tail"`
}

func defaultLongVideoSettings() LongVideoSettings {
	return LongVideoSettings{Strategy: "auto", Threshold: 10, ReferenceThreshold: 8, ReferenceCount: 3, MaxInputSeconds: 8, DefaultSeconds: 5, DefaultSegments: 2, ContextFrames: 22, AudioContextFrames: 24, FPS: 24, MatchAudioTail: true}
}

func (c LongVideoSettings) validate() error {
	if c.Strategy != "auto" && c.Strategy != "normal" && c.Strategy != "long" {
		return fmt.Errorf("请选择自动、普通或长视频策略")
	}
	if !positive(c.Threshold) || !positive(c.ReferenceThreshold) || c.ReferenceCount < 1 || !positive(c.MaxInputSeconds) || !positive(c.DefaultSeconds) || c.DefaultSegments < 1 {
		return fmt.Errorf("时长、参考数量与默认段数必须大于零")
	}
	// These are the actual choices exposed by the installed Motion Context node.
	valid := false
	for _, n := range []int{5, 22, 39, 56} {
		if c.ContextFrames == n {
			valid = true
		}
	}
	if !valid || c.AudioContextFrames < 0 || c.AudioContextFrames > 240 {
		return fmt.Errorf("重叠帧必须符合 H3 网格，音频上下文须为 0–240 帧")
	}
	if c.FPS != 24 {
		return fmt.Errorf("此 H3 模型原生音视频时间轴为 24 fps，不支持修改帧率")
	}
	return nil
}

func positive(v float64) bool { return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) }

// The native H3 VAE rounds up to 17k+5 frames. We expose the actual delivered
// length instead of claiming arbitrary decimals are exactly representable.
func calculateClipTiming(seconds float64, index int, c LongVideoSettings, previousFrames int) (ClipTiming, error) {
	if !positive(seconds) {
		return ClipTiming{}, fmt.Errorf("有效时长必须大于零")
	}
	trim := 0
	if index > 1 {
		for _, n := range []int{124, 107, 90, 73, 56, 39, 22, 5, 1} {
			if n <= c.ContextFrames && n <= previousFrames {
				trim = n
				break
			}
		}
		if trim == 0 {
			return ClipTiming{}, fmt.Errorf("上一片段缺少可续接的帧")
		}
	}
	needed := math.Ceil(seconds*float64(c.FPS)-1e-8) + float64(trim)
	frames := int(5 + 17*math.Ceil(math.Max(0, needed-5)/17))
	// Native node maximum, not the optional 8-second planning preference.
	if needed > 3600 || frames > 3600 || frames <= trim {
		return ClipTiming{}, fmt.Errorf("时长超出当前 H3 节点支持的 3600 帧范围")
	}
	return ClipTiming{seconds, frames, trim, float64(frames) / float64(c.FPS), float64(frames-trim) / float64(c.FPS)}, nil
}

func validateClipRequest(in *generationRequest, allowEmpty bool) error {
	if err := normalizeReferenceVideoFPS(in); err != nil {
		return err
	}
	in.Mode = "multi_reference_video"
	in.UseAgent = false
	if !allowEmpty && strings.TrimSpace(in.Prompt) == "" {
		return fmt.Errorf("请填写当前片段提示词")
	}
	if in.Steps == 0 {
		in.Steps = 50
	}
	if in.Steps < 1 || in.Steps > 100 {
		return fmt.Errorf("步数须为 1–100")
	}
	if in.LoraStrength < 0 || in.LoraStrength > 1 {
		return fmt.Errorf("LoRA 强度须为 0–1")
	}
	switch in.Lora {
	case "":
	case "ref2v_4":
		if in.Steps != 4 {
			return fmt.Errorf("4 步 LoRA 需要 4 步")
		}
	case "ref2v_8":
		if in.Steps != 8 {
			return fmt.Errorf("8 步 LoRA 需要 8 步")
		}
	default:
		return fmt.Errorf("此长视频模板使用 Ref2V 模型，请选择 Ref2V 加速参数")
	}
	return nil
}
