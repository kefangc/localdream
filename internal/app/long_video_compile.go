package app

import (
	"fmt"
	"strconv"
)

func applyClipContext(graph map[string]comfyNode, c *ClipContext) error {
	// TE-Speed is optional and has version-sensitive H3 wrappers. The default
	// long-video path uses native sampling; LoRA acceleration stays independent.
	if !c.Settings.UseTESpeed {
		for id, n := range graph {
			if n.ClassType != "TESpeedMiniMaxH3" {
				continue
			}
			for _, consumer := range graph {
				for key, value := range consumer.Inputs {
					if link, ok := value.([]any); ok && len(link) == 2 && asString(link[0]) == id {
						consumer.Inputs[key] = n.Inputs["model"]
					}
				}
			}
			delete(graph, id)
		}
	}
	folder := "h3_context/localdream/" + c.SequenceID
	seen := map[string]bool{}
	for _, node := range graph {
		switch node.ClassType {
		case "MiniMaxH3ReferenceToVideo":
			node.Inputs["length"] = c.Timing.InputFrames
		case "MiniMaxH3MotionContext":
			node.Inputs["context_length"] = strconv.Itoa(c.Settings.ContextFrames)
			node.Inputs["audio_context_length"] = c.Settings.AudioContextFrames
		case "MiniMaxH3MotionContextLoadLatent":
			node.Inputs["latent_path"] = folder
			node.Inputs["clip_index"] = c.Index - 1
		case "MiniMaxH3MotionContextSaveLatent":
			node.Inputs["filename_prefix"] = folder + "/clip"
			node.Inputs["clip_index"] = c.Index
		case "MiniMaxH3MotionContextTrim":
			node.Inputs["fps"] = c.Settings.FPS
			node.Inputs["match_tail"] = c.Settings.MatchAudioTail
		case "CreateVideo":
			node.Inputs["fps"] = c.Settings.FPS
		case "SaveVideo":
			node.Inputs["filename_prefix"] = fmt.Sprintf("video/localdream/%s/clip_%03d", c.SequenceID, c.Index)
		}
		seen[node.ClassType] = true
	}
	for _, kind := range []string{"MiniMaxH3ReferenceToVideo", "MiniMaxH3MotionContext", "MiniMaxH3MotionContextLoadLatent", "MiniMaxH3MotionContextSaveLatent", "MiniMaxH3MotionContextTrim", "SaveVideo"} {
		if !seen[kind] {
			return fmt.Errorf("长视频模板缺少 %s", kind)
		}
	}
	return nil
}

// Dynamic references replace template sample media. Remove disconnected sample
// loaders and duration arithmetic, while retaining BOTH video and latent outputs.
func pruneLongGraph(graph map[string]comfyNode) {
	keep := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if keep[id] {
			return
		}
		node, ok := graph[id]
		if !ok {
			return
		}
		keep[id] = true
		for _, v := range node.Inputs {
			if link, ok := v.([]any); ok && len(link) == 2 {
				visit(asString(link[0]))
			}
		}
	}
	for id, n := range graph {
		if n.ClassType == "SaveVideo" || n.ClassType == "MiniMaxH3MotionContextSaveLatent" {
			visit(id)
		}
	}
	for id := range graph {
		if !keep[id] {
			delete(graph, id)
		}
	}
}
