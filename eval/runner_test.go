package taxeval

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCallOpenAISuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		var req chatRequest
		if err := readJSON(r, &req); err != nil {
			t.Errorf("bad request json: %v", err)
		}
		if req.Temperature != 0 {
			t.Errorf("temperature must be 0, got %v", req.Temperature)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"code 1001"}}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`))
	}))
	defer srv.Close()

	resp, err := CallOpenAI(context.Background(), Request{
		BaseURL: srv.URL, APIKey: "test-key", Model: "m1",
		Question: "q?", MaxTokens: 128, TimeoutS: 10,
	})
	if err != nil {
		t.Fatalf("CallOpenAI: %v", err)
	}
	if resp.Content != "code 1001" || resp.PromptTokens != 10 || resp.CompletionTokens != 2 {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestCallOpenAIRetryOn429(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(429)
			w.Write([]byte(`{"error":{"message":"rate limited"}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	resp, err := CallOpenAI(context.Background(), Request{
		BaseURL: srv.URL, APIKey: "k", Model: "m", Question: "q", MaxTokens: 16, TimeoutS: 10,
	})
	if err != nil {
		t.Fatalf("retry should succeed: %v", err)
	}
	if resp.Content != "ok" || attempts != 2 {
		t.Errorf("attempts=%d content=%s", attempts, resp.Content)
	}
}

func TestCallOpenAIFailFastOn400(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(400)
		w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer srv.Close()

	_, err := CallOpenAI(context.Background(), Request{
		BaseURL: srv.URL, APIKey: "k", Model: "m", Question: "q", MaxTokens: 16, TimeoutS: 10,
	})
	if err == nil {
		t.Fatal("400 should be an error")
	}
	if attempts != 1 {
		t.Errorf("400 must fail fast, attempts=%d", attempts)
	}
	if attempts != 1 {
		_ = attempts
	}
}
