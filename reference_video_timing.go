package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Opt-in timing v1 leaves all existing requests untouched. H3 accepts only an
// IMAGE batch and assumes 24 fps; a sparse batch alone cannot carry its real fps.
type referenceTimingRecord struct {
	Version            int     `json:"version"`
	SourceSHA256       string  `json:"source_sha256"`
	SamplingFPS        float64 `json:"sampling_fps"`
	ModelFPS           int     `json:"model_fps"`
	SourceDuration     float64 `json:"source_duration_seconds"`
	MotionFrames       int     `json:"motion_frames_at_24fps"`
	ModelFrames        int     `json:"model_frames"`
	TrailingHoldFrames int     `json:"trailing_hold_frames"`
	PreparedDuration   float64 `json:"prepared_duration_seconds"`
}

func referenceModelFrames(duration float64) (int, int, error) {
	if math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 15.1 {
		return 0, 0, fmt.Errorf("duration-preserving reference must be a corresponding clip of at most 15 seconds")
	}
	motionFrames := int(math.Ceil(duration*24 - 1e-6))
	frames := max(5, motionFrames)
	for frames%17 != 5 {
		frames++
	}
	return motionFrames, frames, nil
}

func referenceTool(envName, name string) (string, error) {
	if configured := os.Getenv(envName); configured != "" {
		return configured, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is required for preserved reference timing; set %s: %w", name, envName, err)
	}
	return path, nil
}

func probeReferenceVideo(path string) (duration float64, frames int, err error) {
	tool, err := referenceTool("LOCALDREAM_FFPROBE", "ffprobe")
	if err != nil {
		return 0, 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, "-v", "error", "-select_streams", "v:0", "-show_entries", "stream=duration,nb_frames:format=duration", "-of", "json", path)
	hideProcess(cmd)
	raw, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("probe reference video: %w", err)
	}
	var info struct {
		Streams []struct {
			Duration string `json:"duration"`
			Frames   string `json:"nb_frames"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err = json.Unmarshal(raw, &info); err != nil || len(info.Streams) != 1 {
		return 0, 0, fmt.Errorf("reference video has no readable video stream")
	}
	duration, err = strconv.ParseFloat(info.Streams[0].Duration, 64)
	if err != nil {
		duration, err = strconv.ParseFloat(info.Format.Duration, 64)
	}
	frames, _ = strconv.Atoi(info.Streams[0].Frames)
	return duration, frames, err
}

func (s *Server) prepareReferenceVideoTiming(assets []compiledAsset, samplingFPS float64) ([]compiledAsset, error) {
	prepared := append([]compiledAsset(nil), assets...)
	for i, asset := range prepared {
		if asset.Kind != "video" {
			continue
		}
		filename, err := prepareReferenceVideoFile(filepath.Join(s.comfyInput, asset.Filename), s.comfyInput, samplingFPS)
		if err != nil {
			return nil, err
		}
		prepared[i].Filename = filename
	}
	return prepared, nil
}

func prepareReferenceVideoFile(source, outputDir string, samplingFPS float64) (string, error) {
	if math.IsNaN(samplingFPS) || math.IsInf(samplingFPS, 0) || samplingFPS != 0 && (samplingFPS < 1 || samplingFPS > 60) {
		return "", fmt.Errorf("invalid reference sampling fps")
	}
	f, err := os.Open(source)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		return "", err
	}
	sourceHash := hex.EncodeToString(h.Sum(nil))
	key := sha256.Sum256([]byte("reference-timing-v1:" + sourceHash + ":" + strconv.FormatFloat(samplingFPS, 'g', -1, 64)))
	filename := "localdream_reference24_" + hex.EncodeToString(key[:]) + ".mp4"
	output := filepath.Join(outputDir, filename)
	duration, _, err := probeReferenceVideo(source)
	if err != nil {
		return "", err
	}
	motionFrames, modelFrames, err := referenceModelFrames(duration)
	if err != nil {
		return "", err
	}
	if cachedDuration, cachedFrames, err := probeReferenceVideo(output); err == nil && cachedFrames == modelFrames && math.Abs(cachedDuration-float64(modelFrames)/24) < 0.001 {
		return filename, nil
	}
	tool, err := referenceTool("LOCALDREAM_FFMPEG", "ffmpeg")
	if err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(outputDir, "reference-timing-*.mp4")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	temp.Close()
	defer os.Remove(tempPath)
	filters := []string{"setpts=PTS-STARTPTS"}
	if samplingFPS > 0 && samplingFPS != 24 {
		filters = append(filters, "fps="+strconv.FormatFloat(samplingFPS, 'g', -1, 64)+":start_time=0:round=near:eof_action=pass")
	}
	filters = append(filters, "fps=24:start_time=0:round=near:eof_action=pass", "tpad=stop_mode=clone:stop_duration=2")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, tool, "-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-threads", "2", "-i", source, "-map", "0:v:0", "-an", "-vf", strings.Join(filters, ","), "-frames:v", strconv.Itoa(modelFrames), "-r", "24", "-c:v", "libx264", "-preset", "veryfast", "-crf", "16", "-pix_fmt", "yuv420p", "-movflags", "+faststart", tempPath)
	hideProcess(cmd)
	if raw, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("prepare duration-preserving reference: %w: %s", err, string(raw))
	}
	preparedDuration, preparedFrames, err := probeReferenceVideo(tempPath)
	if err != nil || preparedFrames != modelFrames || math.Abs(preparedDuration-float64(modelFrames)/24) > 0.001 {
		return "", fmt.Errorf("prepared reference failed timing verification (frames=%d, duration=%v): %v", preparedFrames, preparedDuration, err)
	}
	if err := os.Rename(tempPath, output); err != nil {
		return "", err
	}
	record := referenceTimingRecord{1, sourceHash, samplingFPS, 24, duration, motionFrames, modelFrames, modelFrames - motionFrames, preparedDuration}
	raw, _ := json.MarshalIndent(record, "", "  ")
	if err := os.WriteFile(output+".timing.json", raw, 0600); err != nil {
		return "", err
	}
	return filename, nil
}
