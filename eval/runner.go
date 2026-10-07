package taxeval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Request is one HTTP chat-completion request description.
type Request struct {
	BaseURL        string
	APIKey         string
	Model          string
	Question       string
	MaxTokens      int
	TimeoutS       int
	PromptPreamble string
}

// Response is the normalized model reply.
type Response struct {
	Content          string
	PromptTokens     int
	CompletionTokens int
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	TopP        float64       `json:"top_p"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// CallOpenAI sends one chat completion with temp 0 and retries on
// 429/5xx/network errors (3 attempts, backoff 2s/8s).
func CallOpenAI(ctx context.Context, r Request) (Response, error) {
	body := chatRequest{
		Model: r.Model,
		Messages: []chatMessage{
			{Role: "user", Content: r.PromptPreamble + r.Question},
		},
		Temperature: 0,
		MaxTokens:   r.MaxTokens,
		TopP:        1,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	url := r.BaseURL + "/chat/completions"

	var lastErr error
	backoffs := []time.Duration{2 * time.Second, 8 * time.Second}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return Response{}, ctx.Err()
			case <-time.After(backoffs[attempt-1]):
			}
		}
		cctx, cancel := context.WithTimeout(ctx, time.Duration(r.TimeoutS)*time.Second)
		req, err := http.NewRequestWithContext(cctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			cancel()
			return Response{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+r.APIKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("network: %w", err)
			continue
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		cancel()
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			continue
		}
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
			continue
		}
		if resp.StatusCode != 200 {
			// non-retryable client error
			return Response{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
		}
		var cr chatResponse
		if err := json.Unmarshal(raw, &cr); err != nil {
			return Response{}, fmt.Errorf("decode response: %w; body=%s", err, truncate(string(raw), 300))
		}
		if cr.Error != nil {
			return Response{}, fmt.Errorf("API error: %s", cr.Error.Message)
		}
		if len(cr.Choices) == 0 {
			return Response{}, fmt.Errorf("no choices in response; body=%s", truncate(string(raw), 300))
		}
		content, _, err := ExtractAnswer(raw)
		if err != nil {
			return Response{}, err
		}
		return Response{
			Content:          content,
			PromptTokens:     cr.Usage.PromptTokens,
			CompletionTokens: cr.Usage.CompletionTokens,
		}, nil
	}
	return Response{}, lastErr
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
