package workflow

import "fmt"

type CountRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// 不同模型的接受素材数量
// AssetCapility describes the asset limits for one generation mode
type AssetCapability struct {
	Images   CountRange `json:"images"`
	Videos   CountRange `json:"videos"`
	Audios   CountRange `json:"audios"`
	MinTotal int        `json:"min_total"`
}

type ModeCapability struct {
	Mode   string          `json:"mode"`
	Assets AssetCapability `json:"assets"`
}

// 传入的素材
type AssetCounts struct {
	Images int
	Videos int
	Audios int
}

type ValidationError struct {
	Code    string
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func (c ModeCapability) Validate(counts AssetCounts) error {
	checks := []struct {
		field   string
		count   int
		allowed CountRange
	}{
		{
			field:   "images",
			count:   counts.Images,
			allowed: c.Assets.Images,
		},
		{
			field:   "videos",
			count:   counts.Videos,
			allowed: c.Assets.Videos,
		},
		{
			field:   "audios",
			count:   counts.Audios,
			allowed: c.Assets.Audios,
		},
	}
	for _, check := range checks {
		if err := validateCount(check.field, check.count, check.allowed); err != nil {
			return err
		}
	}
	total := counts.Images + counts.Videos + counts.Audios
	if total < c.Assets.MinTotal {
		return &ValidationError{
			Code:  "asset_total_below_minimum",
			Field: "assets",
			Message: fmt.Sprintf(
				"total asset count must be at least (%d)",
				c.Assets.MinTotal,
			),
		}
	}
	return nil
}

func validateCount(field string, count int, allowed CountRange) error {
	if count < 0 {
		return &ValidationError{
			Code:    "asset_count_invalid",
			Field:   field,
			Message: fmt.Sprintf("%s count must not be negative", field),
		}
	}

	if count < allowed.Min {
		return &ValidationError{
			Code:  "asset_minimum_not_met",
			Field: field,
			Message: fmt.Sprintf(
				"%s count must be at least (%d)", field, allowed.Min,
			),
		}
	}

	if count > allowed.Max {
		return &ValidationError{
			Code:  "asset_limit_exceeded",
			Field: field,
			Message: fmt.Sprintf(
				"%s count must not exceed %d", field, allowed.Max),
		}
	}
	return nil
}
