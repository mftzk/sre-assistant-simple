package slackbot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"

	"sre-assistant-simple/internal/agent"
	"sre-assistant-simple/internal/config"
)

const (
	queueSize       = 32
	turnTimeout     = 3 * time.Minute
	maxMessageBytes = 8000
	maxReplyRunes   = 12000
	slackChunkRunes = 3000
	slackTimeout    = 15 * time.Second
)

// Run connects to Slack and wires two goroutines:
//
//	receiveMentions: Slack events -> filter + dedup -> queue
//	processMentions: queue -> harness -> reply in thread
func Run(ctx context.Context, c config.Config, harness *agent.Harness) error {
	api := slack.New(c.BotToken, slack.OptionAppLevelToken(c.AppToken))
	authCtx, cancel := context.WithTimeout(ctx, slackTimeout)
	identity, err := api.AuthTestContext(authCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("Slack auth gagal")
	}

	client := socketmode.New(api)
	queue := make(chan *slackevents.AppMentionEvent, queueSize)
	go processMentions(ctx, api, harness, identity.UserID, queue)
	go receiveMentions(ctx, api, client, identity.UserID, queue)
	return client.RunContext(ctx)
}

// processMentions is the only worker. One worker keeps session updates
// ordered and avoids parallel restart races.
func processMentions(ctx context.Context, api *slack.Client, harness *agent.Harness, botID string, queue <-chan *slackevents.AppMentionEvent) {
	sessions := sessionStore{}
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-queue:
			handleMention(ctx, api, harness, sessions, botID, ev)
		}
	}
}

// handleMention runs one user message through the harness and replies.
func handleMention(ctx context.Context, api *slack.Client, harness *agent.Harness, sessions sessionStore, botID string, ev *slackevents.AppMentionEvent) {
	key := mentionKey(ev)
	text := strings.TrimSpace(strings.ReplaceAll(ev.Text, "<@"+botID+">", ""))
	if text == "" {
		reply(ctx, api, key, "Sebutkan container: cek log demo-app, atau restart demo-app.")
		return
	}
	if len(text) > maxMessageBytes {
		reply(ctx, api, key, "Pesan terlalu panjang; maksimum 8000 byte.")
		return
	}

	session := sessions.get(key, time.Now())
	runCtx, cancel := context.WithTimeout(ctx, turnTimeout)
	// ev.User is the real Slack user ID; the harness uses it for authorization.
	answer, turn, err := harness.Run(runCtx, ev.User, text, session.history())
	cancel()
	if err != nil {
		answer = "Permintaan berhenti: " + err.Error() + ". Jika restart sudah dimulai, cek log sebelum mencoba ulang."
		// Failed turns aren't replayed. Preserve a factual notice so follow-ups don't imply no action happened.
		turn = []agent.Message{{Role: "user", Content: text}, {Role: "assistant", Content: answer}}
	}
	session.add(turn, time.Now())
	reply(ctx, api, key, answer)
}

// receiveMentions acks every Socket Mode event and queues human @mentions.
func receiveMentions(ctx context.Context, api *slack.Client, client *socketmode.Client, botID string, queue chan<- *slackevents.AppMentionEvent) {
	seen := deduplicator{}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-client.Events:
			if !ok {
				return
			}
			if event.Request != nil {
				client.Ack(*event.Request)
			}
			if event.Type == socketmode.EventTypeConnected {
				log.Print("slack_connected")
				continue
			}
			ev := appMention(event)
			if !humanMention(ev, botID) {
				continue
			}
			// Slack may deliver the same event more than once.
			if !seen.accept(ev.Channel+":"+ev.TimeStamp, time.Now()) {
				continue
			}
			select {
			case queue <- ev:
			default:
				busyCtx, cancel := context.WithTimeout(ctx, slackTimeout)
				reply(busyCtx, api, mentionKey(ev), "Antrean demo penuh; kirim mention baru sebentar lagi.")
				cancel()
			}
		}
	}
}

// appMention returns the @mention inside a Socket Mode event, or nil.
func appMention(event socketmode.Event) *slackevents.AppMentionEvent {
	if event.Type != socketmode.EventTypeEventsAPI {
		return nil
	}
	payload, ok := event.Data.(slackevents.EventsAPIEvent)
	if !ok {
		return nil
	}
	ev, _ := payload.InnerEvent.Data.(*slackevents.AppMentionEvent)
	return ev
}

// mentionKey picks the session: a thread reply continues the thread, a new
// mention starts one.
func mentionKey(ev *slackevents.AppMentionEvent) sessionKey {
	thread := ev.ThreadTimeStamp
	if thread == "" {
		thread = ev.TimeStamp
	}
	return sessionKey{ev.Channel, thread, ev.User}
}

// humanMention ignores bots (including ourselves) and edited messages.
func humanMention(ev *slackevents.AppMentionEvent, selfID string) bool {
	return ev != nil && ev.BotID == "" && ev.User != "" && ev.User != selfID && ev.Edited == nil && ev.Channel != "" && ev.TimeStamp != ""
}

// reply posts text in the thread, split into Slack-sized chunks.
func reply(ctx context.Context, api *slack.Client, key sessionKey, text string) {
	// Escape control syntax so log/model output cannot ping users or channels.
	text = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
	runes := []rune(text)
	if len(runes) > maxReplyRunes {
		runes = append(runes[:maxReplyRunes], []rune("\n[jawaban dipotong]")...)
	}
	for len(runes) > 0 {
		n := min(len(runes), slackChunkRunes)
		postCtx, cancel := context.WithTimeout(ctx, slackTimeout)
		_, _, err := api.PostMessageContext(postCtx, key.Channel, slack.MsgOptionText(string(runes[:n]), false),
			slack.MsgOptionTS(key.Thread), slack.MsgOptionDisableLinkUnfurl(), slack.MsgOptionDisableMediaUnfurl())
		cancel()
		if err != nil {
			log.Print("slack_reply_failed (tindakan Docker tidak diulang)")
			return
		}
		runes = runes[n:]
	}
}
