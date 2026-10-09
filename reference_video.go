package main

import (
	"fmt"
	"math"
)

const defaultReferenceVideoFPS = 7.0

// Missing values on NEW requests use 7 fps. Zero explicitly preserves source
// cadence. This controls reference decoding only; H3's output stays at 24 fps.
func normalizeReferenceVideoFPS(in *generationRequest) error {
	fps := defaultReferenceVideoFPS
	if in.ReferenceVideoFPS != nil {
		fps = *in.ReferenceVideoFPS
	}
	if math.IsNaN(fps) || math.IsInf(fps, 0) || fps != 0 && (fps < 1 || fps > 60) {
		return fmt.Errorf("参考视频 FPS 须为 1–60，或设为 0 使用源帧率")
	}
	// Own a fresh value, so copies of drafts cannot change a persisted job.
	in.ReferenceVideoFPS = &fps
	return nil
}

// Old persisted jobs/plans have no FPS field and were decoded at source rate.
// Preserve that behavior on recovery, retry and parameter reuse.
func legacyReferenceVideoFPS(fps *float64) *float64 {
	value := 0.0
	if fps != nil {
		value = *fps
	}
	return &value
}
