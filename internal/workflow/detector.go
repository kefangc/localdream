package workflow

import (
	"encoding/json"
	"fmt"
)

type RuntimeAssetSlots struct {
	ReferenceImages      CountRange
	ReferenceVideos      CountRange
	ReferenceVideoAudios CountRange
	ReferenceAudios      CountRange
}
type runtimeObjectInfo struct {
	Input struct {
		Optional map[string][]json.RawMessage `json:"optional"`
	} `json:"input"`
}

type runtimeAutogrowOptions struct {
	Template struct {
		Min *int `json:"min"`
		Max *int `json:"max"`
	} `json:"template"`
}

func ParseRuntimeAssetSlots(
	raw []byte,
	nodeName string,
) (RuntimeAssetSlots, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return RuntimeAssetSlots{}, fmt.Errorf(
			"decode ComfyUI object info: %w",
			err,
		)
	}

	nodeRaw, ok := root[nodeName]
	if !ok {
		return RuntimeAssetSlots{}, fmt.Errorf(
			"ComfyUI object info does not contain node %q",
			nodeName,
		)
	}

	var node runtimeObjectInfo
	if err := json.Unmarshal(nodeRaw, &node); err != nil {
		return RuntimeAssetSlots{}, fmt.Errorf(
			"decode ComfyUI node %q: %w",
			nodeName,
			err,
		)
	}

	images, err := parseAutogrowRange(
		node.Input.Optional,
		"ref_images",
	)
	if err != nil {
		return RuntimeAssetSlots{}, err
	}

	videos, err := parseAutogrowRange(
		node.Input.Optional,
		"ref_videos",
	)
	if err != nil {
		return RuntimeAssetSlots{}, err
	}

	videoAudios, err := parseAutogrowRange(
		node.Input.Optional,
		"ref_video_audios",
	)

	if err != nil {
		return RuntimeAssetSlots{}, err
	}

	audios, err := parseAutogrowRange(
		node.Input.Optional,
		"ref_audios",
	)

	if err != nil {
		return RuntimeAssetSlots{}, err
	}

	return RuntimeAssetSlots{
		ReferenceImages:      images,
		ReferenceVideos:      videos,
		ReferenceVideoAudios: videoAudios,
		ReferenceAudios:      audios,
	}, nil

}

func parseAutogrowRange(
	optional map[string][]json.RawMessage,
	field string,
) (CountRange, error) {
	input, ok := optional[field]
	if !ok {
		return CountRange{}, fmt.Errorf(
			"ComfyUI object info does not contain optional input %q",
			field,
		)
	}

	if len(input) != 2 {
		return CountRange{}, fmt.Errorf(
			"ComfyUI optional input %q must contain schema and options",
			field,
		)
	}

	var schema string
	if err := json.Unmarshal(input[0], &schema); err != nil {
		return CountRange{}, fmt.Errorf(
			"decode schema for optional input %q: %w",
			field,
			err,
		)
	}

	if schema != "COMFY_AUTOGROW_V3" {
		return CountRange{}, fmt.Errorf(
			"optional input %q uses unsupported schema %q",
			field,
			schema,
		)
	}

	var options runtimeAutogrowOptions
	if err := json.Unmarshal(input[1], &options); err != nil {
		return CountRange{}, fmt.Errorf(
			"decode options for optional input %q: %w",
			field,
			err,
		)
	}

	if options.Template.Min == nil {
		return CountRange{}, fmt.Errorf(
			"optional input %q does not define template.min",
			field,
		)
	}

	if options.Template.Max == nil {
		return CountRange{}, fmt.Errorf(
			"optional input %q does not define template.max",
			field,
		)
	}

	if *options.Template.Min < 0 {
		return CountRange{}, fmt.Errorf(
			"optional input %q has negative minimum %d",
			field,
			*options.Template.Min,
		)
	}

	if *options.Template.Max < *options.Template.Min {
		return CountRange{}, fmt.Errorf(
			"optional input %q maximum %d below minimum %d",
			field,
			*options.Template.Max,
			*options.Template.Min,
		)
	}

	return CountRange{
		Min: *options.Template.Min,
		Max: *options.Template.Max,
	}, nil
}
