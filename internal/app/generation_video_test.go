package app

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestGenerationVideoSubfolderCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, subfolder string
		allowed         bool
	}{
		{"windows", `video\localdream\sequence_test`, true},
		{"forward_slashes", "video/localdream/sequence_test", true},
		{"parent_traversal", `video\..\secret`, false},
		{"absolute", `\video\secret`, false},
		{"drive_path", `C:\video\secret`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.URL.Path != "/view" || r.URL.Query().Get("subfolder") != "video/localdream/sequence_test" || r.URL.Query().Get("filename") != "clip_001.mp4" || r.URL.Query().Get("type") != "output" {
					t.Errorf("unexpected upstream URL: %s", r.URL)
				}
				if r.Header.Get("Range") != "bytes=0-3" {
					t.Error("Range header was not forwarded")
				}
				w.Header().Set("Content-Type", "video/mp4")
				w.Header().Set("Content-Range", "bytes 0-3/8")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("test"))
			}))
			defer upstream.Close()
			query := url.Values{"filename": {"clip_001.mp4"}, "subfolder": {tc.subfolder}, "type": {"output"}}
			s := &Server{comfyURL: upstream.URL, jobs: []Generation{{ID: "clip", Status: "completed", OutputURL: upstream.URL + "/view?" + query.Encode()}}}
			r := httptest.NewRequest(http.MethodGet, "/api/generations/clip/video", nil)
			r.Header.Set("Range", "bytes=0-3")
			w := httptest.NewRecorder()
			s.generationVideo(w, r, "clip")
			if tc.allowed {
				if w.Code != http.StatusPartialContent || w.Body.String() != "test" || w.Header().Get("Content-Range") != "bytes 0-3/8" {
					t.Fatalf("video response: %d %s", w.Code, w.Body)
				}
			} else if w.Code != http.StatusBadGateway || called {
				t.Fatalf("unsafe path reached upstream: status=%d called=%v", w.Code, called)
			}
		})
	}
}
