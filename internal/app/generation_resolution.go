package app

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const generationResolutionPattern = `^(480p|720p|[0-9]+(\.[0-9]+)?MP)$`

var generationResolutionRE = regexp.MustCompile(generationResolutionPattern)

// Share validation and canonicalization across HTTP, Agent and MCP drafts.
// Preserve fractional MP precision; rounding belongs to the pixel-size node.
func normalizeGenerationResolution(in *generationRequest) error {
	if in.Resolution == "480p" || in.Resolution == "720p" {
		return nil
	}
	if !generationResolutionRE.MatchString(in.Resolution) {
		return fmt.Errorf("请选择 480p、720p 或自定义 MP 分辨率（例如 0.75MP）")
	}
	mp, err := strconv.ParseFloat(strings.TrimSuffix(in.Resolution, "MP"), 64)
	if err != nil || math.IsNaN(mp) || math.IsInf(mp, 0) || mp < 0.1 || mp > 1.9 {
		return fmt.Errorf("自定义分辨率必须在 0.1 到 1.9 MP 之间")
	}
	in.Resolution = strconv.FormatFloat(mp, 'f', -1, 64) + "MP"
	return nil
}
