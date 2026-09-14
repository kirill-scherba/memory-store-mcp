package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGenerateAnswerDisablesThinking guards the fix for the suggest/extract
// latency: on the Ollama endpoint the non-streaming request must ask the model
// to skip its reasoning phase, and on OpenAI-compatible endpoints (where the
// field does not exist) it must be absent.
func TestGenerateAnswerDisablesThinking(t *testing.T) {
	newServer := func(body *[]byte) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*body, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
		}))
	}

	oldURL, oldKey := llmURLOverride, llmAPIKeyOverride
	defer func() { llmURLOverride, llmAPIKeyOverride = oldURL, oldKey }()

	t.Run("ollama disables thinking", func(t *testing.T) {
		var body []byte
		srv := newServer(&body)
		defer srv.Close()
		llmURLOverride, llmAPIKeyOverride = srv.URL, ""

		if _, err := generateAnswerWithClient(srv.Client(), "test-model", nil); err != nil {
			t.Fatalf("generateAnswerWithClient: %v", err)
		}

		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		think, ok := req["think"]
		if !ok {
			t.Fatal("request has no think field; reasoning tokens would be generated and billed")
		}
		if think != false {
			t.Fatalf("think = %v, want false", think)
		}
	})

	t.Run("openai omits thinking", func(t *testing.T) {
		var body []byte
		srv := newServer(&body)
		defer srv.Close()
		llmURLOverride, llmAPIKeyOverride = srv.URL, "test-key"

		if _, err := generateAnswerWithClient(srv.Client(), "test-model", nil); err != nil {
			t.Fatalf("generateAnswerWithClient: %v", err)
		}

		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		if _, ok := req["think"]; ok {
			t.Fatal("think must not be sent to OpenAI-compatible endpoints")
		}
	})
}
