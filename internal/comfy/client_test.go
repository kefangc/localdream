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
