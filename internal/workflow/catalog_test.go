package workflow

import (
	"context"
	"errors"
	"testing"
)

type fakeRuntimeSlotsProvider struct {
	slots    RuntimeAssetSlots
	err      error
	nodeName string
	calls    int
}

func (f *fakeRuntimeSlotsProvider) RuntimeAssetSlots(
	_ context.Context,
	nodeName string,
) (RuntimeAssetSlots, error) {
	f.calls++
	f.nodeName = nodeName
	return f.slots, f.err
}

func TestCatalogUsesRuntimeAssetLimits(t *testing.T) {
	provider := &fakeRuntimeSlotsProvider{
		slots: RuntimeAssetSlots{
			ReferenceImages:      CountRange{Min: 0, Max: 11},
			ReferenceVideos:      CountRange{Min: 0, Max: 4},
			ReferenceVideoAudios: CountRange{Min: 0, Max: 6},
			ReferenceAudios:      CountRange{Min: 0, Max: 7},
		},
	}

	catalog := NewCatalog(provider, map[string]ModeDefinition{
		"multi_reference_video": {
			RuntimeNode: "MiniMaxH3ReferenceToVideo",
			Fallback: ModeCapability{
				Mode: "multi_reference_video",
				Assets: AssetCapability{
					Images:   CountRange{Min: 0, Max: 9},
					Videos:   CountRange{Min: 0, Max: 3},
					Audios:   CountRange{Min: 0, Max: 3},
					MinTotal: 1,
				},
			},
		},
	})

	got, err := catalog.Resolve(
		context.Background(),
		"multi_reference_video",
	)

	if err != nil {
		t.Fatalf("resolve capability: %v", err)
	}

	if got.Source != "comfyui_object_info" || got.Fallback {
		t.Fatalf("unexpected source: %+v", got)
	}

	if got.Capability.Assets.Images.Max != 11 ||
		got.Capability.Assets.Videos.Max != 4 ||
		got.Capability.Assets.Audios.Max != 7 {
		t.Fatalf("unexpected runtime capability: %+v", got.Capability)
	}

	if got.Capability.Assets.MinTotal != 1 {
		t.Fatalf("manifest semantics were lost: %+v", got.Capability)
	}
}

func TestCatalogRejectsUnknownMode(t *testing.T) {
	catalog := NewCatalog(
		&fakeRuntimeSlotsProvider{},
		map[string]ModeDefinition{},
	)

	_, err := catalog.Resolve(
		context.Background(),
		"unknown_mode",
	)
	if err == nil {
		t.Fatal("expected unknown mode to return an error")
	}
}

func TestCatalogFallsBackWhenRuntimeProviderFails(t *testing.T) {
	provider := &fakeRuntimeSlotsProvider{
		err: errors.New("ComfyUI is unavailable"),
	}

	fallback := ModeCapability{
		Mode: "multi_reference_video",
		Assets: AssetCapability{
			Images:   CountRange{Min: 0, Max: 9},
			Videos:   CountRange{Min: 0, Max: 3},
			Audios:   CountRange{Min: 0, Max: 3},
			MinTotal: 1,
		},
	}

	catalog := NewCatalog(provider, map[string]ModeDefinition{
		"multi_reference_video": {
			RuntimeNode: "MiniMaxH3ReferenceToVideo",
			Fallback:    fallback,
		},
	})

	got, err := catalog.Resolve(
		context.Background(),
		"multi_reference_video",
	)
	if err != nil {
		t.Fatalf("resolve fallback capability: %v", err)
	}

	if !got.Fallback {
		t.Fatal("expected fallback capability")
	}

	if got.Source != "manifest" {
		t.Fatalf(
			"source: got %q, want %q",
			got.Source,
			"manifest",
		)
	}

	if got.Capability.Assets.Images.Max != 9 {
		t.Fatalf(
			"fallback image max: got %d, want 9",
			got.Capability.Assets.Images.Max,
		)
	}

	if len(got.Warnings) == 0 {
		t.Fatal("expected fallback warning")
	}
}

func TestCatalogDoesNotFallbackWhenContextIsCancelled(t *testing.T) {
	provider := &fakeRuntimeSlotsProvider{
		err: context.Canceled,
	}

	catalog := NewCatalog(provider, map[string]ModeDefinition{
		"multi_reference_video": {
			RuntimeNode: "MiniMaxH3ReferenceToVideo",
			Fallback: ModeCapability{
				Mode: "multi_reference_video",
				Assets: AssetCapability{
					Images:   CountRange{Min: 0, Max: 9},
					Videos:   CountRange{Min: 0, Max: 3},
					Audios:   CountRange{Min: 0, Max: 3},
					MinTotal: 1,
				},
			},
		},
	})

	_, err := catalog.Resolve(
		context.Background(),
		"multi_reference_video",
	)
	if err == nil {
		t.Fatal("expected context cancellation to return an error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"expected context .Canceled, got %T: %v",
			err,
			err,
		)
	}
}

func TestCatalogRejectsAlreadyCanceledCotextBeforeCallingProvider(t *testing.T) {
	provider := &fakeRuntimeSlotsProvider{}

	catalog := NewCatalog(provider, map[string]ModeDefinition{
		"multi_reference_video": {
			RuntimeNode: "MiniMaxH3ReferenceToVideo",
			Fallback: ModeCapability{
				Mode: "multi_reference_video",
				Assets: AssetCapability{
					Images:   CountRange{Min: 0, Max: 9},
					Videos:   CountRange{Min: 0, Max: 3},
					Audios:   CountRange{Min: 0, Max: 3},
					MinTotal: 1,
				},
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_,err:=catalog.Resolve(ctx,"multi_reference_video")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context .Cancelled, got %v", err)

	}

	if provider.calls !=0{
		t.Errorf("provider calls: got %d, want 0", provider.calls)
	}
}
