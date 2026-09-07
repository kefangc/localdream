package workflow

import (
	"testing"
)

func TestParseRuntimeAssetSlots(t *testing.T) {
	raw := []byte(`{
		"MiniMaxH3ReferenceToVideo":{
			"input":{
				"optional":{
					"ref_images":[
						"COMFY_AUTOGROW_V3",
						{
							"template":{
							"min":0,
							"max":11
							}
						}
					],
					"ref_videos":[
						"COMFY_AUTOGROW_V3",
						{
							"template":{
							"min":0,
							"max":4
							}
						}
					],
					"ref_video_audios":[
						"COMFY_AUTOGROW_V3",
						{
							"template":{
							"min":0,
							"max":6
							}
						}
					],
					"ref_audios":[
						"COMFY_AUTOGROW_V3",
						{
							"template":{
							"min":0,
							"max":7
							}
						}
					]
				}
			}
		}
	}`)

	slots, err := ParseRuntimeAssetSlots(
		raw,
		"MiniMaxH3ReferenceToVideo",
	)

	if err != nil {
		t.Fatalf("parse runtime asset slots: %v", err)
	}

	if slots.ReferenceImages.Max != 11 {
		t.Errorf(
			"reference images max: got %d, want %d",
			slots.ReferenceImages.Max,
			11,
		)
	}

	if slots.ReferenceVideos.Max != 4 {
		t.Errorf(
			"reference video max: got %d, want %d",
			slots.ReferenceVideos.Max,
			4,
		)
	}

	if slots.ReferenceVideoAudios.Max != 6 {
		t.Errorf(
			"reference video audio max: got %d, want %d",
			slots.ReferenceVideoAudios.Max,
			6,
		)
	}

	if slots.ReferenceAudios.Max != 7 {
		t.Errorf(
			"standalone audio max: got %d, want %d",
			slots.ReferenceAudios.Max,
			7,
		)
	}
}

func TestParseRuntimeAssetSlotsRejectsNonAutogrowInput(t *testing.T) {
	raw := []byte(`{
"MiniMaxH3ReferenceToVideo":{
"input":{
"optional":{
"ref_images":[
"STRING",
{}
]
}
}
}
}`)
	_, err := ParseRuntimeAssetSlots(
		raw,
		"MiniMaxH3ReferenceToVideo",
	)
	if err == nil {
		t.Errorf("expected parse to reject non autogrow input")
	}
}

func TestParseRuntimeAssetSlotsRejectsMissingNode(t *testing.T) {
	raw := []byte(`{
"SomeOtherNode":{
"input":{
"optional":{}
}
}
}`)
	_, err := ParseRuntimeAssetSlots(
		raw,
		"MiniMaxH3ReferenceToVideo",
	)
	if err == nil {
		t.Fatal("expected parse to reject missing node")
	}
}
