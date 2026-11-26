package app

import (
	"context"
	"net/url"
	"testing"
)

// Changing the password and deleting the account check the current password, so they are
// limited for the account: a stolen session must not guess the password freely (#345).
func TestSettingsPasswordChecksAreLimited(t *testing.T) {
	cases := []struct {
		name, path string
		form       url.Values
	}{
		{"change password", "/settings/password", url.Values{"current_password": {"wrong-guess"}, "new_password": {"a-new-password"}, "confirm_password": {"a-new-password"}}},
		{"delete account", "/settings/delete-account", url.Values{"password": {"wrong-guess"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t)
			user := mustCreateTestUser(t, a, "carlo@example.test", "right-password")
			b := newBrowser(t, a)
			if code, _ := logIn(b, "carlo@example.test", "right-password"); code != 303 {
				t.Fatalf("login: %d", code)
			}
			for i := 0; i < 5; i++ { // five tries for the account: signing in does not use them
				if code, _, _ := b.submit("/settings/account", tc.path, tc.form); code != 422 {
					t.Fatalf("guess %d: %d, want 422", i+1, code)
				}
			}
			if code, _, _ := b.submit("/settings/account", tc.path, tc.form); code != 429 {
				t.Fatalf("the sixth try in a minute should be refused, got %d", code)
			}
			// And a right password is refused too while the limit holds, so nothing changes.
			right := url.Values{}
			for k := range tc.form {
				right.Set(k, "right-password")
			}
			if tc.name == "change password" {
				right.Set("new_password", "a-new-password")
				right.Set("confirm_password", "a-new-password")
			}
			if code, _, _ := b.submit("/settings/account", tc.path, right); code != 429 {
				t.Fatalf("a right password within the limit: %d, want 429", code)
			}
			if _, err := a.Users.GetByID(context.Background(), user.ID); err != nil {
				t.Fatal("the account must still be there")
			}
		})
	}
}

// The limit is the account's, not the address's: a neighbour on the same address cannot use up
// someone else's tries, and a thief on another address does not get a fresh five.
func TestPasswordTriesAreTheAccountsNotTheAddresses(t *testing.T) {
	a := newTestApp(t)
	mustCreateTestUser(t, a, "carlo@example.test", "right-password")
	mustCreateTestUser(t, a, "eva@example.test", "eva-password")
	eva := newBrowser(t, a)
	logIn(eva, "eva@example.test", "eva-password")
	for i := 0; i < 9; i++ { // many wrong tries from the same address by someone else
		eva.submit("/settings/account", "/settings/password", url.Values{"current_password": {"x"}, "new_password": {"a-new-password"}, "confirm_password": {"a-new-password"}})
	}
	b := newBrowser(t, a)
	logIn(b, "carlo@example.test", "right-password")
	if code, _, _ := b.submit("/settings/account", "/settings/password", url.Values{"current_password": {"right-password"}, "new_password": {"a-new-password"}, "confirm_password": {"a-new-password"}}); code != 303 {
		t.Fatalf("a right password of another account on the same address: %d, want 303", code)
	}
}
