package swarm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Agent is one member of the swarm.
type Agent struct {
	Label string
	Name  string
	Model string
}

// Agents mirrors gopilot's swarm: three free models on the opencode gateway.
var Agents = []Agent{
	{Label: "DeepSeek", Name: "deepseek", Model: "opencode/deepseek-v4.1-flash-free"},
	{Label: "GLM", Name: "glm", Model: "opencode/minimax-m3.1-flash-free"},
	{Label: "Bunny", Name: "bunny", Model: "opencode/nemotron-3.5-super-vl-free"},
}

const guard = "Отвечай по-русски, кратко и по существу. Не используй инструменты, не читай файлы, не запускай команды — только текст ответа."

func binary() string {
	if p, err := exec.LookPath("opencode"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".opencode", "bin", "opencode")
}

// Run executes one non-interactive opencode run and returns its stdout.
func Run(ctx context.Context, agent, model, prompt string) (string, error) {
	dir, err := os.MkdirTemp("", "nasdash-swarm-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	args := []string{"run", guard + "\n\n" + prompt, "--agent", agent}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.CommandContext(ctx, binary(), args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.WaitDelay = 2_000_000_000 // 2s

	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(out.String())
		if detail == "" {
			detail = strings.TrimSpace(errb.String())
		}
		if len(detail) > 400 {
			detail = detail[len(detail)-400:]
		}
		return "", fmt.Errorf("%v: %s", err, detail)
	}
	text := strings.TrimSpace(out.String())
	if text == "" {
		text = strings.TrimSpace(errb.String())
	}
	if text == "" {
		return "", fmt.Errorf("пустой ответ")
	}
	return text, nil
}

// Architect asks the architect agent to synthesize the swarm answers.
func Architect(ctx context.Context, question string, answers []string) (string, error) {
	var b strings.Builder
	b.WriteString("Вопрос пользователя:\n")
	b.WriteString(question)
	b.WriteString("\n\nОтветы трёх агентов:\n")
	for i, a := range answers {
		fmt.Fprintf(&b, "\n[%d] %s\n", i+1, a)
	}
	b.WriteString("\nСинтезируй лучший ответ: убери ошибки, соедини полезное, дай финальный ответ по делу.")

	out, err := Run(ctx, "architect", "", b.String())
	if err != nil {
		// Fall back to the default agent if the architect profile is missing.
		return Run(ctx, "build", "", b.String())
	}
	return out, nil
}
