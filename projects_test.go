package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func projectRequest(t *testing.T, handler http.HandlerFunc, method, path string, body any, want int) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	handler(w, httptest.NewRequest(method, path, bytes.NewReader(raw)))
	if w.Code != want {
		t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
	}
	return w
}

func TestProjectSidebarLifecycle(t *testing.T) {
	s := sequenceTestServer(t)
	w := projectRequest(t, s.projectsHandler, "GET", "/api/projects", nil, 200)
	if w.Body.String() != "[]\n" {
		t.Fatal("empty project list must be []")
	}
	w = projectRequest(t, s.projectsHandler, "POST", "/api/projects", Project{Name: "test"}, 201)
	var p Project
	json.Unmarshal(w.Body.Bytes(), &p)
	path := "/api/projects/" + p.ID
	projectRequest(t, s.projectHandler, "PUT", path, Project{Name: "海岸线", Description: "清晨"}, 200)
	projectRequest(t, s.projectHandler, "POST", path+"/archive", nil, 200)
	if s.validProjectLocked(p.ID) {
		t.Fatal("archived project accepted a new creation")
	}
	projectRequest(t, s.projectHandler, "POST", path+"/restore", nil, 200)
	s.assets = []Asset{{ID: "asset", ProjectID: p.ID}}
	s.jobs = []Generation{{ID: "job", ProjectID: p.ID}}
	projectRequest(t, s.projectHandler, "DELETE", path, nil, 200)
	if len(s.assets) != 1 || len(s.jobs) != 1 {
		t.Fatal("project removal deleted contents")
	}
	if w = projectRequest(t, s.projectsHandler, "GET", "/api/projects", nil, 200); w.Body.String() != "[]\n" {
		t.Fatal("removed project still in sidebar")
	}
}

func TestCreationProjectOwnership(t *testing.T) {
	s := sequenceTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.ctx = ctx // no inference is launched
	s.projects = []Project{{ID: "project_test", Name: "test"}}
	for _, id := range []string{"", "project_test"} {
		in := generationRequest{ProjectID: id, Mode: "text_to_video", Prompt: "A quiet coastline", Duration: 5, Steps: 24, AspectRatio: "16:9", Resolution: "480p", Strategy: "normal"}
		w := projectRequest(t, s.generationsHandler, "POST", "/api/generations", in, 202)
		s.workers.Wait()
		var g Generation
		json.Unmarshal(w.Body.Bytes(), &g)
		if g.ProjectID != id {
			t.Fatal("creation acquired the wrong project")
		}
		plan := sequenceInput{ProjectID: id, Title: "测试", AspectRatio: "16:9", Resolution: "480p", Segments: []generationRequest{{Prompt: "Coast", Duration: 5}}}
		projectRequest(t, s.longVideosHandler, "POST", "/api/long-videos", plan, 201)
		if s.longVideos[0].ProjectID != id {
			t.Fatal("long-video plan lost project")
		}
		s.longVideos[0].Paused = false
		g2, err := s.claimSequenceClip(0)
		if err != nil || g2 == nil || g2.ProjectID != id {
			t.Fatalf("segment project: %+v %v", g2, err)
		}
	}
}
