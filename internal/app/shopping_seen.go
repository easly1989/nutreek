package app

import (
	"strconv"
	"sync"
	"time"
)

// shopAddsSeen remembers the client ids of the items the queue of the shopping list has
// added, per user, for a day: a request sent again because its answer came late finds its id
// here and adds nothing. It is in memory: a restart in the minutes between the two requests
// is the one case it does not cover.
var shopAddsSeen = &seenIDs{ttl: 24 * time.Hour, max: 5000, at: map[string]time.Time{}}

type seenIDs struct {
	mu  sync.Mutex
	ttl time.Duration
	max int
	at  map[string]time.Time
}

func seenKey(userID int64, id string) string {
	return strconv.FormatInt(userID, 10) + "/" + id
}

// claim marks the id as seen and says whether it was new: of two requests with the same id
// that arrive together (two tabs, or a retry while the first is still running) only one gets true.
func (s *seenIDs) claim(userID int64, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.at[seenKey(userID, id)]; ok && time.Since(t) < s.ttl {
		return false
	}
	if len(s.at) >= s.max { // drop the old ones; if they are all recent, start over
		for k, t := range s.at {
			if time.Since(t) >= s.ttl {
				delete(s.at, k)
			}
		}
		if len(s.at) >= s.max {
			s.at = map[string]time.Time{}
		}
	}
	s.at[seenKey(userID, id)] = time.Now()
	return true
}

// release forgets an id whose request failed, so the retry can add the item.
func (s *seenIDs) release(userID int64, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.at, seenKey(userID, id))
}
