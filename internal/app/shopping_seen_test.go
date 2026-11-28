package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Of many requests with the same client id that arrive together, one wins; a released id can
// be claimed again; an id is per user; an old one is forgotten.
func TestSeenIDsClaim(t *testing.T) {
	s := &seenIDs{ttl: time.Hour, max: 10, at: map[string]time.Time{}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.claim(1, "abc") {
				mu.Lock()
				won++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if won != 1 {
		t.Fatalf("%d requests claimed the same id, want 1", won)
	}
	if !s.claim(2, "abc") {
		t.Error("the same id of another user is a different one")
	}
	s.release(1, "abc")
	if !s.claim(1, "abc") {
		t.Error("a released id can be claimed again")
	}
	s.at["1/old"] = time.Now().Add(-2 * time.Hour)
	if !s.claim(1, "old") {
		t.Error("an id older than the ttl is forgotten")
	}
	for i := 0; i < 30; i++ { // never grows past its cap
		s.claim(1, string(rune('a'+i%26))+string(rune('A'+i/26)))
	}
	if len(s.at) > s.max {
		t.Errorf("%d ids kept, cap %d", len(s.at), s.max)
	}
}

// A client id that is too long is refused, not read as «already added».
func TestShoppingAddLongCid(t *testing.T) {
	a := newTestApp(t)
	alice := mustCreateTestUser(t, a, "alice@x.com", "password-di-prova")
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("description=Sale&cid="+strings.Repeat("a", 65)))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("X-Shop-Queue", "1")
	w := httptest.NewRecorder()
	a.Sessions.LoadAndSave(http.HandlerFunc(a.handleAddShoppingUserItem)).ServeHTTP(w, withIDParam(requestWithUser(r, alice), 1))
	if w.Code != http.StatusBadRequest {
		t.Errorf("code %d, want 400", w.Code)
	}
}
