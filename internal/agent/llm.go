package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}
type LLM struct {
	BaseURL, Key, Model string
	Client              *http.Client
}

func (l *LLM) Chat(ctx context.Context, messages []Message, tools []map[string]any) (Message, error) {
	payload, err := json.Marshal(map[string]any{"model": l.Model, "messages": messages, "tools": tools})
	if err != nil {
		return Message{}, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", l.BaseURL+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Authorization", "Bearer "+l.Key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.Client.Do(req)
	if err != nil {
		return Message{}, fmt.Errorf("LLM request gagal")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Message{}, fmt.Errorf("LLM HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if err != nil || len(body) > 1024*1024 {
		return Message{}, fmt.Errorf("LLM response gagal/terlalu besar")
	}
	var data struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &data); err != nil || len(data.Choices) == 0 {
		return Message{}, fmt.Errorf("LLM response tidak valid")
	}
	m := data.Choices[0].Message
	m.Role = "assistant"
	return m, nil
}
