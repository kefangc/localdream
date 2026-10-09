package workflow

import (
	"context"
	"errors"
	"fmt"
)

type RuntimeSlotsProvider interface {
	RuntimeAssetSlots(
		ctx context.Context,
		nodeName string,
	) (RuntimeAssetSlots, error)
}

type ModeDefinition struct {
	RuntimeNode string
	Fallback    ModeCapability
}

type ResolvedCapability struct {
	Capability   ModeCapability    `json:"capability"`
	RuntimeSlots RuntimeAssetSlots `json:"runtime_slots"`
	Source       string            `json:"source"`
	Fallback     bool              `json:"fallback"`
	Warnings     []string          `json:"warnings"`
}

type Catalog struct {
	providers RuntimeSlotsProvider
	modes     map[string]ModeDefinition
}

func NewCatalog(
	provider RuntimeSlotsProvider,
	modes map[string]ModeDefinition,
) *Catalog {
	return &Catalog{
		providers: provider,
		modes:     modes,
	}
}

func (c *Catalog) Resolve(
	ctx context.Context,
	mode string,
) (ResolvedCapability, error) {
	if err := ctx.Err(); err != nil {
		return ResolvedCapability{}, fmt.Errorf(
			"resolve capability for mode %q: %w",
			mode,
			err,
		)
	}
	definition, ok := c.modes[mode]
	if !ok {
		return ResolvedCapability{}, fmt.Errorf(
			"unknown workflow mode %q",
			mode,
		)
	}

	if definition.RuntimeNode == "" {
		return ResolvedCapability{}, fmt.Errorf(
			"workflow mode %q has no runtime node",
			mode,
		)
	}

	if c.providers == nil {
		return ResolvedCapability{}, fmt.Errorf(
			"runtime capability provider is not configured",
		)
	}

	slots, err := c.providers.RuntimeAssetSlots(
		ctx,
		definition.RuntimeNode,
	)

	if err != nil {
		if errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			return ResolvedCapability{}, fmt.Errorf(
				"resolve runtime capability for mode %q: %w",
				mode,
				err,
			)
		}

		return c.manifestFallback(
			mode,
			definition,
			err,
		), nil
	}

	capability := definition.Fallback
	capability.Mode = mode
	capability.Assets.Images = slots.ReferenceImages
	capability.Assets.Videos = slots.ReferenceVideos
	capability.Assets.Audios = slots.ReferenceAudios

	return ResolvedCapability{
		Capability:   capability,
		RuntimeSlots: slots,
		Source:       "comfyui_object_info",
		Fallback:     false,
		Warnings:     []string{},
	}, nil
}

func (c *Catalog) manifestFallback(
	mode string,
	definition ModeDefinition,
	cause error,
) ResolvedCapability {
	capability := definition.Fallback
	capability.Mode = mode

	warning := fmt.Sprintf(
		"runtime capability unavailable; using manifest fallback: %v",
		cause,
	)

	return ResolvedCapability{
		Capability:   capability,
		RuntimeSlots: RuntimeAssetSlots{},
		Source:       "manifest",
		Fallback:     true,
		Warnings:     []string{warning},
	}
}
