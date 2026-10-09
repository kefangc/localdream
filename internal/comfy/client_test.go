package comfy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientObjectInfo(t *testing.T) {
	const nodeName = "MiniMaxH3ReferenceToVideo"
	const response = `{"MiniMaxH3ReferenceToVideo":{"input":{"optional":{}}}}`

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf(
					"Method got: %q, want: %q",
					r.Method,
					http.MethodGet,
				)
			}

			wantPath := "/object_info/" + nodeName
			if r.URL.Path != wantPath {
				t.Errorf(
					"Path got: %q, want: %q",
					r.URL.Path,
					wantPath,
				)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, response)

		},
	))
	defer server.Close()

	client := NewClient(
		server.URL+"/",
		server.Client(),
	)

	got, err := client.ObjectInfo(
		context.Background(),
		nodeName,
	)

	if err != nil {
		t.Fatalf("get object info: %v", err)
	}

	if string(got) != response {
		t.Fatalf("response got: %q, want: %q",
			string(got),
			response,
		)
	}
}

func TestClientObjectInfoRejectsNonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			http.Error(
				w,
				"ComfyUI is starting",
				http.StatusServiceUnavailable,
			)
		},
	))

	defer server.Close()
	client := NewClient(server.URL, server.Client())

	_, err := client.ObjectInfo(
		context.Background(),
		"MiniMaxH3ReferenceToVideo",
	)

	if err == nil {
		t.Fatalf("expected non-success status to return an error")
	}

	for _, expected := range []string{
		"503 Service Unavailable",
		"ComfyUI is starting",
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf(
				"error %q does not contain %q",
				err.Error(),
				expected,
			)
		}
	}
}

func TestClientObjectInfoHonorsCancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	))

	defer server.Close()

	client := NewClient(server.URL, server.Client())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.ObjectInfo(
		ctx,
		"MiniMaxH3ReferenceToVideo",
	)

	if err == nil {
		t.Fatalf("expected cancelled context to return an error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %T: %v",
			err,
			err,
		)
	}
}

func TestClientObjectInfoRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(
				w,
				strings.Repeat(
					"x",
					int(maxObjectInfoResponseBytes)+1,
				),
			)
		},
	))
	defer server.Close()

	client := NewClient(server.URL, server.Client())

	_, err := client.ObjectInfo(
		context.Background(),
		"MiniMaxH3ReferenceToVideo",
	)
	if err == nil {
		t.Fatalf("expected oversized response to return an error")
	}

	if !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestClientRuntimeAssetSlots(t *testing.T) {
	const response = `{
		"MiniMaxH3ReferenceToVideo": {
			"input": {
				"optional": {
					"ref_images":[
						"COMFY_AUTOGROW_V3",
						{"template":{"min":0,"max":11}}
					],
					"ref_videos": [
						"COMFY_AUTOGROW_V3",
						{"template":{"min":0,"max":4}}
					],
					"ref_video_audios": [
						"COMFY_AUTOGROW_V3",
						{"template":{"min":0,"max":6}}
					],
					"ref_audios": [
						"COMFY_AUTOGROW_V3",
						{"template":{"min":0,"max":7}}
					]
				}
			}
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, response)
		},
	))

	defer server.Close()

	client := NewClient(server.URL, server.Client())

	slots, err := client.RuntimeAssetSlots(
		context.Background(),
		"MiniMaxH3ReferenceToVideo",
	)

	if err != nil {
		t.Fatalf("get runtime asset slots :%v", err)
	}

	if slots.ReferenceImages.Max != 11 {
		t.Errorf(
			"Reference images max: got %d, want 11",
			slots.ReferenceImages.Max,
		)
	}

	if slots.ReferenceVideos.Max != 4 {
		t.Errorf(
			"Reference video max: got %d, want 4",
			slots.ReferenceVideos.Max,
		)
	}

	if slots.ReferenceVideoAudios.Max != 6 {
		t.Errorf(
			"Reference video audio max: got %d, want 6",
			slots.ReferenceVideoAudios.Max,
		)
	}

	if slots.ReferenceAudios.Max != 7 {
		t.Errorf(
			"Reference audio max: got %d, want 7",
			slots.ReferenceAudios.Max,
		)
	}
}

func TestClientRuntimeAssetSlotsWrapsParserError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{}`)
		},
	))
	defer server.Close()

	client := NewClient(server.URL, server.Client())

	_, err := client.RuntimeAssetSlots(
		context.Background(),
		"MiniMaxH3ReferenceToVideo",
	)

	if err == nil {
		t.Fatalf("expected parser error")
	}

	for _, expected := range []string{
		"parse ComfyUI runtime asset slots",
		`does not contain node "MiniMaxH3ReferenceToVideo"`,
	} {
		if !strings.Contains(err.Error(), expected) {
			t.Fatalf(
				"error %q does not contain %q",
				err.Error(),
				expected,
			)
		}
	}

}
