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

func mentionKey(ev *slackevents.AppMentionEvent) sessionKey {
	thread := ev.ThreadTimeStamp
	if thread == "" {
		thread = ev.TimeStamp
	}
	return sessionKey{ev.Channel, thread, ev.User}
}
func humanMention(ev *slackevents.AppMentionEvent, selfID string) bool {
	return ev != nil && ev.BotID == "" && ev.User != "" && ev.User != selfID && ev.Edited == nil && ev.Channel != "" && ev.TimeStamp != ""
}

func reply(ctx context.Context, api *slack.Client, key sessionKey, text string) {
	// Escape control syntax so log/model output cannot ping users or channels.
	text = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
	runes := []rune(text)
	if len(runes) > 12000 {
		runes = append(runes[:12000], []rune("\n[jawaban dipotong]")...)
	}
	for len(runes) > 0 {
		n := min(len(runes), 3000)
		postCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
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

func Run(ctx context.Context, c config.Config, harness *agent.Harness) error {
	api := slack.New(c.BotToken, slack.OptionAppLevelToken(c.AppToken))
	authCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	identity, err := api.AuthTestContext(authCtx)
	cancel()
	if err != nil {
		return fmt.Errorf("Slack auth gagal")
	}
	client := socketmode.New(api)
	queue := make(chan *slackevents.AppMentionEvent, 32)
	// One worker keeps session updates ordered and avoids parallel restart races.
	go func() {
		sessions := sessionStore{}
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-queue:
				key := mentionKey(ev)
				text := strings.TrimSpace(strings.ReplaceAll(ev.Text, "<@"+identity.UserID+">", ""))
				if text == "" {
					reply(ctx, api, key, "Sebutkan container: cek log demo-app, atau restart demo-app.")
					continue
				}
				if len(text) > 8000 {
					reply(ctx, api, key, "Pesan terlalu panjang; maksimum 8000 byte.")
					continue
				}
				session := sessions.get(key, time.Now())
				runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
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
		}
	}()
	go func() {
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
				if event.Type != socketmode.EventTypeEventsAPI {
					continue
				}
				payload, ok := event.Data.(slackevents.EventsAPIEvent)
				if !ok {
					continue
				}
				ev, ok := payload.InnerEvent.Data.(*slackevents.AppMentionEvent)
				if !ok || !humanMention(ev, identity.UserID) {
					continue
				}
				if !seen.accept(ev.Channel+":"+ev.TimeStamp, time.Now()) {
					continue
				}
				select {
				case queue <- ev:
				default:
					busyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
					reply(busyCtx, api, mentionKey(ev), "Antrean demo penuh; kirim mention baru sebentar lagi.")
					cancel()
				}
			}
		}
	}()
	return client.RunContext(ctx)
}
