package taxeval

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CallCodex runs one question through the local `codex` CLI (ChatGPT
// subscription) in non-interactive exec mode and captures the final agent
// message from stdout. The question is passed via ARGV (never stdin) to avoid
// quoting bugs. model pins the CLI model (e.g. gpt-6-luna); empty means
// codex default.
func CallCodex(ctx context.Context, question, model string, timeoutS int) (Response, error) {
	cctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutS)*time.Second)
	defer cancel()

	args := []string{"exec", "--skip-git-repo-check"}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, question)
	cmd := exec.CommandContext(cctx, "codex", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if cctx.Err() == context.DeadlineExceeded {
			return Response{}, fmt.Errorf("codex CLI timeout after %ds", timeoutS)
		}
		return Response{}, fmt.Errorf("codex CLI: %w; stderr=%s", err, truncate(stderr.String(), 300))
	}
	if strings.TrimSpace(string(out)) == "" {
		return Response{}, fmt.Errorf("codex CLI returned empty output")
	}
	return Response{Content: strings.TrimSpace(string(out))}, nil
}
