package slackbot

import (
	"time"

	"sre-assistant-simple/internal/agent"
)

// Memory lives only in RAM: it is lost when the bot restarts and is never
// rebuilt from Slack history.
const (
	sessionTTL    = 30 * time.Minute
	maxSessions   = 100
	maxTurns      = 10
	dedupDuration = 10 * time.Minute
)

// sessionKey includes User, so an SRE never inherits another user's context
// in the same thread.
type sessionKey struct{ Channel, Thread, User string }
type conversation struct {
	Turns   [][]agent.Message
	Updated time.Time
}
type sessionStore map[sessionKey]*conversation

// get returns the session for key, after dropping expired ones and, when
// full, the oldest one.
func (s sessionStore) get(key sessionKey, now time.Time) *conversation {
	for k, v := range s {
		if now.Sub(v.Updated) >= sessionTTL {
			delete(s, k)
		}
	}
	if v := s[key]; v != nil {
		return v
	}
	if len(s) >= maxSessions {
		var oldest sessionKey
		t := now
		for k, v := range s {
			if v.Updated.Before(t) {
				oldest, t = k, v.Updated
			}
		}
		delete(s, oldest)
	}
	v := &conversation{Updated: now}
	s[key] = v
	return v
}

// history flattens the stored turns into the message list sent to the LLM.
func (c *conversation) history() []agent.Message {
	var messages []agent.Message
	for _, turn := range c.Turns {
		messages = append(messages, turn...)
	}
	return messages
}
func (c *conversation) add(turn []agent.Message, now time.Time) {
	c.Turns = append(c.Turns, turn)
	if len(c.Turns) > maxTurns {
		c.Turns = append([][]agent.Message(nil), c.Turns[len(c.Turns)-maxTurns:]...)
	}
	c.Updated = now
}

// deduplicator remembers event keys for dedupDuration.
type deduplicator map[string]time.Time

func (d deduplicator) accept(key string, now time.Time) bool {
	for k, expiry := range d {
		if !now.Before(expiry) {
			delete(d, k)
		}
	}
	if _, exists := d[key]; exists {
		return false
	}
	d[key] = now.Add(dedupDuration)
	return true
}
