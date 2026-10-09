package responses

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEndpoint(t *testing.T) {
	for input, want := range map[string]string{
		"https://api.example.test":              "https://api.example.test/v1/responses",
		"https://api.example.test/v1":           "https://api.example.test/v1/responses",
		"https://api.example.test/v1/responses": "https://api.example.test/v1/responses",
	} {
		got, err := Endpoint(input)
		if err != nil || got != want {
			t.Fatalf("Endpoint(%q) = %q, %v", input, got, err)
		}
	}
}

func TestCreatePreservesFunctionItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected request: %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "test-model" || body["store"] != false {
			t.Errorf("bad request body: %#v", body)
		}
		_, _ = w.Write([]byte(`{"id":"resp_1","status":"completed","output":[{"type":"reasoning","id":"reason_1"},{"type":"function_call","call_id":"call_1","name":"load_skill","arguments":"{\"name\":\"h3-prompt-writing\"}"}]}`))
	}))
	defer server.Close()
	response, err := (&Client{}).Create(context.Background(), server.URL, "test-key", Request{Model: "test-model", Input: []any{"hi"}, Store: false})
	if err != nil {
		t.Fatal(err)
	}
	calls, err := response.Calls()
	if err != nil || len(calls) != 1 || calls[0].CallID != "call_1" {
		t.Fatalf("unexpected calls: %#v %v", calls, err)
	}
	if !strings.Contains(string(response.Output[0]), "reason_1") {
		t.Fatal("reasoning item was lost")
	}
}
