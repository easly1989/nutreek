package app

import (
	"net/url"
	"testing"
)

// A right sign-in does not give back the tries a person has used: someone could otherwise
// sign in to their own account between guesses at another one (#338).
func TestLoginLimiterIsNotResetByASignIn(t *testing.T) {
	a := newTestApp(t)
	mustCreateTestUser(t, a, "victim@example.test", "victim-password")
	mustCreateTestUser(t, a, "mine@example.test", "my-password")
	post := func(email, pw string) int {
		code, _, _ := newBrowser(t, a).submit("/login", "/login", url.Values{"email": {email}, "password": {pw}})
		return code
	}
	for i := 0; i < 4; i++ {
		if code := post("victim@example.test", "guess"); code != 401 {
			t.Fatalf("guess %d: %d", i+1, code)
		}
	}
	if code := post("mine@example.test", "my-password"); code != 303 { // the fifth try: a right one
		t.Fatalf("own sign-in: %d", code)
	}
	if code := post("victim@example.test", "another guess"); code != 429 {
		t.Fatalf("the sixth try in a minute should be refused, got %d", code)
	}
}
