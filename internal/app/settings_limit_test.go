package app

import (
	"context"
	"net/url"
	"testing"
)

// Changing the password and deleting the account check the current password, so they
// count on the sign-in limiter: a stolen session must not guess the password freely (#345).
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
			if code, _ := logIn(b, "carlo@example.test", "right-password"); code != 303 { // one try
				t.Fatalf("login: %d", code)
			}
			for i := 0; i < 4; i++ {
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
