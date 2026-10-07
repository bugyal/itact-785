package taxeval

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CallClaude runs one question through the local `claude` CLI (Max
// subscription) with JSON output and extracts .result. The question is passed
// via ARGV (never stdin) to avoid quoting bugs. model pins the CLI model
// (e.g. sonnet-4-5); empty means profile default.
func CallClaude(ctx context.Context, question, model string, timeoutS int) (Response, error) {
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutS)*time.Second)
	defer cancel()

	args := []string{"-p", question, "--output-format", "json"}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.CommandContext(cctx, "claude", args...)
	out, err := cmd.Output()
	if err != nil {
		if cctx.Err() == context.DeadlineExceeded {
			return Response{}, fmt.Errorf("claude CLI timeout after %ds", timeoutS)
		}
		return Response{}, fmt.Errorf("claude CLI: %w", err)
	}
	var parsed struct {
		Type   string `json:"type"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return Response{}, fmt.Errorf("decode claude output: %w; raw=%s", err, truncate(string(out), 300))
	}
	if strings.TrimSpace(parsed.Result) == "" {
		return Response{}, fmt.Errorf("claude CLI returned empty result; raw=%s", truncate(string(out), 300))
	}
	return Response{Content: parsed.Result}, nil
}
