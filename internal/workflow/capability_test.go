package workflow

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestModeCapabilityValidate(t *testing.T) {
	multiReference := ModeCapability{
		Mode: "multi_reference_video",
		Assets: AssetCapability{
			Images:   CountRange{Min: 0, Max: 9},
			Videos:   CountRange{Min: 0, Max: 3},
			Audios:   CountRange{Min: 0, Max: 3},
			MinTotal: 1,
		},
	}
	imageToVideo := ModeCapability{
		Mode: "image_to_video",
		Assets: AssetCapability{
			Images:   CountRange{Min: 1, Max: 1},
			Videos:   CountRange{Min: 0, Max: 0},
			Audios:   CountRange{Min: 0, Max: 0},
			MinTotal: 1,
		},
	}
	tests := []struct {
		name       string
		capability ModeCapability
		counts     AssetCounts
		wantCode   string
		wantField  string
	}{
		{
			name:       "accepts counts at upper limits",
			capability: multiReference,
			counts:     AssetCounts{Images: 9, Videos: 3, Audios: 3},
		},
		{
			name:       "rejects too many images",
			capability: multiReference,
			counts:     AssetCounts{Images: 10},
			wantCode:   "asset_limit_exceeded",
			wantField:  "images",
		},
		{
			name:       "rejects too many videos",
			capability: multiReference,
			counts:     AssetCounts{Videos: 4},
			wantCode:   "asset_limit_exceeded",
			wantField:  "videos",
		},
		{
			name:       "rejects too many audios",
			capability: multiReference,
			counts:     AssetCounts{Audios: 4},
			wantCode:   "asset_limit_exceeded",
			wantField:  "audios",
		},
		{
			name:       "rejects empty asset selection",
			capability: multiReference,
			counts:     AssetCounts{},
			wantCode:   "asset_total_below_minimum",
			wantField:  "assets",
		},
		{
			name:       "reject negative counts",
			capability: multiReference,
			counts:     AssetCounts{Images: -1, Videos: 1},
			wantCode:   "asset_count_invalid",
			wantField:  "images",
		},
		{
			name:       "reject count below type minimum",
			capability: imageToVideo,
			counts:     AssetCounts{},
			wantCode:   "asset_minimum_not_met",
			wantField:  "images",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.capability.Validate(test.counts)

			if test.wantCode == "" {
				if err != nil {
					t.Fatalf("expected validation to pass,got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected validation error %q", test.wantCode)
			}

			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError,got %T: %v", err, err)
			}

			if validationErr.Code != test.wantCode {
				t.Fatalf("error code: got %q, want %q", validationErr.Code, test.wantCode)
			}

			if validationErr.Field != test.wantField {
				t.Fatalf("error field: got %q, want %q", validationErr.Field, test.wantField)
			}
		})
	}
}

func TestParseAutogrowRangeRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		optional    map[string][]json.RawMessage
		wantMessage string
	}{
		{
			name:        "missing field",
			optional:    map[string][]json.RawMessage{},
			wantMessage: `does not contain optional input "ref_images"`,
		},
		{
			name: "incomplete tuple",
			optional: map[string][]json.RawMessage{
				"ref_images": {
					json.RawMessage(`"COMFY_AUTOGROW_V3"`),
				},
			},
			wantMessage: "must contain schema and options",
		},
		{
			name: "invalid schema JSON",
			optional: map[string][]json.RawMessage{
				"ref_images": {
					json.RawMessage(`invalid`),
					json.RawMessage(`{}`),
				},
			},
			wantMessage: "decode schema",
		},
		{
			name: "unsupported schema",
			optional: map[string][]json.RawMessage{
				"ref_images": {
					json.RawMessage(`"STRING"`),
					json.RawMessage(`{}`),
				},
			},
			wantMessage: `uses unsupported schema "STRING"`,
		},
		{
			name: "invalid options JSON",
			optional: map[string][]json.RawMessage{
				"ref_images": {
					json.RawMessage(`"COMFY_AUTOGROW_V3"`),
					json.RawMessage(`invalid`),
				},
			},
			wantMessage: "decode options",
		},
		{
			name: "missing minimum",
			optional: map[string][]json.RawMessage{
				"ref_images": {
					json.RawMessage(`"COMFY_AUTOGROW_V3"`),
					json.RawMessage(`{"template":{"max":9}}`),
				},
			},
			wantMessage: "does not define template.min",
		},
		{
			name: "missing maximum",
			optional: map[string][]json.RawMessage{
				"ref_images": {
					json.RawMessage(`"COMFY_AUTOGROW_V3"`),
					json.RawMessage(`{"template":{"min":0}}`),
				},
			},
			wantMessage: "does not define template.max",
		},
		{
			name: "negative minimum",
			optional: map[string][]json.RawMessage{
				"ref_images": {
					json.RawMessage(`"COMFY_AUTOGROW_V3"`),
					json.RawMessage(`{"template":{"min":-1,"max":9}}`),
				},
			},
			wantMessage: "has negative minimum -1",
		},
		{
			name: "maximum below minimum",
			optional: map[string][]json.RawMessage{
				"ref_images": {
					json.RawMessage(`"COMFY_AUTOGROW_V3"`),
					json.RawMessage(`{"template":{"min":5,"max":3}}`),
				},
			},
			wantMessage: "maximum 3 below minimum 5",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseAutogrowRange(
				test.optional,
				"ref_images",
			)
			if err == nil {
				t.Fatalf("expected error containing %q",
					test.wantMessage,
				)
			}

			if !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf(
					"expected got %q, want substring %q",
					err.Error(),
					test.wantMessage,
				)
			}
		})
	}


}

func TestParseAutogrowRangeAcceptsZeroRange(t *testing.T) {
	optional := map[string][]json.RawMessage{
		"ref_images": {
			json.RawMessage(`"COMFY_AUTOGROW_V3"`),
			json.RawMessage(`{"template":{"min":0,"max":0}}`),
		},
	}

	got, err := parseAutogrowRange(optional, "ref_images")
	if err != nil {
		t.Fatalf("parse zero range: %v", err)
	}

	want := CountRange{Min: 0, Max: 0}
	if got != want {
		t.Fatalf("range: got %+v, want %+v", got, want)
	}
}