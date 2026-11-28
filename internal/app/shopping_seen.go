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

func (s *seenIDs) seen(userID int64, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.at[seenKey(userID, id)]
	return ok && time.Since(t) < s.ttl
}

func (s *seenIDs) remember(userID int64, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
}
