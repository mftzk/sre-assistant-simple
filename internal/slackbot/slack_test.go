package slackbot

import (
	"strings"
	"testing"
	"time"

	"github.com/slack-go/slack/slackevents"

	"sre-assistant-simple/internal/agent"
)

func TestSessionIsolationTTLAndTurnBound(t *testing.T) {
	now := time.Now()
	store := sessionStore{}
	k := sessionKey{"C", "thread", "U1"}
	c := store.get(k, now)
	for i := 0; i < 12; i++ {
		c.add([]agent.Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "world"}}, now)
	}
	if len(c.Turns) != 10 || len(c.history()) != 20 {
		t.Fatal("turn pruning failed")
	}
	for _, other := range []sessionKey{{"C", "thread", "U2"}, {"C", "different", "U1"}, {"D", "thread", "U1"}} {
		if len(store.get(other, now).history()) != 0 {
			t.Fatal("context leaked")
		}
	}
	if len(store.get(k, now.Add(31*time.Minute)).history()) != 0 {
		t.Fatal("expired context retained")
	}
}
func TestDedupAndHumanMentions(t *testing.T) {
	now := time.Now()
	d := deduplicator{}
	if !d.accept("C:ts", now) || d.accept("C:ts", now.Add(time.Minute)) || !d.accept("C:ts", now.Add(11*time.Minute)) {
		t.Fatal("dedup TTL failed")
	}
	ev := &slackevents.AppMentionEvent{User: "U1", Channel: "C", TimeStamp: "root"}
	if !humanMention(ev, "BOT") || mentionKey(ev).Thread != "root" {
		t.Fatal("human root event dropped")
	}
	ev.ThreadTimeStamp = "thread"
	if mentionKey(ev).Thread != "thread" {
		t.Fatal("thread lost")
	}
	ev.BotID = "B1"
	if humanMention(ev, "BOT") {
		t.Fatal("bot event accepted")
	}
	ev.BotID = ""
	ev.User = "BOT"
	if humanMention(ev, "BOT") {
		t.Fatal("self event accepted")
	}
}

func TestChunksAndToolTitles(t *testing.T) {
	parts := chunks(strings.Repeat("a", 7000))
	if len(parts) != 3 || len([]rune(parts[0])) != 3000 {
		t.Fatal(len(parts))
	}
	if escape("<@U1> & <!here>") != "&lt;@U1&gt; &amp; &lt;!here&gt;" {
		t.Fatal("mentions not escaped")
	}
	if toolTitle(agent.ToolEvent{Tool: "docker_logs", Container: "demo-api"}) != "Baca log demo-api" ||
		toolTitle(agent.ToolEvent{Tool: "docker_inspect"}) != "Inspect container" {
		t.Fatal("unexpected titles")
	}
}
