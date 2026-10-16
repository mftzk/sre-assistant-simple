package slackbot

import (
	"context"
	"log"

	"github.com/slack-go/slack"

	"sre-assistant-simple/internal/agent"
)

// thinkingTask is the first task card, shown while the model reads the question.
const thinkingTask = "thinking"

// stream shows a turn with Slack's native streaming message
// (chat.startStream / appendStream / stopStream): every tool call becomes a
// task card that turns from "in progress" to done, then the answer streams in.
// Any Slack error marks the stream broken and the caller falls back to reply.
type stream struct {
	api     *slack.Client
	ctx     context.Context
	channel string
	ts      string
	broken  bool
}

func startStream(ctx context.Context, api *slack.Client, key sessionKey, teamID string) *stream {
	callCtx, cancel := context.WithTimeout(ctx, slackTimeout)
	defer cancel()
	_, ts, err := api.StartStreamContext(callCtx, key.Channel,
		slack.MsgOptionTS(key.Thread),
		slack.MsgOptionRecipientUserID(key.User),
		slack.MsgOptionRecipientTeamID(teamID),
		slack.MsgOptionTaskDisplayMode(slack.TaskDisplayModeTimeline),
		slack.MsgOptionChunks(task(thinkingTask, "Memahami pertanyaan", slack.TaskCardStatusInProgress)))
	if err != nil {
		log.Printf("slack_stream_unavailable: %v", err)
		return nil
	}
	return &stream{api: api, ctx: ctx, channel: key.Channel, ts: ts}
}

// progress is passed to the harness; it turns each ToolEvent into a task card.
func (s *stream) progress(ev agent.ToolEvent) {
	status := slack.TaskCardStatusInProgress
	if ev.Done {
		status = slack.TaskCardStatusComplete
		if !ev.OK {
			status = slack.TaskCardStatusError
		}
	}
	s.append(task(thinkingTask, "Memahami pertanyaan", slack.TaskCardStatusComplete),
		task(ev.ID, toolTitle(ev), status))
}

// finish streams the answer and closes the message. It reports false when the
// stream broke, so the caller can post the answer as a normal reply.
func (s *stream) finish(answer string) bool {
	s.append(task(thinkingTask, "Memahami pertanyaan", slack.TaskCardStatusComplete))
	for _, part := range chunks(answer) {
		s.append(slack.NewMarkdownTextChunk(part))
	}
	// Close the message even after a failed append, so it does not stay "typing".
	callCtx, cancel := context.WithTimeout(s.ctx, slackTimeout)
	defer cancel()
	if _, _, err := s.api.StopStreamContext(callCtx, s.channel, s.ts); err != nil {
		log.Printf("slack_stream_stop_failed: %v", err)
	}
	return !s.broken
}

func (s *stream) append(parts ...slack.StreamChunk) {
	if s == nil || s.broken {
		return
	}
	callCtx, cancel := context.WithTimeout(s.ctx, slackTimeout)
	defer cancel()
	if _, _, err := s.api.AppendStreamContext(callCtx, s.channel, s.ts, slack.MsgOptionChunks(parts...)); err != nil {
		log.Printf("slack_stream_append_failed: %v", err)
		s.broken = true
	}
}

func task(id, title string, status slack.TaskCardStatus) slack.TaskUpdateChunk {
	chunk := slack.NewTaskUpdateChunk(id, title)
	chunk.Status = status
	return chunk
}

// toolTitle is the human label of a task card, e.g. "Baca log demo-api".
func toolTitle(ev agent.ToolEvent) string {
	target := ev.Container
	if target == "" {
		target = "container"
	}
	switch ev.Tool {
	case "docker_list":
		return "Lihat daftar container"
	case "docker_inspect":
		return "Inspect " + target
	case "docker_logs":
		return "Baca log " + target
	case "docker_restart":
		return "Restart " + target
	case "ssh_command":
		return "Jalankan perintah di server"
	default:
		return "Tool tidak dikenal"
	}
}
