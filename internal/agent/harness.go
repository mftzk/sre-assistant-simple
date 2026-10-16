package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"sre-assistant-simple/internal/authz"
)

// Guardrails of the loop. A harness is mostly these limits plus the loop below.
const (
	maxToolRounds    = 6 // tool rounds per user message
	maxCallsPerRound = 4 // tool calls the model may request at once
)

// Tools is the narrow execution boundary; the loop has no SSH credentials.
type Tools interface {
	Execute(ctx context.Context, caller string, restartAllowed bool, call ToolCall) string
	AllowedContainers() []string
	CanRestart(caller, text string) bool
}

// Harness = model + tools + the loop that connects them.
type Harness struct {
	LLM    *LLM
	Docker Tools
}

// Run handles one user message:
//
//  1. decide which tools this user may see
//  2. send system prompt + history + new message to the LLM
//  3. if the LLM answers with text, we are done
//  4. if the LLM asks for tools, run them and send the results back (go to 2)
//
// It returns the final answer and the messages of this turn, so the caller
// can keep them as history for follow-up questions.
func (h *Harness) Run(ctx context.Context, caller, text string, history []Message) (string, []Message, error) {
	// Step 1: permission is decided by code from the real Slack user ID and the
	// current message, before the model sees anything.
	canRestart := h.Docker.CanRestart(caller, text)
	tools := toolSchemas(canRestart)

	// Step 2: build the conversation the model will see.
	messages := []Message{{Role: "system", Content: systemPrompt(h.Docker.AllowedContainers())}}
	messages = append(messages, history...)
	turn := []Message{{Role: "user", Content: text}}
	messages = append(messages, turn...)

	restarted := map[string]bool{} // at most one restart attempt per container per message

	// One extra round lets the model give its final answer after the last tool round.
	for round := 0; round <= maxToolRounds; round++ {
		log.Printf("llm_round=%d user=%s", round+1, caller)
		answer, err := h.LLM.Chat(ctx, messages, tools)
		if err != nil {
			return "", nil, err
		}

		// Step 3: no tool calls means this is the final answer.
		if len(answer.ToolCalls) == 0 {
			if strings.TrimSpace(answer.Content) == "" {
				return "", nil, fmt.Errorf("LLM mengembalikan jawaban kosong")
			}
			turn = append(turn, answer)
			return answer.Content, turn, nil
		}

		// Step 4: the model wants tools. Check the request, then run each call.
		if round == maxToolRounds {
			return "", nil, fmt.Errorf("batas enam putaran tool tercapai; tindakan sebelumnya mungkin sudah dijalankan")
		}
		if err := checkToolCalls(answer.ToolCalls); err != nil {
			return "", nil, err
		}
		messages = append(messages, answer)
		turn = append(turn, answer)

		for _, call := range answer.ToolCalls {
			result := h.runTool(ctx, caller, canRestart, restarted, call)
			// The result goes back as a "tool" message linked by tool_call_id.
			message := Message{Role: "tool", ToolCallID: call.ID, Content: result}
			messages = append(messages, message)
			turn = append(turn, message)
		}
	}
	return "", nil, fmt.Errorf("batas harness tercapai")
}

// checkToolCalls rejects malformed requests before anything runs.
func checkToolCalls(calls []ToolCall) error {
	if len(calls) > maxCallsPerRound {
		return fmt.Errorf("terlalu banyak tool call dalam satu putaran")
	}
	seen := map[string]bool{}
	for _, call := range calls {
		if call.ID == "" || seen[call.ID] || call.Type != "function" {
			return fmt.Errorf("tool call LLM tidak valid")
		}
		seen[call.ID] = true
	}
	return nil
}

// runTool executes one tool call and returns its JSON result for the model.
func (h *Harness) runTool(ctx context.Context, caller string, canRestart bool, restarted map[string]bool, call ToolCall) string {
	start := time.Now()
	container := containerArg(call)

	permitted := canRestart
	if call.Function.Name == "docker_restart" {
		permitted = permitted && !restarted[container]
		restarted[container] = true
	}
	result := h.Docker.Execute(ctx, caller, permitted, call)

	traceTool(call, container, result, time.Since(start), caller)
	return result
}

// containerArg reads the container name only for the restart-once rule and
// the trace. The tool itself parses and validates its arguments strictly.
func containerArg(call ToolCall) string {
	var args struct {
		Container string `json:"container"`
	}
	_ = json.Unmarshal([]byte(call.Function.Arguments), &args)
	return args.Container
}

// traceTool prints one line per tool call. It never prints log contents,
// prompts or model-supplied values that failed validation.
func traceTool(call ToolCall, container, result string, duration time.Duration, caller string) {
	var status struct {
		OK bool `json:"ok"`
	}
	_ = json.Unmarshal([]byte(result), &status)

	switch {
	case container == "":
		container = "-"
	case !authz.ValidContainer(container):
		container = "invalid"
	}
	tool := call.Function.Name
	switch tool {
	case "docker_list", "docker_inspect", "docker_logs", "docker_restart":
	default:
		tool = "unknown"
	}
	log.Printf("tool=%s container=%s ok=%t duration=%s user=%s", tool, container, status.OK, duration.Round(time.Millisecond), caller)
}
