// claude_proto.go — Anthropic /v1/messages protocol backend (z.ai GLM coding
// plan route). Mirrors CallOpenAI's retry/backoff semantics.
package taxeval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type anthropicRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	Temperature float64         `json:"temperature"`
	Messages    []anthroMessage `json:"messages"`
	Thinking    *anthroThink    `json:"thinking,omitempty"`
}

type anthroMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthroThink struct {
	Type string `json:"type"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// CallAnthropic sends one /v1/messages request (temp 0, thinking disabled,
// 3 attempts with backoff on 429/5xx/network).
func CallAnthropic(ctx context.Context, r Request) (Response, error) {
	body := anthropicRequest{
		Model:       r.Model,
		MaxTokens:   r.MaxTokens,
		Temperature: 0,
		Messages: []anthroMessage{
			{Role: "user", Content: r.PromptPreamble + r.Question},
		},
		Thinking: &anthroThink{Type: "disabled"},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	url := r.BaseURL + "/messages"

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
		req.Header.Set("x-api-key", r.APIKey)
		req.Header.Set("anthropic-version", "2023-06-01")

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
			return Response{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
		}
		var ar anthropicResponse
		if err := json.Unmarshal(raw, &ar); err != nil {
			return Response{}, fmt.Errorf("decode anthropic response: %w; body=%s", err, truncate(string(raw), 300))
		}
		if ar.Error != nil {
			return Response{}, fmt.Errorf("API error: %s", ar.Error.Message)
		}
		var text string
		for _, b := range ar.Content {
			if b.Type == "text" {
				text += b.Text
			}
		}
		if strings.TrimSpace(text) == "" {
			return Response{}, fmt.Errorf("anthropic response had no text blocks (stop=%s)", ar.StopReason)
		}
		return Response{
			Content:          text,
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
		}, nil
	}
	return Response{}, lastErr
}
