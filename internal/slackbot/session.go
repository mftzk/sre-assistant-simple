package slackbot

import (
	"time"

	"sre-assistant-simple/internal/agent"
)

type sessionKey struct{ Channel, Thread, User string }
type conversation struct {
	Turns   [][]agent.Message
	Updated time.Time
}
type sessionStore map[sessionKey]*conversation

func (s sessionStore) get(key sessionKey, now time.Time) *conversation {
	for k, v := range s {
		if now.Sub(v.Updated) >= 30*time.Minute {
			delete(s, k)
		}
	}
	if v := s[key]; v != nil {
		return v
	}
	if len(s) >= 100 {
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
func (c *conversation) history() []agent.Message {
	var messages []agent.Message
	for _, turn := range c.Turns {
		messages = append(messages, turn...)
	}
	return messages
}
func (c *conversation) add(turn []agent.Message, now time.Time) {
	c.Turns = append(c.Turns, turn)
	if len(c.Turns) > 10 {
		c.Turns = append([][]agent.Message(nil), c.Turns[len(c.Turns)-10:]...)
	}
	c.Updated = now
}

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
	d[key] = now.Add(10 * time.Minute)
	return true
}
