package taxeval

import (
	"encoding/json"
	"errors"
	"strings"
)

var errNoChoices = errors.New("no choices in response")

// chatResponse reasoning handling: some models (qwen3.5 thinking mode, gpt-oss)
// return reasoning in a separate field and leave content empty, or put
// <think>...</think> blocks inside content. Extract the final answer.

type chatResponseWithReasoning struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			Reasoning string `json:"reasoning"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ExtractAnswer normalizes a raw chat-completion body into the final answer
// text: strips <think> blocks, falls back to the reasoning field when content
// is empty (taking text after the last blank line — the "conclusion").
func ExtractAnswer(raw []byte) (string, string, error) {
	var cr chatResponseWithReasoning
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", "", err
	}
	if len(cr.Choices) == 0 {
		return "", "", errNoChoices
	}
	content := cr.Choices[0].Message.Content
	reasoning := cr.Choices[0].Message.Reasoning

	// strip <think>...</think> if present in content
	if idx := strings.Index(content, "</think>"); idx >= 0 {
		content = content[idx+len("</think>"):]
	}
	content = strings.TrimSpace(content)
	if content != "" {
		return content, reasoning, nil
	}
	// thinking model that put everything in reasoning: take the tail after
	// the last double newline (the summary/conclusion sentence block)
	if reasoning != "" {
		if idx := strings.Index(reasoning, "</think>"); idx >= 0 {
			reasoning = reasoning[idx+len("</think>"):]
		}
		r := strings.TrimSpace(reasoning)
		if parts := strings.Split(r, "\n\n"); len(parts) > 1 {
			tail := strings.TrimSpace(parts[len(parts)-1])
			if tail != "" {
				return tail, r, nil
			}
		}
		return r, r, nil
	}
	return "", "", nil
}
