package authz

import "testing"

func TestRestartIntent(t *testing.T) {
	for text, want := range map[string]bool{"restart demo-app": true, "restart container tadi": true, "cek log demo-app": false, "jangan restart demo-app": false, "do not restart demo-app": false, "restartkan demo-app": true} {
		if RestartRequested(text) != want {
			t.Errorf("%q", text)
		}
	}
}

func TestAuthorizer(t *testing.T) {
	users := map[string]bool{"USRE": true, "UDEV": false, "": true}
	a := New(users)
	delete(users, "USRE")
	for _, id := range []string{"UDEV", "", "unknown"} {
		if a.IsSRE(id) || a.CanRestart(id, true) {
			t.Fatalf("unauthorized ID accepted: %q", id)
		}
	}
	if !a.IsSRE("USRE") || !a.CanRestart("USRE", true) || a.CanRestart("USRE", false) {
		t.Fatal("SRE/request authorization changed")
	}
	var missing *Authorizer
	if missing.IsSRE("USRE") || missing.CanRestart("USRE", true) {
		t.Fatal("nil authorizer must deny")
	}
}
