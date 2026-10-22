package commit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Planner turns a prompt into a commit plan.
type Planner interface {
	Plan(ctx context.Context, prompt string) (Reply, error)
}

// Claude plans with the claude CLI in print mode, with the reply held to
// replySchema by structured output.
type Claude struct {
	// Model is the model name or alias passed to --model.
	Model string
}

// envelope is the part of `claude -p --output-format json` this reads.
type envelope struct {
	IsError          bool            `json:"is_error"`
	Result           string          `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

// Plan runs claude once. It runs outside the repo with no tools, settings or
// MCP servers: the prompt carries everything the plan needs.
func (c Claude) Plan(ctx context.Context, prompt string) (Reply, error) {
	cmd := exec.CommandContext(ctx, "claude", "-p",
		"--model", c.Model,
		"--output-format", "json",
		"--json-schema", replySchema,
		"--tools", "",
		"--setting-sources", "",
		"--strict-mcp-config",
		"--disable-slash-commands",
		"--no-session-persistence",
	)
	cmd.Dir = os.TempDir()
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return Reply{}, errors.New("the claude CLI is not on PATH; install Claude Code to use commit")
		}
		return Reply{}, fmt.Errorf("claude: %v: %s", err, strings.TrimSpace(stderr.String()+" "+stdout.String()))
	}

	var env envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return Reply{}, fmt.Errorf("read the claude reply: %w", err)
	}
	if env.IsError || len(env.StructuredOutput) == 0 || string(env.StructuredOutput) == "null" {
		return Reply{}, fmt.Errorf("claude returned no plan: %s", strings.TrimSpace(env.Result))
	}
	var reply Reply
	if err := json.Unmarshal(env.StructuredOutput, &reply); err != nil {
		return Reply{}, fmt.Errorf("read the claude plan: %w", err)
	}
	return reply, nil
}
