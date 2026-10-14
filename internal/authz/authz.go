package authz

import "regexp"

var containerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
var restartWord = regexp.MustCompile(`(?i)\b(restart|reboot|restartkan)\b`)
var noRestart = regexp.MustCompile(`(?i)\b(jangan|tanpa|bukan|never|without)\b|don't|do not`)

func ValidContainer(name string) bool { return containerName.MatchString(name) }

// RestartRequested only reads the current human message, never tool output.
func RestartRequested(text string) bool {
	return restartWord.MatchString(text) && !noRestart.MatchString(text)
}

// Authorizer uses the real Slack event user ID, not model-supplied identity.
type Authorizer struct{ sreUsers map[string]bool }

func New(users map[string]bool) *Authorizer {
	copy := make(map[string]bool, len(users))
	for id, allowed := range users {
		if id != "" {
			copy[id] = allowed
		}
	}
	return &Authorizer{sreUsers: copy}
}

func (a *Authorizer) IsSRE(userID string) bool {
	return a != nil && userID != "" && a.sreUsers[userID]
}

func (a *Authorizer) CanRestart(userID string, explicitlyRequested bool) bool {
	return a.IsSRE(userID) && explicitlyRequested
}
